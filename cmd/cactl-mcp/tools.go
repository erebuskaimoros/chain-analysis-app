package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/client"
)

// Tool results are digests: the parts of a trace, profile or actor summary
// a reader needs, capped so they fit comfortably in a conversation.
const (
	digestFlows          = 40
	digestEndpoints      = 25
	digestCounterparties = 20
)

type toolset struct {
	client *client.Client
}

var txHashPattern = regexp.MustCompile(`^(0[xX])?[0-9a-fA-F]{64}$`)

type traceFundsInput struct {
	Seeds           []string `json:"seeds" jsonschema:"addresses (CHAIN|address when the chain is ambiguous, e.g. ETH|0x…) or 64-hex transaction hashes to trace from"`
	StartTime       string   `json:"start_time,omitempty" jsonschema:"window start, RFC 3339 (optional for a transaction hash seed)"`
	EndTime         string   `json:"end_time,omitempty" jsonschema:"window end, RFC 3339 (default start + 30 days, capped at now)"`
	Direction       string   `json:"direction,omitempty" jsonschema:"forward (where did it go, default) or backward (where did it come from)"`
	Policy          string   `json:"policy,omitempty" jsonschema:"allocation at mixed addresses: fifo (default), haircut or largest_out"`
	MaxDepth        int      `json:"max_depth,omitempty" jsonschema:"hops to follow (default 3, max 8)"`
	MaxBranches     int      `json:"max_branches,omitempty" jsonschema:"branches followed per address (default 8)"`
	Amount          float64  `json:"amount,omitempty" jsonschema:"amount to trace in asset units (default: everything)"`
	Asset           string   `json:"asset,omitempty" jsonschema:"asset of amount, such as ETH.ETH or BTC.BTC"`
	MinUSDAtTime    float64  `json:"min_usd_at_time,omitempty" jsonschema:"skip branches worth less than this at transaction time"`
	StopCategories  []string `json:"stop_categories,omitempty" jsonschema:"label categories that end the trace (default exchange, sanctioned, mixer)"`
	IncludeHoldings bool     `json:"include_holdings,omitempty" jsonschema:"look up what each endpoint holds now (slower)"`
}

type endpointDigest struct {
	Label     string            `json:"label"`
	Chain     string            `json:"chain,omitempty"`
	Address   string            `json:"address,omitempty"`
	Category  string            `json:"category,omitempty"`
	Reason    string            `json:"reason"`
	Hop       int               `json:"hop"`
	Traced    []app.TraceAmount `json:"traced"`
	USD       float64           `json:"usd_at_time"`
	Conf      float64           `json:"confidence"`
	HoldsUSD  *float64          `json:"holds_now_usd,omitempty"`
	FirstSeen time.Time         `json:"first_at"`
}

type flowDigest struct {
	Time        time.Time `json:"time"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Action      string    `json:"action"`
	Traced      string    `json:"traced"`
	USD         float64   `json:"usd_at_time"`
	TxID        string    `json:"tx_id"`
	InboundTxID string    `json:"inbound_tx_id,omitempty"`
	Confidence  float64   `json:"confidence"`
	Reason      string    `json:"confidence_reason,omitempty"`
}

type traceDigest struct {
	RunID          int64             `json:"run_id"`
	Direction      string            `json:"direction"`
	Window         string            `json:"window"`
	Method         []string          `json:"method"`
	Seeds          []app.TraceAmount `json:"traced_from_seeds"`
	SeedUSD        float64           `json:"seed_usd"`
	SinkUSD        float64           `json:"sink_usd"`
	FrontierUSD    float64           `json:"frontier_usd"`
	Sinks          []endpointDigest  `json:"sinks"`
	Frontier       []endpointDigest  `json:"frontier"`
	Flows          []flowDigest      `json:"largest_flows"`
	TotalFlows     int               `json:"total_flows"`
	CoverageGaps   []string          `json:"coverage_gaps"`
	Warnings       []string          `json:"warnings"`
	TruncatedLists bool              `json:"truncated_lists"`
}

func digestEndpoint(e app.TraceEndpoint) endpointDigest {
	return endpointDigest{
		Label: e.Label, Chain: e.Chain, Address: e.Address, Category: e.Category, Reason: e.Reason, Hop: e.Depth,
		Traced: e.Assets, USD: e.TracedUSDAtTime, Conf: e.Confidence, HoldsUSD: e.HoldingsUSD, FirstSeen: e.FirstAt,
	}
}

func digestTrace(resp app.TraceResponse) traceDigest {
	labels := map[string]string{}
	for _, node := range resp.Nodes {
		label := firstNonEmpty(node.Label, node.ID)
		if address, _ := node.Metrics["address"].(string); address != "" && !strings.Contains(label, address) {
			label = fmt.Sprintf("%s (%s %s)", label, node.Chain, address)
		}
		labels[node.ID] = label
	}
	d := traceDigest{
		RunID: resp.RunID, Direction: resp.Query.Direction, Method: resp.Method,
		Window: resp.Query.StartTime.UTC().Format(time.RFC3339) + " to " + resp.Query.EndTime.UTC().Format(time.RFC3339),
		Seeds:  resp.Totals.SeedAssets, SeedUSD: resp.Totals.SeedUSD, SinkUSD: resp.Totals.SinkUSD, FrontierUSD: resp.Totals.FrontierUSD,
		Sinks: []endpointDigest{}, Frontier: []endpointDigest{}, Flows: []flowDigest{},
		CoverageGaps: append([]string{}, resp.CoverageGaps...), Warnings: append([]string{}, resp.Warnings...),
	}
	for i, e := range resp.Sinks {
		if i == digestEndpoints {
			d.TruncatedLists = true
			break
		}
		d.Sinks = append(d.Sinks, digestEndpoint(e))
	}
	for i, e := range resp.Frontier {
		if i == digestEndpoints {
			d.TruncatedLists = true
			break
		}
		d.Frontier = append(d.Frontier, digestEndpoint(e))
	}
	for _, edge := range resp.Edges {
		for _, tx := range edge.TracedTransactions {
			traced := fmt.Sprintf("%g %s", tx.TracedAmount, tx.Asset)
			if tx.InputAsset != "" {
				traced = fmt.Sprintf("%g %s → %s", tx.TracedInputAmount, tx.InputAsset, traced)
			}
			d.Flows = append(d.Flows, flowDigest{
				Time: tx.Time, From: labels[edge.From], To: labels[edge.To], Action: firstNonEmpty(edge.ActionLabel, edge.ActionClass),
				Traced: traced, USD: tx.TracedUSDAtTime, TxID: tx.TxID, InboundTxID: tx.InboundTxID,
				Confidence: tx.Confidence, Reason: tx.ConfidenceReason,
			})
		}
	}
	d.TotalFlows = len(d.Flows)
	sort.SliceStable(d.Flows, func(i, j int) bool { return d.Flows[i].USD > d.Flows[j].USD })
	if len(d.Flows) > digestFlows {
		d.Flows = d.Flows[:digestFlows]
		d.TruncatedLists = true
	}
	return d
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (t *toolset) traceFunds(ctx context.Context, _ *mcp.CallToolRequest, in traceFundsInput) (*mcp.CallToolResult, traceDigest, error) {
	req := app.TraceRequest{
		StartTime: in.StartTime, EndTime: in.EndTime, Direction: in.Direction, Policy: in.Policy,
		MaxDepth: in.MaxDepth, MaxBranches: in.MaxBranches, Amount: in.Amount, Asset: in.Asset,
		MinUSDAtTime: in.MinUSDAtTime, StopCategories: in.StopCategories, IncludeHoldings: in.IncludeHoldings,
	}
	for _, seed := range in.Seeds {
		seed = strings.TrimSpace(seed)
		switch {
		case seed == "":
		case txHashPattern.MatchString(seed):
			req.Seeds = append(req.Seeds, app.TraceSeed{TxID: seed})
		case strings.Contains(seed, "|"):
			chain, address, _ := strings.Cut(seed, "|")
			req.Seeds = append(req.Seeds, app.TraceSeed{Chain: strings.ToUpper(chain), Address: address})
		default:
			req.Seeds = append(req.Seeds, app.TraceSeed{Address: seed})
		}
	}
	if len(req.Seeds) == 0 {
		return nil, traceDigest{}, fmt.Errorf("seeds must contain at least one address or transaction hash")
	}
	resp, err := t.client.Trace(ctx, req, nil)
	if err != nil {
		return nil, traceDigest{}, err
	}
	return nil, digestTrace(resp), nil
}

type addressProfileInput struct {
	Address string `json:"address" jsonschema:"the address (CHAIN|address when the chain is ambiguous)"`
	Chain   string `json:"chain,omitempty" jsonschema:"chain of the address, such as ETH or BTC, when ambiguous"`
	Days    int    `json:"days,omitempty" jsonschema:"days of flows to summarise (default 30)"`
}

func (t *toolset) addressProfile(ctx context.Context, _ *mcp.CallToolRequest, in addressProfileInput) (*mcp.CallToolResult, app.AddressProfile, error) {
	profile, err := t.client.AddressProfile(ctx, app.AddressProfileRequest{Address: in.Address, Chain: in.Chain, Days: in.Days}, nil)
	if err != nil {
		return nil, app.AddressProfile{}, err
	}
	if len(profile.Flows.Counterparties) > digestCounterparties {
		profile.Flows.Counterparties = profile.Flows.Counterparties[:digestCounterparties]
	}
	return nil, profile, nil
}

type actorSummaryInput struct {
	Actor   string `json:"actor" jsonschema:"actor ID or name"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"record a new holdings and flows snapshot first (takes a few minutes)"`
}

type actorDigest struct {
	ActorID       int64                   `json:"actor_id"`
	Name          string                  `json:"name"`
	Addresses     int                     `json:"addresses"`
	Watch         bool                    `json:"watch"`
	LastViewedAt  string                  `json:"last_viewed_at,omitempty"`
	Series        []app.ActorSeriesPoint  `json:"holdings_series"`
	LatestUSD     float64                 `json:"latest_holdings_usd"`
	LatestAt      *time.Time              `json:"latest_at,omitempty"`
	SinceLastView *app.ActorChanges       `json:"since_last_view,omitempty"`
	TopHoldings   []app.ActorHolding      `json:"top_holdings"`
	NewSinceView  []app.ActorCounterparty `json:"new_counterparties"`
	AssetChanges  []app.ActorAssetDelta   `json:"asset_changes"`
	BiggestMoves  []app.ActorHoldingDelta `json:"address_changes"`
	Counterparts  []app.ActorCounterparty `json:"counterparties"`
}

func (t *toolset) actorSummary(ctx context.Context, _ *mcp.CallToolRequest, in actorSummaryInput) (*mcp.CallToolResult, actorDigest, error) {
	actor, err := t.client.ResolveActor(ctx, in.Actor)
	if err != nil {
		return nil, actorDigest{}, err
	}
	if in.Refresh {
		if _, err := t.client.ActorRefresh(ctx, actor.ID, nil); err != nil {
			return nil, actorDigest{}, fmt.Errorf("refresh %s: %w", actor.Name, err)
		}
	}
	monitor, err := t.client.ActorMonitor(ctx, actor.ID)
	if err != nil {
		return nil, actorDigest{}, err
	}
	d := actorDigest{
		ActorID: actor.ID, Name: actor.Name, Addresses: len(actor.Addresses), Watch: monitor.Watch, LastViewedAt: monitor.LastViewedAt,
		Series: monitor.Series, TopHoldings: []app.ActorHolding{}, NewSinceView: []app.ActorCounterparty{},
		AssetChanges: []app.ActorAssetDelta{}, BiggestMoves: []app.ActorHoldingDelta{}, Counterparts: []app.ActorCounterparty{},
	}
	if monitor.Latest != nil {
		d.LatestUSD = monitor.Latest.TotalUSD
		at := monitor.Latest.TakenAt
		d.LatestAt = &at
		for i, holding := range monitor.Latest.Holdings {
			if i == digestCounterparties {
				break
			}
			holding.Assets = nil
			d.TopHoldings = append(d.TopHoldings, holding)
		}
	}
	if changes := monitor.SinceLastView; changes != nil {
		limit := func(n int) int { return min(n, digestCounterparties) }
		d.NewSinceView = append(d.NewSinceView, changes.NewCounterparties[:limit(len(changes.NewCounterparties))]...)
		d.AssetChanges = append(d.AssetChanges, changes.AssetDeltas[:limit(len(changes.AssetDeltas))]...)
		d.BiggestMoves = append(d.BiggestMoves, changes.HoldingDeltas[:limit(len(changes.HoldingDeltas))]...)
		d.Counterparts = append(d.Counterparts, changes.Flows.Counterparties[:limit(len(changes.Flows.Counterparties))]...)
		summary := *changes
		summary.NewCounterparties, summary.AssetDeltas, summary.HoldingDeltas, summary.Flows.Counterparties = nil, nil, nil, nil
		d.SinceLastView = &summary
	}
	return nil, d, nil
}

type txLookupInput struct {
	TxID string `json:"tx_id" jsonschema:"transaction hash, with or without 0x"`
}

type legDigest struct {
	Address string   `json:"address"`
	TxID    string   `json:"tx_id,omitempty"`
	Coins   []string `json:"coins"`
}

type actionDigest struct {
	Protocol string      `json:"protocol"`
	Type     string      `json:"type"`
	Status   string      `json:"status"`
	Time     time.Time   `json:"time"`
	Height   string      `json:"height"`
	Pools    []string    `json:"pools,omitempty"`
	In       []legDigest `json:"in"`
	Out      []legDigest `json:"out"`
}

type txDigest struct {
	TxID    string         `json:"tx_id"`
	Actions []actionDigest `json:"actions"`
}

func (t *toolset) txLookup(ctx context.Context, _ *mcp.CallToolRequest, in txLookupInput) (*mcp.CallToolResult, txDigest, error) {
	txID := strings.TrimSpace(in.TxID)
	if txHashPattern.MatchString(txID) {
		txID = strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(txID, "0x"), "0X"))
	}
	result, err := t.client.LookupTx(ctx, txID)
	if err != nil {
		return nil, txDigest{}, err
	}
	d := txDigest{TxID: result.TxID, Actions: []actionDigest{}}
	for _, action := range result.Actions {
		item := actionDigest{Protocol: action.SourceProtocol, Type: action.Type, Status: action.Status, Height: action.Height, Pools: action.Pools, In: []legDigest{}, Out: []legDigest{}}
		if nanos, err := parseNanos(action.Date); err == nil {
			item.Time = nanos
		}
		for _, leg := range action.In {
			digest := legDigest{Address: leg.Address, TxID: leg.TxID, Coins: []string{}}
			for _, coin := range leg.Coins {
				digest.Coins = append(digest.Coins, formatCoin(coin.Asset, coin.Amount))
			}
			item.In = append(item.In, digest)
		}
		for _, leg := range action.Out {
			digest := legDigest{Address: leg.Address, TxID: leg.TxID, Coins: []string{}}
			for _, coin := range leg.Coins {
				digest.Coins = append(digest.Coins, formatCoin(coin.Asset, coin.Amount))
			}
			item.Out = append(item.Out, digest)
		}
		d.Actions = append(d.Actions, item)
	}
	return nil, d, nil
}

func parseNanos(raw string) (time.Time, error) {
	var nanos int64
	if _, err := fmt.Sscan(raw, &nanos); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, nanos).UTC(), nil
}

// formatCoin renders a Midgard coin (1e8 base units) as "1.5 ETH.ETH".
func formatCoin(asset, amount string) string {
	var units float64
	if _, err := fmt.Sscan(amount, &units); err != nil {
		return amount + " " + asset
	}
	return strconv.FormatFloat(units/1e8, 'f', -1, 64) + " " + asset
}

type caseListInput struct{}

type caseListOutput struct {
	Cases []app.Case `json:"cases"`
}

func (t *toolset) caseList(ctx context.Context, _ *mcp.CallToolRequest, _ caseListInput) (*mcp.CallToolResult, caseListOutput, error) {
	cases, err := t.client.Cases(ctx)
	if err != nil {
		return nil, caseListOutput{}, err
	}
	return nil, caseListOutput{Cases: cases}, nil
}

type caseExportInput struct {
	CaseID int64  `json:"case_id" jsonschema:"the case ID (see case_list)"`
	Format string `json:"format,omitempty" jsonschema:"md (default) or csv"`
}

type caseExportOutput struct {
	CaseID  int64  `json:"case_id"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

func (t *toolset) caseExport(ctx context.Context, _ *mcp.CallToolRequest, in caseExportInput) (*mcp.CallToolResult, caseExportOutput, error) {
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "md"
	}
	data, err := t.client.CaseExport(ctx, in.CaseID, format)
	if err != nil {
		return nil, caseExportOutput{}, err
	}
	out := caseExportOutput{CaseID: in.CaseID, Format: format, Content: string(data)}
	// The report reads best as text; the structured copy stays for clients
	// that want fields.
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Content}}}, out, nil
}
