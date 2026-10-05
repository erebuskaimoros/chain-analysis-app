package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	liveHoldingsSnapshotFresh = 10 * time.Minute
	liveHoldingsJobPasses     = 6
	liveHoldingsJobBudget     = 4 * time.Minute
	liveHoldingsMaxWait       = 30 * time.Second
)

// LiveHoldingsNodeUpdate carries a node's refreshed live-holdings metrics.
type LiveHoldingsNodeUpdate struct {
	ID      string         `json:"id"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

// LiveHoldingsJobResult is the result (and partial result) of a
// live-holdings job: one update per requested node.
type LiveHoldingsJobResult struct {
	Nodes       []LiveHoldingsNodeUpdate `json:"nodes"`
	Warnings    []string                 `json:"warnings"`
	RefreshedAt time.Time                `json:"refreshed_at"`
}

// liveHoldingsAddressKey returns the (chain, address) a node's holdings are
// looked up by, matching the enrichment's address lookup tasks.
func liveHoldingsAddressKey(node FlowNode) (string, string, bool) {
	switch node.Kind {
	case "pool", "node":
		return "", "", false
	}
	address := normalizeAddress(getString(node.Metrics, "address"))
	if address == "" {
		return "", "", false
	}
	chain := strings.ToUpper(strings.TrimSpace(node.Chain))
	if chain == "" {
		chain = normalizeChain("", address)
	}
	if chain == "" {
		return "", "", false
	}
	return chain, address, true
}

func liveHoldingsMetricsSubset(metrics map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range metrics {
		if strings.HasPrefix(key, "live_holdings") {
			out[key] = value
		}
	}
	return out
}

// liveHoldingsFinal reports whether a node's lookup has reached a state that
// retrying will not change: a value, or a failure that is not retryable.
func liveHoldingsFinal(metrics map[string]any) bool {
	switch strings.ToLower(getString(metrics, "live_holdings_status")) {
	case "available":
		return true
	case "error":
		return !isRetryableProviderKind(providerErrorKind(getString(metrics, "live_holdings_error_kind")))
	default:
		return false
	}
}

// runLiveHoldingsJob refreshes live holdings for nodes on the server: it reuses
// recent snapshots (unless force), retries only retryable failures while
// honoring provider backoff, publishes updates after every pass, and ends with
// every node in a final state.
func (a *App) runLiveHoldingsJob(ctx context.Context, nodes []FlowNode, force bool) (LiveHoldingsJobResult, error) {
	started := time.Now()
	deadline := started.Add(liveHoldingsJobBudget)
	order := make([]string, 0, len(nodes))
	latest := map[string]FlowNode{}
	warnings := map[string]struct{}{}
	progress := buildProgressFromContext(ctx)

	result := func() LiveHoldingsJobResult {
		out := LiveHoldingsJobResult{RefreshedAt: time.Now().UTC(), Warnings: []string{}}
		for _, id := range order {
			if node, ok := latest[id]; ok {
				out.Nodes = append(out.Nodes, LiveHoldingsNodeUpdate{ID: node.ID, Metrics: liveHoldingsMetricsSubset(node.Metrics)})
			}
		}
		for warning := range warnings {
			out.Warnings = append(out.Warnings, warning)
		}
		sort.Strings(out.Warnings)
		return out
	}
	publish := func() {
		publishJobPartial(ctx, func() (any, map[string]int) {
			res := result()
			return res, map[string]int{"nodes": len(res.Nodes), "total": len(nodes)}
		})
	}

	pending := make([]FlowNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Metrics == nil {
			node.Metrics = map[string]any{}
		}
		order = append(order, node.ID)
		if !force {
			if chain, address, ok := liveHoldingsAddressKey(node); ok {
				if metrics, found := latestHoldingsSnapshot(ctx, a.db, chain, address, started.Add(-liveHoldingsSnapshotFresh)); found {
					for key, value := range metrics {
						node.Metrics[key] = value
					}
					latest[node.ID] = node
					continue
				}
			}
		}
		pending = append(pending, node)
	}
	publish()

	for pass := 0; pass < liveHoldingsJobPasses && len(pending) > 0 && ctx.Err() == nil; pass++ {
		progress.set("live holdings", len(latest), len(nodes), fmt.Sprintf("pass %d: %d lookups", pass+1, len(pending)))
		passCtx, cancel := context.WithTimeout(ctx, a.cfg.RequestTimeout*2)
		passWarnings, err := a.refreshActorTrackerLiveHoldings(passCtx, pending)
		cancel()
		if err != nil {
			return result(), err
		}
		for _, warning := range passWarnings {
			if !strings.Contains(warning, "budget exhausted") && !strings.HasPrefix(warning, "live holdings unavailable for") {
				warnings[warning] = struct{}{}
			}
		}

		lastPass := pass == liveHoldingsJobPasses-1
		var retry []FlowNode
		for _, node := range pending {
			latest[node.ID] = node
			if liveHoldingsFinal(node.Metrics) {
				a.saveHoldingsSnapshot(ctx, node)
				continue
			}
			if !lastPass {
				retry = append(retry, node)
			}
		}
		pending = retry
		publish()
		if len(pending) == 0 {
			break
		}

		wait := min(time.Duration(pass+1)*time.Second, liveHoldingsMaxWait)
		for _, node := range pending {
			if chain, _, ok := liveHoldingsAddressKey(node); ok {
				if until, _ := a.trackerHealth.circuitOpenUntil(a.liveHoldingsProviderForChain(chain), chain); !until.IsZero() {
					wait = max(wait, min(time.Until(until), liveHoldingsMaxWait))
				}
			}
		}
		if time.Now().Add(wait).After(deadline) || !sleepWithContext(ctx, wait) {
			break
		}
	}

	// Anything still unresolved is reported as a final failure, never left
	// pending.
	failedByKind := map[string]int{}
	for _, id := range order {
		node, ok := latest[id]
		if !ok {
			continue
		}
		if strings.EqualFold(getString(node.Metrics, "live_holdings_status"), "available") {
			continue
		}
		kind := getString(node.Metrics, "live_holdings_error_kind")
		if !liveHoldingsFinal(node.Metrics) {
			node.Metrics["live_holdings_status"] = "error"
			node.Metrics["live_holdings_available"] = false
			node.Metrics["live_holdings_error_kind"] = "retries_exhausted"
			if kind != "" {
				node.Metrics["live_holdings_last_error_kind"] = kind
			}
			kind = "retries_exhausted"
			latest[id] = node
		}
		if _, _, ok := liveHoldingsAddressKey(node); ok {
			failedByKind[firstNonEmpty(kind, "unknown")]++
		}
	}
	if len(failedByKind) > 0 {
		kinds := make([]string, 0, len(failedByKind))
		total := 0
		for kind, count := range failedByKind {
			kinds = append(kinds, fmt.Sprintf("%s %d", strings.ReplaceAll(kind, "_", " "), count))
			total += count
		}
		sort.Strings(kinds)
		warnings[fmt.Sprintf("live holdings unavailable for %d address nodes (%s)", total, strings.Join(kinds, ", "))] = struct{}{}
	}
	progress.set("live holdings", len(latest), len(nodes), "done")
	return result(), nil
}

// saveHoldingsSnapshot records a final lookup result for reuse and history.
func (a *App) saveHoldingsSnapshot(ctx context.Context, node FlowNode) {
	chain, address, ok := liveHoldingsAddressKey(node)
	if !ok {
		return
	}
	metrics := liveHoldingsMetricsSubset(node.Metrics)
	raw, err := json.Marshal(metrics)
	if err != nil {
		return
	}
	usd := 0.0
	if value, ok := metrics["live_holdings_usd_spot"].(float64); ok {
		usd = value
	}
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO holdings_snapshots(chain, address, taken_at, status, usd_total, error_kind, metrics_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chain, address, taken_at) DO UPDATE SET
			status = excluded.status, usd_total = excluded.usd_total,
			error_kind = excluded.error_kind, metrics_json = excluded.metrics_json
	`, chain, address, time.Now().UTC().Unix(), getString(metrics, "live_holdings_status"), usd,
		getString(metrics, "live_holdings_error_kind"), string(raw)); err != nil {
		logError(ctx, "holdings_snapshot_save_failed", err, map[string]any{"chain": chain, "address": address})
	}
}

// latestHoldingsSnapshot returns the newest final snapshot taken after since.
func latestHoldingsSnapshot(ctx context.Context, db *sql.DB, chain, address string, since time.Time) (map[string]any, bool) {
	var raw string
	err := db.QueryRowContext(ctx, `
		SELECT metrics_json FROM holdings_snapshots
		WHERE chain = ? AND address = ? AND taken_at >= ?
		ORDER BY taken_at DESC LIMIT 1
	`, chain, address, since.Unix()).Scan(&raw)
	if err != nil {
		return nil, false
	}
	metrics := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metrics); err != nil {
		return nil, false
	}
	return metrics, true
}
