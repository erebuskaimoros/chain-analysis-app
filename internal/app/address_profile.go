package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// An address profile answers "what is this address?": its labels, what it
// holds now, and who it dealt with recently (one hop, valued at transaction
// time).

const (
	JobAddressProfile        JobKind = "address_profile"
	JobLabelsImport          JobKind = "labels_import"
	addressProfileDefaultDay         = 30
	addressProfileRecentTxs          = 25
	addressProfileHoldings           = 60 * time.Second
)

type AddressProfileRequest struct {
	Chain   string `json:"chain,omitempty"`
	Address string `json:"address"`
	Days    int    `json:"days,omitempty"`
}

type AddressProfileTx struct {
	Time         time.Time        `json:"time"`
	Direction    string           `json:"direction"`
	Counterparty string           `json:"counterparty"`
	Category     string           `json:"category,omitempty"`
	Action       string           `json:"action"`
	Assets       []FlowAssetValue `json:"assets"`
	USDAtTime    float64          `json:"usd_at_time"`
	TxID         string           `json:"tx_id"`
	InboundTxID  string           `json:"inbound_tx_id,omitempty"`
}

type AddressProfile struct {
	Chain     string             `json:"chain"`
	Address   string             `json:"address"`
	Labels    []AddressLabel     `json:"labels"`
	Holdings  ActorHolding       `json:"holdings"`
	Start     time.Time          `json:"start"`
	End       time.Time          `json:"end"`
	Flows     ActorFlowSummary   `json:"flows"`
	Recent    []AddressProfileTx `json:"recent"`
	Warnings  []string           `json:"warnings"`
	Explorer  string             `json:"explorer_url,omitempty"`
	FetchedAt time.Time          `json:"fetched_at"`
}

// AddressProfile builds a profile of one address over the last req.Days.
func (a *App) AddressProfile(ctx context.Context, req AddressProfileRequest) (AddressProfile, error) {
	raw := strings.TrimSpace(req.Address)
	if chain := strings.ToUpper(strings.TrimSpace(req.Chain)); chain != "" && !strings.Contains(raw, "|") {
		raw = chain + "|" + raw
	}
	seed := normalizeFrontierAddress(raw)
	if seed.Address == "" {
		return AddressProfile{}, fmt.Errorf("invalid address %q", req.Address)
	}
	days := req.Days
	if days <= 0 {
		days = addressProfileDefaultDay
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	chain := normalizeChain(seed.Chain, seed.Address)
	key := frontierKey(chain, seed.Address)
	profile := AddressProfile{
		Chain: chain, Address: seed.Address, Start: start, End: end, FetchedAt: end,
		Labels: []AddressLabel{}, Recent: []AddressProfileTx{}, Warnings: []string{},
		Explorer: explorerAddressURL(chain, seed.Address),
	}
	if labels, err := a.AddressLabels(ctx, seed.Address); err == nil && labels != nil {
		profile.Labels = labels
	}

	progress := buildProgressFromContext(ctx)
	progress.set("flows", 0, 2, fmt.Sprintf("one hop over %d days", days))
	graph, err := a.expandActorTrackerOneHop(withoutBuildLiveHoldings(ctx), ActorTrackerExpandRequest{
		Addresses:       []string{encodeFrontierAddress(seed)},
		StartTime:       start.Format(time.RFC3339),
		EndTime:         end.Format(time.RFC3339),
		IncludeUnpriced: true,
	})
	if err != nil {
		return AddressProfile{}, fmt.Errorf("fetch flows: %w", err)
	}
	profile.Warnings = append(profile.Warnings, graph.Warnings...)
	profile.Flows, profile.Recent = summarizeAddressFlows(graph, key)

	progress.set("holdings", 1, 2, "looking up current holdings")
	node := FlowNode{ID: key, Kind: "external_address", Chain: chain, Label: shortAddress(seed.Address), Metrics: map[string]any{"address": seed.Address}}
	holdingsCtx, cancel := context.WithTimeout(ctx, addressProfileHoldings)
	defer cancel()
	profile.Holdings = ActorHolding{Chain: chain, Address: seed.Address, Label: node.Label, Status: "unavailable"}
	if holdings, err := a.runLiveHoldingsJob(holdingsCtx, []FlowNode{node}, false); err != nil {
		profile.Warnings = append(profile.Warnings, "holdings lookup failed: "+err.Error())
	} else if len(holdings.Nodes) > 0 {
		update := holdings.Nodes[0]
		profile.Holdings.Status = getString(update.Metrics, "live_holdings_status")
		profile.Holdings.ErrorKind = getString(update.Metrics, "live_holdings_error_kind")
		if usd, ok := update.Metrics["live_holdings_usd_spot"].(float64); ok {
			profile.Holdings.USD = usd
		}
		profile.Holdings.Assets = actorAssetHoldings(update.Metrics["live_holdings_assets"])
	}
	return profile, nil
}

// summarizeAddressFlows totals an address's one-hop flows by counterparty and
// lists its most recent transactions.
func summarizeAddressFlows(graph ActorTrackerResponse, key string) (ActorFlowSummary, []AddressProfileTx) {
	nodes := map[string]FlowNode{}
	own := map[string]bool{}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
		if nodeKey, _, _, _ := traceNodeIdentity(node); nodeKey == key {
			own[node.ID] = true
		}
	}
	summary := ActorFlowSummary{}
	byKey := map[string]*ActorCounterparty{}
	recent := []AddressProfileTx{}
	for _, edge := range graph.Edges {
		if edge.ActionClass == "ownership" || own[edge.From] == own[edge.To] {
			continue
		}
		incoming := own[edge.To]
		other := nodes[edge.To]
		direction := "out"
		if incoming {
			other, direction = nodes[edge.From], "in"
		}
		cpKey := other.ID
		if nodeKey, _, _, _ := traceNodeIdentity(other); nodeKey != "" {
			cpKey = nodeKey
		}
		cp := byKey[cpKey]
		if cp == nil {
			cp = &ActorCounterparty{
				Key: cpKey, Address: getString(other.Metrics, "address"), Chain: other.Chain,
				Label: firstNonEmpty(other.Label, other.ID), Kind: other.Kind, Category: getString(other.Metrics, "label_category"),
			}
			byKey[cpKey] = cp
		}
		if incoming {
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
		for _, tx := range edge.Transactions {
			recent = append(recent, AddressProfileTx{
				Time: tx.Time, Direction: direction, Counterparty: cp.Label, Category: cp.Category,
				Action: firstNonEmpty(edge.ActionLabel, edge.ActionClass), Assets: tx.Assets, USDAtTime: tx.USDAtTime,
				TxID: tx.TxID, InboundTxID: tx.InboundTxID,
			})
		}
	}
	for _, cp := range byKey {
		summary.Counterparties = append(summary.Counterparties, *cp)
	}
	sort.Slice(summary.Counterparties, func(i, j int) bool {
		ci, cj := summary.Counterparties[i], summary.Counterparties[j]
		if ci.InUSD+ci.OutUSD != cj.InUSD+cj.OutUSD {
			return ci.InUSD+ci.OutUSD > cj.InUSD+cj.OutUSD
		}
		return ci.Key < cj.Key
	})
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].Time.After(recent[j].Time) })
	if len(recent) > addressProfileRecentTxs {
		recent = recent[:addressProfileRecentTxs]
	}
	return summary, recent
}

// StartAddressProfileJob builds an address profile in the background.
func (a *App) StartAddressProfileJob(req AddressProfileRequest) JobSnapshot {
	return a.jobs.start(JobAddressProfile, analysisJobTimeout, "", func(ctx context.Context) (any, error) {
		return a.AddressProfile(ctx, req)
	})
}

// StartLabelsImportJob imports a label source from a path on this machine.
func (a *App) StartLabelsImportJob(source, path string) JobSnapshot {
	return a.jobs.start(JobLabelsImport, analysisJobTimeout, "", func(ctx context.Context) (any, error) {
		return a.ImportLabels(ctx, source, path)
	})
}

// LedgerRange is a span of ledger time.
type LedgerRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type LedgerSourceCoverage struct {
	Source   string        `json:"source"`
	Covered  []LedgerRange `json:"covered"`
	Gaps     []LedgerRange `json:"gaps"`
	Deferred []LedgerRange `json:"deferred"`
}

type LedgerCoverageReport struct {
	Address string                 `json:"address"`
	Start   time.Time              `json:"start"`
	End     time.Time              `json:"end"`
	Sources []LedgerSourceCoverage `json:"sources"`
}

func ledgerRanges(intervals []ledgerInterval) []LedgerRange {
	out := make([]LedgerRange, 0, len(intervals))
	for _, iv := range intervals {
		out = append(out, LedgerRange{From: time.Unix(iv.From, 0).UTC(), To: time.Unix(iv.To, 0).UTC()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From.Before(out[j].From) })
	return out
}

// LedgerCoverage reports, per source, which parts of [start, end] the ledger
// holds for an address and which it would still have to fetch.
func (a *App) LedgerCoverage(ctx context.Context, address string, start, end time.Time) (LedgerCoverageReport, error) {
	address = strings.TrimSpace(address)
	if _, plain, ok := strings.Cut(address, "|"); ok {
		address = plain
	}
	if address == "" {
		return LedgerCoverageReport{}, fmt.Errorf("address is required")
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if start.IsZero() {
		start = end.Add(-addressProfileDefaultDay * 24 * time.Hour)
	}
	report := LedgerCoverageReport{Address: address, Start: start, End: end, Sources: []LedgerSourceCoverage{}}
	rows, err := a.db.QueryContext(ctx, `
		SELECT DISTINCT source, address FROM ledger_coverage WHERE lower(address) = lower(?)
		UNION SELECT DISTINCT source, address FROM ledger_deferrals WHERE lower(address) = lower(?)
		ORDER BY 1
	`, address, address)
	if err != nil {
		return LedgerCoverageReport{}, err
	}
	type sourceKey struct{ source, address string }
	var keys []sourceKey
	for rows.Next() {
		var k sourceKey
		if err := rows.Scan(&k.source, &k.address); err != nil {
			rows.Close()
			return LedgerCoverageReport{}, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	for _, k := range keys {
		covered, err := loadLedgerCoverage(ctx, a.db, k.source, k.address)
		if err != nil {
			return LedgerCoverageReport{}, err
		}
		deferred, err := loadActiveLedgerDeferrals(ctx, a.db, k.source, k.address, 0, time.Now().UTC())
		if err != nil {
			return LedgerCoverageReport{}, err
		}
		report.Sources = append(report.Sources, LedgerSourceCoverage{
			Source:   k.source,
			Covered:  ledgerRanges(covered),
			Gaps:     ledgerRanges(ledgerGaps(covered, start.Unix(), end.Unix())),
			Deferred: ledgerRanges(deferred),
		})
	}
	return report, nil
}
