package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// Actor monitoring: each refresh records an actor's holdings and the flows
// since its previous refresh, so a user can see what changed since they last
// looked. Refreshes reuse the graph builder over the new window (the ledger
// fetches only the tail) and the server-side live-holdings job.

const (
	actorMonitorBaselineWindow         = 30 * 24 * time.Hour
	JobActorRefresh            JobKind = "actor_refresh"
)

type ActorHolding struct {
	Chain     string              `json:"chain"`
	Address   string              `json:"address"`
	Label     string              `json:"label"`
	USD       float64             `json:"usd"`
	Status    string              `json:"status"`
	ErrorKind string              `json:"error_kind,omitempty"`
	Assets    []ActorAssetHolding `json:"assets,omitempty"`
}

// ActorAssetHolding is one asset balance at an address; AmountRaw uses the
// app's 1e8 base-unit convention.
type ActorAssetHolding struct {
	Asset     string  `json:"asset"`
	AmountRaw string  `json:"amount_raw"`
	USD       float64 `json:"usd_spot"`
}

type ActorCounterparty struct {
	Key           string   `json:"key"`
	Address       string   `json:"address,omitempty"`
	Chain         string   `json:"chain,omitempty"`
	Label         string   `json:"label"`
	Kind          string   `json:"kind"`
	Category      string   `json:"category,omitempty"`
	InUSD         float64  `json:"in_usd"`
	OutUSD        float64  `json:"out_usd"`
	Transactions  int      `json:"transactions"`
	ActionClasses []string `json:"action_classes"`
	FirstSeen     bool     `json:"first_seen"`
}

type ActorFlowSummary struct {
	InUSD          float64             `json:"in_usd"`
	OutUSD         float64             `json:"out_usd"`
	Transactions   int                 `json:"transactions"`
	Counterparties []ActorCounterparty `json:"counterparties"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type ActorSnapshot struct {
	ID          int64            `json:"id"`
	ActorID     int64            `json:"actor_id"`
	TakenAt     time.Time        `json:"taken_at"`
	WindowStart time.Time        `json:"window_start"`
	WindowEnd   time.Time        `json:"window_end"`
	TotalUSD    float64          `json:"total_usd"`
	Baseline    bool             `json:"baseline"`
	Holdings    []ActorHolding   `json:"holdings"`
	Flows       ActorFlowSummary `json:"flows"`
}

// ActorMonitor is what the monitoring view shows for one actor.
type ActorMonitor struct {
	ActorID       int64              `json:"actor_id"`
	Watch         bool               `json:"watch"`
	LastViewedAt  string             `json:"last_viewed_at"`
	Series        []ActorSeriesPoint `json:"series"`
	Latest        *ActorSnapshot     `json:"latest,omitempty"`
	SinceLastView *ActorChanges      `json:"since_last_view,omitempty"`
}

type ActorSeriesPoint struct {
	TakenAt  time.Time `json:"taken_at"`
	TotalUSD float64   `json:"total_usd"`
}

// ActorChanges summarizes the snapshots taken after the last view.
type ActorChanges struct {
	Since             string              `json:"since"`
	Snapshots         int                 `json:"snapshots"`
	TotalUSDBefore    float64             `json:"total_usd_before"`
	TotalUSDAfter     float64             `json:"total_usd_after"`
	HoldingDeltas     []ActorHoldingDelta `json:"holding_deltas"`
	AssetDeltas       []ActorAssetDelta   `json:"asset_deltas"`
	Flows             ActorFlowSummary    `json:"flows"`
	NewCounterparties []ActorCounterparty `json:"new_counterparties"`
}

type ActorHoldingDelta struct {
	Chain     string  `json:"chain"`
	Address   string  `json:"address"`
	Label     string  `json:"label"`
	BeforeUSD float64 `json:"before_usd"`
	AfterUSD  float64 `json:"after_usd"`
	DeltaUSD  float64 `json:"delta_usd"`
}

type ActorAssetDelta struct {
	Asset        string  `json:"asset"`
	BeforeAmount float64 `json:"before_amount"`
	AfterAmount  float64 `json:"after_amount"`
	BeforeUSD    float64 `json:"before_usd"`
	AfterUSD     float64 `json:"after_usd"`
	DeltaUSD     float64 `json:"delta_usd"`
}

// actorAssetHoldings reads the live_holdings_assets metric, which is a
// []map[string]any when fresh and a []any after a JSON round trip.
func actorAssetHoldings(value any) []ActorAssetHolding {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var assets []ActorAssetHolding
	if err := json.Unmarshal(raw, &assets); err != nil {
		return nil
	}
	return assets
}

func actorAssetUnits(amountRaw string) float64 {
	amount, ok := new(big.Float).SetString(strings.TrimSpace(amountRaw))
	if !ok {
		return 0
	}
	units, _ := new(big.Float).Quo(amount, big.NewFloat(1e8)).Float64()
	return units
}

// actorRefreshLocks prevents two refreshes of one actor from overlapping.
var actorRefreshLocks sync.Map

// RefreshActor records a new snapshot for an actor: flows since the previous
// snapshot (or a 30-day baseline) and current holdings.
func (a *App) RefreshActor(ctx context.Context, actorID int64) (ActorSnapshot, error) {
	lock, _ := actorRefreshLocks.LoadOrStore(actorID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	if !mu.TryLock() {
		return ActorSnapshot{}, fmt.Errorf("actor %d is already refreshing", actorID)
	}
	defer mu.Unlock()

	if _, err := getActorByID(ctx, a.db, actorID); err != nil {
		return ActorSnapshot{}, err
	}
	snapshots, err := listActorSnapshots(ctx, a.db, actorID)
	if err != nil {
		return ActorSnapshot{}, err
	}
	now := time.Now().UTC()
	snap := ActorSnapshot{ActorID: actorID, TakenAt: now, WindowEnd: now, WindowStart: now.Add(-actorMonitorBaselineWindow), Baseline: len(snapshots) == 0}
	if len(snapshots) > 0 {
		snap.WindowStart = snapshots[len(snapshots)-1].WindowEnd
	}

	progress := buildProgressFromContext(ctx)
	progress.set("actor flows", 0, 2, "building flows since "+snap.WindowStart.Format(time.RFC3339))
	graph, err := a.buildActorTracker(withoutBuildLiveHoldings(ctx), ActorTrackerRequest{
		ActorIDs:  []int64{actorID},
		StartTime: snap.WindowStart.Format(time.RFC3339),
		EndTime:   snap.WindowEnd.Format(time.RFC3339),
		MaxHops:   1,
		FlowTypes: []string{"liquidity", "swaps", "bonds", "transfers"},
	})
	if err != nil {
		return ActorSnapshot{}, fmt.Errorf("build actor flows: %w", err)
	}
	seen := map[string]struct{}{}
	for _, previous := range snapshots {
		for _, cp := range previous.Flows.Counterparties {
			seen[cp.Key] = struct{}{}
		}
	}
	snap.Flows = summarizeActorFlows(graph, actorID, seen, snap.Baseline)

	progress.set("actor holdings", 1, 2, "refreshing holdings")
	var addressNodes []FlowNode
	for _, node := range graph.Nodes {
		if node.Kind == "actor_address" {
			addressNodes = append(addressNodes, node)
		}
	}
	if len(addressNodes) > 0 {
		holdings, err := a.runLiveHoldingsJob(ctx, addressNodes, true)
		if err != nil {
			return ActorSnapshot{}, fmt.Errorf("refresh actor holdings: %w", err)
		}
		byID := map[string]FlowNode{}
		for _, node := range addressNodes {
			byID[node.ID] = node
		}
		for _, update := range holdings.Nodes {
			node := byID[update.ID]
			holding := ActorHolding{
				Chain:     node.Chain,
				Address:   getString(node.Metrics, "address"),
				Label:     node.Label,
				Status:    getString(update.Metrics, "live_holdings_status"),
				ErrorKind: getString(update.Metrics, "live_holdings_error_kind"),
			}
			if chain, _, ok := liveHoldingsAddressKey(node); ok && holding.Chain == "" {
				holding.Chain = chain
			}
			if usd, ok := update.Metrics["live_holdings_usd_spot"].(float64); ok {
				holding.USD = usd
			}
			holding.Assets = actorAssetHoldings(update.Metrics["live_holdings_assets"])
			snap.TotalUSD += holding.USD
			snap.Holdings = append(snap.Holdings, holding)
		}
		snap.Flows.Warnings = append(snap.Flows.Warnings, holdings.Warnings...)
	}
	sort.Slice(snap.Holdings, func(i, j int) bool { return snap.Holdings[i].USD > snap.Holdings[j].USD })

	id, err := insertActorSnapshot(ctx, a.db, snap)
	if err != nil {
		return ActorSnapshot{}, err
	}
	snap.ID = id
	progress.set("done", 2, 2, "")
	return snap, nil
}

// summarizeActorFlows aggregates edges between the actor's addresses and
// everything else by counterparty, valued at transaction time.
func summarizeActorFlows(graph ActorTrackerResponse, actorID int64, seen map[string]struct{}, baseline bool) ActorFlowSummary {
	nodes := map[string]FlowNode{}
	own := map[string]bool{}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
		if node.Kind == "actor_address" || node.Kind == "actor" {
			for _, id := range node.ActorIDs {
				if id == actorID {
					own[node.ID] = true
				}
			}
		}
	}
	byKey := map[string]*ActorCounterparty{}
	var summary ActorFlowSummary
	for _, edge := range graph.Edges {
		if edge.ActionClass == "ownership" {
			continue
		}
		fromOwn, toOwn := own[edge.From], own[edge.To]
		if fromOwn == toOwn {
			continue
		}
		otherID := edge.To
		if toOwn {
			otherID = edge.From
		}
		other := nodes[otherID]
		key := otherID
		if address := normalizeAddress(getString(other.Metrics, "address")); address != "" {
			key = strings.ToUpper(other.Chain) + "|" + address
		}
		cp, ok := byKey[key]
		if !ok {
			cp = &ActorCounterparty{
				Key:      key,
				Address:  getString(other.Metrics, "address"),
				Chain:    other.Chain,
				Label:    firstNonEmpty(other.Label, otherID),
				Kind:     other.Kind,
				Category: getString(other.Metrics, "label_category"),
			}
			if _, known := seen[key]; !known && !baseline {
				cp.FirstSeen = true
			}
			byKey[key] = cp
		}
		if toOwn {
			cp.InUSD += edge.USDAtTime
			summary.InUSD += edge.USDAtTime
		} else {
			cp.OutUSD += edge.USDAtTime
			summary.OutUSD += edge.USDAtTime
		}
		cp.Transactions += len(edge.Transactions)
		summary.Transactions += len(edge.Transactions)
		if !containsString(cp.ActionClasses, edge.ActionClass) {
			cp.ActionClasses = append(cp.ActionClasses, edge.ActionClass)
		}
	}
	for _, cp := range byKey {
		summary.Counterparties = append(summary.Counterparties, *cp)
	}
	sort.Slice(summary.Counterparties, func(i, j int) bool {
		vi := summary.Counterparties[i].InUSD + summary.Counterparties[i].OutUSD
		vj := summary.Counterparties[j].InUSD + summary.Counterparties[j].OutUSD
		if vi == vj {
			return summary.Counterparties[i].Key < summary.Counterparties[j].Key
		}
		return vi > vj
	})
	return summary
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func insertActorSnapshot(ctx context.Context, db *sql.DB, snap ActorSnapshot) (int64, error) {
	holdings, err := json.Marshal(snap.Holdings)
	if err != nil {
		return 0, err
	}
	flows, err := json.Marshal(snap.Flows)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO actor_snapshots(actor_id, taken_at, window_start, window_end, total_usd, baseline, holdings_json, flows_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, snap.ActorID, snap.TakenAt.Format(time.RFC3339Nano), snap.WindowStart.Format(time.RFC3339Nano), snap.WindowEnd.Format(time.RFC3339Nano),
		snap.TotalUSD, snap.Baseline, string(holdings), string(flows))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func listActorSnapshots(ctx context.Context, db *sql.DB, actorID int64) ([]ActorSnapshot, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, taken_at, window_start, window_end, total_usd, baseline, holdings_json, flows_json
		FROM actor_snapshots WHERE actor_id = ? ORDER BY taken_at ASC, id ASC
	`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActorSnapshot
	for rows.Next() {
		snap := ActorSnapshot{ActorID: actorID}
		var takenAt, windowStart, windowEnd, holdings, flows string
		if err := rows.Scan(&snap.ID, &takenAt, &windowStart, &windowEnd, &snap.TotalUSD, &snap.Baseline, &holdings, &flows); err != nil {
			return nil, err
		}
		snap.TakenAt, _ = time.Parse(time.RFC3339Nano, takenAt)
		snap.WindowStart, _ = time.Parse(time.RFC3339Nano, windowStart)
		snap.WindowEnd, _ = time.Parse(time.RFC3339Nano, windowEnd)
		_ = json.Unmarshal([]byte(holdings), &snap.Holdings)
		_ = json.Unmarshal([]byte(flows), &snap.Flows)
		out = append(out, snap)
	}
	return out, rows.Err()
}

func actorWatchState(ctx context.Context, db *sql.DB, actorID int64) (bool, string) {
	var watch bool
	var lastViewed string
	_ = db.QueryRowContext(ctx, `SELECT watch, last_viewed_at FROM actor_watch WHERE actor_id = ?`, actorID).Scan(&watch, &lastViewed)
	return watch, lastViewed
}

// SetActorWatch turns scheduled refreshes on or off for an actor.
func (a *App) SetActorWatch(ctx context.Context, actorID int64, watch bool) error {
	if _, err := getActorByID(ctx, a.db, actorID); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO actor_watch(actor_id, watch) VALUES (?, ?)
		ON CONFLICT(actor_id) DO UPDATE SET watch = excluded.watch
	`, actorID, watch)
	return err
}

// MarkActorViewed records that the user has seen the actor's changes.
func (a *App) MarkActorViewed(ctx context.Context, actorID int64) error {
	if _, err := getActorByID(ctx, a.db, actorID); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO actor_watch(actor_id, last_viewed_at) VALUES (?, ?)
		ON CONFLICT(actor_id) DO UPDATE SET last_viewed_at = excluded.last_viewed_at
	`, actorID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// ActorMonitor returns an actor's holdings series, latest snapshot, and what
// changed in snapshots taken after the user last viewed it.
func (a *App) ActorMonitor(ctx context.Context, actorID int64) (ActorMonitor, error) {
	if _, err := getActorByID(ctx, a.db, actorID); err != nil {
		return ActorMonitor{}, err
	}
	watch, lastViewed := actorWatchState(ctx, a.db, actorID)
	snapshots, err := listActorSnapshots(ctx, a.db, actorID)
	if err != nil {
		return ActorMonitor{}, err
	}
	out := ActorMonitor{ActorID: actorID, Watch: watch, LastViewedAt: lastViewed, Series: []ActorSeriesPoint{}}
	for _, snap := range snapshots {
		out.Series = append(out.Series, ActorSeriesPoint{TakenAt: snap.TakenAt, TotalUSD: snap.TotalUSD})
	}
	if len(snapshots) == 0 {
		return out, nil
	}
	latest := snapshots[len(snapshots)-1]
	out.Latest = &latest
	out.SinceLastView = actorChangesSince(snapshots, lastViewed)
	return out, nil
}

// actorChangesSince merges snapshots taken after lastViewed (all
// non-baseline snapshots when the actor was never viewed).
func actorChangesSince(snapshots []ActorSnapshot, lastViewed string) *ActorChanges {
	viewedAt, _ := time.Parse(time.RFC3339Nano, lastViewed)
	var before *ActorSnapshot
	var after []ActorSnapshot
	for i := range snapshots {
		snap := snapshots[i]
		if (!viewedAt.IsZero() && !snap.TakenAt.After(viewedAt)) || (viewedAt.IsZero() && snap.Baseline) {
			before = &snapshots[i]
			continue
		}
		after = append(after, snap)
	}
	if len(after) == 0 {
		return nil
	}
	changes := &ActorChanges{Since: lastViewed, Snapshots: len(after)}
	latest := after[len(after)-1]
	changes.TotalUSDAfter = latest.TotalUSD
	beforeHoldings := map[string]ActorHolding{}
	if before != nil {
		changes.TotalUSDBefore = before.TotalUSD
		for _, h := range before.Holdings {
			beforeHoldings[strings.ToUpper(h.Chain)+"|"+normalizeAddress(h.Address)] = h
		}
	}
	for _, h := range latest.Holdings {
		key := strings.ToUpper(h.Chain) + "|" + normalizeAddress(h.Address)
		prior := beforeHoldings[key]
		delete(beforeHoldings, key)
		changes.HoldingDeltas = append(changes.HoldingDeltas, ActorHoldingDelta{
			Chain: h.Chain, Address: h.Address, Label: h.Label,
			BeforeUSD: prior.USD, AfterUSD: h.USD, DeltaUSD: h.USD - prior.USD,
		})
	}
	for _, prior := range beforeHoldings {
		changes.HoldingDeltas = append(changes.HoldingDeltas, ActorHoldingDelta{
			Chain: prior.Chain, Address: prior.Address, Label: prior.Label,
			BeforeUSD: prior.USD, DeltaUSD: -prior.USD,
		})
	}
	assetDeltas := map[string]*ActorAssetDelta{}
	assetDelta := func(asset string) *ActorAssetDelta {
		key := strings.ToUpper(asset)
		if assetDeltas[key] == nil {
			assetDeltas[key] = &ActorAssetDelta{Asset: asset}
		}
		return assetDeltas[key]
	}
	if before != nil {
		for _, h := range before.Holdings {
			for _, asset := range h.Assets {
				d := assetDelta(asset.Asset)
				d.BeforeAmount += actorAssetUnits(asset.AmountRaw)
				d.BeforeUSD += asset.USD
			}
		}
	}
	for _, h := range latest.Holdings {
		for _, asset := range h.Assets {
			d := assetDelta(asset.Asset)
			d.AfterAmount += actorAssetUnits(asset.AmountRaw)
			d.AfterUSD += asset.USD
		}
	}
	for _, d := range assetDeltas {
		d.DeltaUSD = d.AfterUSD - d.BeforeUSD
		changes.AssetDeltas = append(changes.AssetDeltas, *d)
	}
	sort.Slice(changes.AssetDeltas, func(i, j int) bool {
		di, dj := changes.AssetDeltas[i], changes.AssetDeltas[j]
		if absFloat(di.DeltaUSD) != absFloat(dj.DeltaUSD) {
			return absFloat(di.DeltaUSD) > absFloat(dj.DeltaUSD)
		}
		return di.Asset < dj.Asset
	})
	sort.Slice(changes.HoldingDeltas, func(i, j int) bool {
		return absFloat(changes.HoldingDeltas[i].DeltaUSD) > absFloat(changes.HoldingDeltas[j].DeltaUSD)
	})
	merged := map[string]*ActorCounterparty{}
	var order []string
	for _, snap := range after {
		changes.Flows.InUSD += snap.Flows.InUSD
		changes.Flows.OutUSD += snap.Flows.OutUSD
		changes.Flows.Transactions += snap.Flows.Transactions
		for _, cp := range snap.Flows.Counterparties {
			existing, ok := merged[cp.Key]
			if !ok {
				copy := cp
				merged[cp.Key] = &copy
				order = append(order, cp.Key)
				continue
			}
			existing.InUSD += cp.InUSD
			existing.OutUSD += cp.OutUSD
			existing.Transactions += cp.Transactions
			existing.FirstSeen = existing.FirstSeen || cp.FirstSeen
			for _, class := range cp.ActionClasses {
				if !containsString(existing.ActionClasses, class) {
					existing.ActionClasses = append(existing.ActionClasses, class)
				}
			}
		}
	}
	for _, key := range order {
		cp := *merged[key]
		changes.Flows.Counterparties = append(changes.Flows.Counterparties, cp)
		if cp.FirstSeen {
			changes.NewCounterparties = append(changes.NewCounterparties, cp)
		}
	}
	sort.Slice(changes.Flows.Counterparties, func(i, j int) bool {
		ci, cj := changes.Flows.Counterparties[i], changes.Flows.Counterparties[j]
		return ci.InUSD+ci.OutUSD > cj.InUSD+cj.OutUSD
	})
	return changes
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// StartActorRefreshJob refreshes an actor in the background.
func (a *App) StartActorRefreshJob(actorID int64) JobSnapshot {
	return a.jobs.start(JobActorRefresh, analysisJobTimeout, "", func(ctx context.Context) (any, error) {
		return a.RefreshActor(ctx, actorID)
	})
}

// runActorScheduler refreshes watched actors every interval until ctx ends.
func (a *App) runActorScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := a.db.QueryContext(ctx, `SELECT actor_id FROM actor_watch WHERE watch = 1`)
			if err != nil {
				logError(ctx, "actor_scheduler_list_failed", err, nil)
				continue
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			for _, id := range ids {
				job := a.StartActorRefreshJob(id)
				logInfo(ctx, "actor_scheduler_refresh_started", map[string]any{"actor_id": id, "job_id": job.ID})
			}
		}
	}
}
