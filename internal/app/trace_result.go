package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// traceHoldingsBudget bounds the live-holdings lookup for trace endpoints.
const traceHoldingsBudget = 60 * time.Second

func traceUnitsToRaw(units float64) string {
	return strconv.FormatFloat(math.Round(units*1e8), 'f', 0, 64)
}

type traceAmountSet map[string]*TraceAmount

func (s traceAmountSet) add(asset string, amount, usd float64) {
	if amount <= traceEpsilon {
		return
	}
	entry := s[asset]
	if entry == nil {
		entry = &TraceAmount{Asset: asset}
		s[asset] = entry
	}
	entry.Amount += amount
	entry.USDAtTime += usd
}

func (s traceAmountSet) list() []TraceAmount {
	out := make([]TraceAmount, 0, len(s))
	for _, entry := range s {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USDAtTime != out[j].USDAtTime {
			return out[i].USDAtTime > out[j].USDAtTime
		}
		return out[i].Asset < out[j].Asset
	})
	return out
}

func (s traceAmountSet) usd() float64 {
	total := 0.0
	for _, entry := range s {
		total += entry.USDAtTime
	}
	return total
}

// deliveredUSD values what arrived at the recipient, falling back to the
// input side when the output asset has no price.
func (m *traceMovement) deliveredUSD() float64 {
	if m.deliverPriced {
		return m.tracedDeliver * m.deliverUSDUnit
	}
	if m.consumePriced {
		return m.tracedConsume * m.consumeUSDUnit
	}
	return 0
}

func (m *traceMovement) consumedUSD() float64 {
	if m.consumePriced {
		return m.tracedConsume * m.consumeUSDUnit
	}
	if m.deliverPriced {
		return m.tracedDeliver * m.deliverUSDUnit
	}
	return 0
}

func (t *tracer) result() TraceResponse {
	resp := TraceResponse{
		Query:        t.query,
		CoverageGaps: append([]string{}, t.gaps...),
		Warnings:     append([]string{}, t.warnings...),
	}
	movements := t.sortedMovements()
	traced := make([]*traceMovement, 0, len(movements))
	for _, m := range movements {
		if m.tracedDeliver > traceEpsilon {
			traced = append(traced, m)
		}
	}

	roles := map[string]string{}
	for _, key := range t.seeds {
		roles[key] = "seed"
	}
	resp.Edges = t.traceEdges(traced, roles)
	resp.Sinks, resp.Frontier = t.endpoints(traced, roles)

	seedAssets := traceAmountSet{}
	for _, m := range traced {
		if t.query.Direction == TraceBackward {
			if t.seedSet[m.to] {
				seedAssets.add(m.deliverAsset, m.tracedDeliver, m.deliveredUSD())
			}
		} else if t.seedSet[m.from] {
			seedAssets.add(m.consumeAsset, m.tracedConsume, m.consumedUSD())
		}
	}
	resp.Totals.SeedAssets = seedAssets.list()
	resp.Totals.SeedUSD = seedAssets.usd()
	for _, sink := range resp.Sinks {
		resp.Totals.SinkUSD += sink.TracedUSDAtTime
	}
	for _, frontier := range resp.Frontier {
		resp.Totals.FrontierUSD += frontier.TracedUSDAtTime
	}

	for key, role := range roles {
		node := t.nodes[key]
		if node == nil {
			continue
		}
		out := node.node
		if out.ID == "" {
			out = FlowNode{ID: key, Kind: "external_address", Label: shortAddress(node.address), Chain: node.chain, Metrics: map[string]any{"address": node.address}}
		}
		out.ID = key
		metrics := map[string]any{}
		for k, v := range out.Metrics {
			metrics[k] = v
		}
		metrics["trace_role"] = role
		metrics["trace_depth"] = node.depth
		out.Metrics = metrics
		out.Depth = max(node.depth, 0)
		resp.Nodes = append(resp.Nodes, out)
	}
	sort.Slice(resp.Nodes, func(i, j int) bool {
		if resp.Nodes[i].Depth != resp.Nodes[j].Depth {
			return resp.Nodes[i].Depth < resp.Nodes[j].Depth
		}
		return resp.Nodes[i].ID < resp.Nodes[j].ID
	})
	resp.Method = t.method()
	// Empty lists serialise as [] so clients need no null checks.
	if resp.Nodes == nil {
		resp.Nodes = []FlowNode{}
	}
	if resp.Edges == nil {
		resp.Edges = []TraceEdge{}
	}
	if resp.Sinks == nil {
		resp.Sinks = []TraceEndpoint{}
	}
	if resp.Frontier == nil {
		resp.Frontier = []TraceEndpoint{}
	}
	if resp.Totals.SeedAssets == nil {
		resp.Totals.SeedAssets = []TraceAmount{}
	}
	return resp
}

func (t *tracer) traceEdges(traced []*traceMovement, roles map[string]string) []TraceEdge {
	edges := map[string]*TraceEdge{}
	delivered := map[string]traceAmountSet{}
	inputs := map[string]traceAmountSet{}
	var order []string
	for _, m := range traced {
		id := m.from + "->" + m.to + "|" + m.actionKey
		edge := edges[id]
		if edge == nil {
			edge = &TraceEdge{FlowEdge: FlowEdge{
				ID:           id,
				From:         m.from,
				To:           m.to,
				ActionClass:  m.actionClass,
				ActionKey:    m.actionKey,
				ActionLabel:  m.actionLabel,
				ActionDomain: m.actionClass,
				Confidence:   m.confidence,
			}, ConfidenceReason: m.confidenceReason}
			edges[id] = edge
			delivered[id] = traceAmountSet{}
			inputs[id] = traceAmountSet{}
			order = append(order, id)
		}
		if m.confidence < edge.Confidence {
			edge.Confidence, edge.ConfidenceReason = m.confidence, m.confidenceReason
		}
		for _, key := range []string{m.from, m.to} {
			if roles[key] == "" {
				roles[key] = "intermediate"
			}
		}
		usd := m.consumedUSD()
		if !m.isSwap() {
			usd = m.deliveredUSD()
		}
		delivered[id].add(m.deliverAsset, m.tracedDeliver, m.deliveredUSD())
		if m.isSwap() {
			inputs[id].add(m.consumeAsset, m.tracedConsume, m.consumedUSD())
		}
		edge.TracedUSDAtTime += usd
		edge.TxIDs = appendUniqueString(edge.TxIDs, m.txID)
		if m.inboundTxID != "" {
			edge.TxIDs = appendUniqueString(edge.TxIDs, m.inboundTxID)
		}
		tx := TraceTransaction{
			TxID:             m.txID,
			InboundTxID:      m.inboundTxID,
			Time:             m.time,
			Height:           m.height,
			Asset:            m.deliverAsset,
			Amount:           m.deliverAmount,
			TracedAmount:     m.tracedDeliver,
			TracedUSDAtTime:  usd,
			Priced:           m.deliverPriced || m.consumePriced,
			Fraction:         m.tracedDeliver / m.deliverAmount,
			Confidence:       m.confidence,
			ConfidenceReason: m.confidenceReason,
		}
		flowTx := FlowEdgeTransaction{TxID: m.txID, InboundTxID: m.inboundTxID, Height: m.height, Time: m.time, USDAtTime: usd, USDSpot: usd}
		if m.isSwap() {
			tx.InputAsset, tx.InputAmount, tx.TracedInputAmount = m.consumeAsset, m.consumeAmount, m.tracedConsume
			flowTx.Assets = append(flowTx.Assets, FlowAssetValue{Asset: m.consumeAsset, AmountRaw: traceUnitsToRaw(m.tracedConsume), USDAtTime: m.consumedUSD(), USDSpot: m.consumedUSD(), Direction: "in"})
			flowTx.Assets = append(flowTx.Assets, FlowAssetValue{Asset: m.deliverAsset, AmountRaw: traceUnitsToRaw(m.tracedDeliver), USDAtTime: m.deliveredUSD(), USDSpot: m.deliveredUSD(), Direction: "out"})
		} else {
			flowTx.Assets = append(flowTx.Assets, FlowAssetValue{Asset: m.deliverAsset, AmountRaw: traceUnitsToRaw(m.tracedDeliver), USDAtTime: usd, USDSpot: usd})
		}
		edge.TracedTransactions = append(edge.TracedTransactions, tx)
		edge.Transactions = append(edge.Transactions, flowTx)
		if m.height > 0 {
			edge.Heights = append(edge.Heights, m.height)
		}
	}
	out := make([]TraceEdge, 0, len(order))
	for _, id := range order {
		edge := edges[id]
		edge.TracedAssets = delivered[id].list()
		edge.TracedInputAssets = inputs[id].list()
		edge.USDAtTime = edge.TracedUSDAtTime
		edge.USDSpot = edge.TracedUSDAtTime
		for _, asset := range edge.TracedInputAssets {
			edge.Assets = append(edge.Assets, FlowAssetValue{Asset: asset.Asset, AmountRaw: traceUnitsToRaw(asset.Amount), USDAtTime: asset.USDAtTime, USDSpot: asset.USDAtTime, Direction: "in"})
		}
		direction := ""
		if len(edge.TracedInputAssets) > 0 {
			direction = "out"
		}
		for _, asset := range edge.TracedAssets {
			edge.Assets = append(edge.Assets, FlowAssetValue{Asset: asset.Asset, AmountRaw: traceUnitsToRaw(asset.Amount), USDAtTime: asset.USDAtTime, USDSpot: asset.USDAtTime, Direction: direction})
		}
		from, to := t.nodes[edge.From], t.nodes[edge.To]
		if from != nil && to != nil {
			edge.Depth = max(from.depth, to.depth)
		}
		out = append(out, *edge)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].TracedUSDAtTime > out[j].TracedUSDAtTime
	})
	return out
}

// endpointReason says why traced value stopped at an address the trace did
// not expand, and whether that makes it a sink (true) or a frontier.
func (t *tracer) endpointReason(node *traceNode) (string, bool) {
	switch node.kind {
	case "pool":
		return "pool", true
	case "bond":
		return "bond", true
	case "contract":
		return "contract", true
	case "protocol":
		return "protocol", true
	}
	if node.category != "" && t.stop[node.category] {
		return node.category, true
	}
	switch {
	case node.depth >= t.query.MaxDepth:
		return "max_depth", false
	case t.branchCut[node.key]:
		return "max_branches", false
	case t.minCut[node.key]:
		return "below_min_usd", false
	case t.limitCut[node.key]:
		return "expansion_limit", false
	}
	return "unexpanded", false
}

func (t *tracer) endpoints(traced []*traceMovement, roles map[string]string) ([]TraceEndpoint, []TraceEndpoint) {
	type accumulator struct {
		assets        traceAmountSet
		conf          float64
		first, last   time.Time
		forcedReason  string
		forcedAsASink bool
	}
	acc := map[string]*accumulator{}
	get := func(key string) *accumulator {
		a := acc[key]
		if a == nil {
			a = &accumulator{assets: traceAmountSet{}, conf: 1}
			acc[key] = a
		}
		return a
	}
	touch := func(a *accumulator, m *traceMovement) {
		if m.confidence < a.conf {
			a.conf = m.confidence
		}
		if a.first.IsZero() || m.time.Before(a.first) {
			a.first = m.time
		}
		if m.time.After(a.last) {
			a.last = m.time
		}
	}

	if t.query.Direction == TraceBackward {
		for _, m := range traced {
			source := t.nodes[m.from]
			if source == nil || t.seedSet[m.from] {
				continue
			}
			if source.expanded && source.depth < t.query.MaxDepth {
				// Only the part spent from funds held before the window stops here.
				opening := 0.0
				consumed := 0.0
				for _, portion := range m.sources {
					consumed += portion.amount
					if portion.origin == "" {
						opening += portion.amount
					}
				}
				if opening <= traceEpsilon || consumed <= 0 {
					continue
				}
				share := m.tracedConsume * opening / consumed
				a := get(m.from)
				a.forcedReason, a.forcedAsASink = "held_before_window", true
				a.assets.add(m.consumeAsset, share, share*m.consumeUSDUnit)
				touch(a, m)
				continue
			}
			a := get(m.from)
			a.assets.add(m.consumeAsset, m.tracedConsume, m.consumedUSD())
			touch(a, m)
		}
	} else {
		for _, m := range traced {
			target := t.nodes[m.to]
			if target == nil || m.from == m.to || target.expanded {
				continue
			}
			a := get(m.to)
			a.assets.add(m.deliverAsset, m.tracedDeliver, m.deliveredUSD())
			touch(a, m)
		}
		// Expanded addresses still holding traced funds at the end are sinks.
		usdUnit := map[string]float64{}
		for _, m := range t.movements {
			if m.deliverPriced {
				usdUnit[m.id] = m.deliverUSDUnit
			}
		}
		for id, b := range t.forwardBooks {
			key, asset, _ := strings.Cut(id, "\x00")
			node := t.nodes[key]
			// A seed traced in full already counts every outflow, so funds
			// that loop back into it are not held anywhere new.
			if node == nil || !node.expanded || (t.fullSeed && t.seedSet[key]) {
				continue
			}
			for _, lot := range b.lots {
				if lot.tainted <= 1e-6 {
					continue
				}
				a := get(key)
				a.forcedReason, a.forcedAsASink = "held", true
				a.assets.add(asset, lot.tainted, lot.tainted*usdUnit[lot.origin])
				if lot.conf < a.conf {
					a.conf = lot.conf
				}
				if a.first.IsZero() || lot.at.Before(a.first) {
					a.first = lot.at
				}
				if lot.at.After(a.last) {
					a.last = lot.at
				}
			}
		}
	}

	var sinks, frontier []TraceEndpoint
	for key, a := range acc {
		node := t.nodes[key]
		if node == nil || len(a.assets) == 0 {
			continue
		}
		reason, sink := a.forcedReason, a.forcedAsASink
		if reason == "" {
			reason, sink = t.endpointReason(node)
		}
		role := "frontier"
		if sink {
			role = "sink"
		}
		if roles[key] == "" || roles[key] == "intermediate" {
			roles[key] = role
		}
		endpoint := TraceEndpoint{
			NodeID:          key,
			Chain:           node.chain,
			Address:         node.address,
			Label:           firstNonEmpty(node.node.Label, shortAddress(node.address), key),
			Kind:            node.kind,
			Category:        node.category,
			Reason:          reason,
			Depth:           max(node.depth, 0),
			Assets:          a.assets.list(),
			TracedUSDAtTime: a.assets.usd(),
			Confidence:      a.conf,
			FirstAt:         a.first,
			LastAt:          a.last,
		}
		if sink {
			sinks = append(sinks, endpoint)
		} else {
			frontier = append(frontier, endpoint)
		}
	}
	sortEndpoints := func(list []TraceEndpoint) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].TracedUSDAtTime != list[j].TracedUSDAtTime {
				return list[i].TracedUSDAtTime > list[j].TracedUSDAtTime
			}
			return list[i].NodeID < list[j].NodeID
		})
	}
	sortEndpoints(sinks)
	sortEndpoints(frontier)
	return sinks, frontier
}

func (t *tracer) method() []string {
	q := t.query
	verb := "Forward trace"
	if q.Direction == TraceBackward {
		verb = "Backward trace"
	}
	lines := []string{fmt.Sprintf("%s from %d seed address(es), %s to %s UTC.", verb, len(t.seeds), q.StartTime.UTC().Format("2006-01-02 15:04"), q.EndTime.UTC().Format("2006-01-02 15:04"))}
	switch {
	case t.fullSeed && q.Direction == TraceBackward:
		lines = append(lines, "Every inflow to the seed in the window is traced back in full.")
	case t.fullSeed:
		lines = append(lines, "Every outflow from the seed in the window is traced in full.")
	case q.Direction == TraceBackward:
		lines = append(lines, fmt.Sprintf("Explains %s %s held by the seed at the end of the window.", strconv.FormatFloat(q.Amount, 'f', -1, 64), q.Asset))
	default:
		lines = append(lines, fmt.Sprintf("Traces %s %s held by the seed at the start of the window.", strconv.FormatFloat(q.Amount, 'f', -1, 64), q.Asset))
	}
	switch q.Policy {
	case TracePolicyHaircut:
		lines = append(lines, "Haircut: each payment carries the address's traced share of its balance at that moment.")
	case TracePolicyLargestOut:
		if q.Direction == TraceBackward {
			lines = append(lines, "Largest first: each payment is attributed to the largest earlier receipts first.")
		} else {
			lines = append(lines, "Largest out: traced funds are assigned to the largest later payments first.")
		}
	default:
		lines = append(lines, "FIFO: each payment spends the oldest funds received first.")
	}
	lines = append(lines,
		"Swaps carry the traced share of the input into the output asset.",
		"Funds held before the window are treated as untraced; mixing traced with untraced funds lowers confidence.",
		fmt.Sprintf("Stops at %s labels, liquidity pools, validator bonds and protocol contracts; follows up to %d hops and %d branches per address.", strings.Join(q.StopCategories, ", "), q.MaxDepth, q.MaxBranches),
	)
	if q.MinUSDAtTime > 0 {
		lines = append(lines, fmt.Sprintf("Branches under $%.0f at transaction time are not followed.", q.MinUSDAtTime))
	}
	return lines
}

// attachHoldings looks up what each address endpoint holds now.
func (t *tracer) attachHoldings(ctx context.Context, resp *TraceResponse) {
	var nodes []FlowNode
	index := map[string][]*TraceEndpoint{}
	for _, list := range [][]TraceEndpoint{resp.Sinks, resp.Frontier} {
		for i := range list {
			endpoint := &list[i]
			if endpoint.Kind != "address" || endpoint.Address == "" {
				continue
			}
			if _, ok := index[endpoint.NodeID]; !ok {
				nodes = append(nodes, FlowNode{ID: endpoint.NodeID, Kind: "external_address", Chain: endpoint.Chain, Label: endpoint.Label, Metrics: map[string]any{"address": endpoint.Address}})
			}
			index[endpoint.NodeID] = append(index[endpoint.NodeID], endpoint)
		}
	}
	if len(nodes) == 0 {
		return
	}
	lookupCtx, cancel := context.WithTimeout(ctx, traceHoldingsBudget)
	defer cancel()
	result, err := t.app.runLiveHoldingsJob(lookupCtx, nodes, false)
	if err != nil {
		resp.Warnings = append(resp.Warnings, "holdings lookup failed: "+err.Error())
		return
	}
	for _, update := range result.Nodes {
		status := getString(update.Metrics, "live_holdings_status")
		usd, ok := update.Metrics["live_holdings_usd_spot"].(float64)
		for _, endpoint := range index[update.ID] {
			endpoint.HoldingsStatus = status
			if ok && status == "available" {
				value := usd
				endpoint.HoldingsUSD = &value
			}
		}
	}
}
