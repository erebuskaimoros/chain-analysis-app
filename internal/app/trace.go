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

// Follow-the-funds tracing. A trace starts at seed addresses (or at the sender
// of a THORChain action) and follows value hop by hop through the same
// projected flows the graphs use, so a swap arrives as one wallet→recipient
// movement with an asset conversion.
//
// Forward traces run a chronological simulation: every expanded address keeps
// per-asset lots of received funds, each with a traced share, and an
// allocation policy decides which outflows carry it:
//
//	fifo         outflows consume the oldest received funds first
//	haircut      every outflow carries the address's traced share at that time
//	largest_out  traced value goes to the largest outflows first
//
// Backward traces decompose each outflow into the inflows it spent (oldest
// first, proportionally, or largest first) and walk that provenance upstream.
//
// Funds held before the trace window are unknown and treated as untraced.

const (
	JobTrace JobKind = "trace"

	TraceForward  = "forward"
	TraceBackward = "backward"

	TracePolicyFIFO       = "fifo"
	TracePolicyHaircut    = "haircut"
	TracePolicyLargestOut = "largest_out"

	traceDefaultMaxDepth    = 3
	traceMaxDepthLimit      = 8
	traceDefaultMaxBranches = 8
	traceMaxBranchesLimit   = 64
	traceMaxExpansions      = 160
	traceDefaultWindow      = 30 * 24 * time.Hour
	traceMixingFactor       = 0.85
	traceEpsilon            = 1e-9
	traceSeedReason         = "seed address"
)

var traceDefaultStopCategories = []string{"exchange", "sanctioned", "mixer"}

type TraceRequest struct {
	Seeds           []TraceSeed `json:"seeds"`
	StartTime       string      `json:"start_time"`
	EndTime         string      `json:"end_time,omitempty"`
	Amount          float64     `json:"amount,omitempty"`
	Asset           string      `json:"asset,omitempty"`
	Direction       string      `json:"direction,omitempty"`
	Policy          string      `json:"policy,omitempty"`
	MaxDepth        int         `json:"max_depth,omitempty"`
	MaxBranches     int         `json:"max_branches,omitempty"`
	MinUSDAtTime    float64     `json:"min_usd_at_time,omitempty"`
	StopCategories  []string    `json:"stop_categories,omitempty"`
	FlowTypes       []string    `json:"flow_types,omitempty"`
	IncludeHoldings bool        `json:"include_holdings,omitempty"`
}

// TraceSeed is an address (with an optional chain) or a THORChain action tx ID,
// whose sender becomes the seed.
type TraceSeed struct {
	Chain   string `json:"chain,omitempty"`
	Address string `json:"address,omitempty"`
	TxID    string `json:"tx_id,omitempty"`
}

type TraceQuery struct {
	Seeds          []TraceSeed `json:"seeds"`
	StartTime      time.Time   `json:"start_time"`
	EndTime        time.Time   `json:"end_time"`
	Amount         float64     `json:"amount,omitempty"`
	Asset          string      `json:"asset,omitempty"`
	Direction      string      `json:"direction"`
	Policy         string      `json:"policy"`
	MaxDepth       int         `json:"max_depth"`
	MaxBranches    int         `json:"max_branches"`
	MinUSDAtTime   float64     `json:"min_usd_at_time"`
	StopCategories []string    `json:"stop_categories"`
	FlowTypes      []string    `json:"flow_types"`
}

type TraceAmount struct {
	Asset     string  `json:"asset"`
	Amount    float64 `json:"amount"`
	USDAtTime float64 `json:"usd_at_time"`
}

// TraceTransaction is one traced movement on an edge.
type TraceTransaction struct {
	TxID              string    `json:"tx_id"`
	InboundTxID       string    `json:"inbound_tx_id,omitempty"`
	Time              time.Time `json:"time"`
	Height            int64     `json:"height,omitempty"`
	Asset             string    `json:"asset"`
	Amount            float64   `json:"amount"`
	TracedAmount      float64   `json:"traced_amount"`
	InputAsset        string    `json:"input_asset,omitempty"`
	InputAmount       float64   `json:"input_amount,omitempty"`
	TracedInputAmount float64   `json:"traced_input_amount,omitempty"`
	TracedUSDAtTime   float64   `json:"traced_usd_at_time"`
	Priced            bool      `json:"priced"`
	Fraction          float64   `json:"fraction"`
	Confidence        float64   `json:"confidence"`
	ConfidenceReason  string    `json:"confidence_reason,omitempty"`
}

// TraceEdge is a graph edge carrying traced value. The embedded FlowEdge
// holds the traced amounts (assets, usd_at_time, transactions) so the graph
// canvas can draw it like any other edge.
type TraceEdge struct {
	FlowEdge
	Depth              int                `json:"depth"`
	TracedAssets       []TraceAmount      `json:"traced_assets"`
	TracedInputAssets  []TraceAmount      `json:"traced_input_assets,omitempty"`
	TracedUSDAtTime    float64            `json:"traced_usd_at_time"`
	ConfidenceReason   string             `json:"confidence_reason,omitempty"`
	TracedTransactions []TraceTransaction `json:"traced_transactions"`
}

// TraceEndpoint is where traced value stopped: a sink (labelled entity, pool,
// bond, or an address still holding it) or a frontier left by a limit.
type TraceEndpoint struct {
	NodeID          string        `json:"node_id"`
	Chain           string        `json:"chain,omitempty"`
	Address         string        `json:"address,omitempty"`
	Label           string        `json:"label"`
	Kind            string        `json:"kind"`
	Category        string        `json:"category,omitempty"`
	Reason          string        `json:"reason"`
	Depth           int           `json:"depth"`
	Assets          []TraceAmount `json:"assets"`
	TracedUSDAtTime float64       `json:"traced_usd_at_time"`
	Confidence      float64       `json:"confidence"`
	FirstAt         time.Time     `json:"first_at"`
	LastAt          time.Time     `json:"last_at"`
	HoldingsUSD     *float64      `json:"holdings_usd,omitempty"`
	HoldingsStatus  string        `json:"holdings_status,omitempty"`
}

type TraceTotals struct {
	SeedAssets  []TraceAmount `json:"seed_assets"`
	SeedUSD     float64       `json:"seed_usd"`
	SinkUSD     float64       `json:"sink_usd"`
	FrontierUSD float64       `json:"frontier_usd"`
}

type TraceResponse struct {
	RunID        int64           `json:"run_id,omitempty"`
	Query        TraceQuery      `json:"query"`
	Nodes        []FlowNode      `json:"nodes"`
	Edges        []TraceEdge     `json:"edges"`
	Sinks        []TraceEndpoint `json:"sinks"`
	Frontier     []TraceEndpoint `json:"frontier"`
	CoverageGaps []string        `json:"coverage_gaps"`
	Warnings     []string        `json:"warnings"`
	Method       []string        `json:"method"`
	Totals       TraceTotals     `json:"totals"`
	Stats        map[string]any  `json:"stats"`
}

// traceMovement is one asset moving between two addresses in one
// transaction. Swaps consume one asset at the sender and deliver another.
type traceMovement struct {
	id               string
	txID             string
	inboundTxID      string
	time             time.Time
	height           int64
	from, to         string
	actionClass      string
	actionKey        string
	actionLabel      string
	edgeConfidence   float64
	edgeReason       string
	consumeAsset     string
	consumeAmount    float64
	consumeUSDUnit   float64
	consumePriced    bool
	deliverAsset     string
	deliverAmount    float64
	deliverUSDUnit   float64
	deliverPriced    bool
	tracedConsume    float64
	tracedDeliver    float64
	confidence       float64
	confidenceReason string
	// sources holds the backward decomposition: which inflows (by movement
	// ID, "" for funds held before the window) this outflow spent.
	sources []traceSourcePortion
}

type traceSourcePortion struct {
	origin string
	amount float64
}

func (m *traceMovement) isSwap() bool { return m.consumeAsset != m.deliverAsset }

func (m *traceMovement) tracedUSD() float64 {
	if m.isSwap() && m.consumePriced {
		return m.tracedConsume * m.consumeUSDUnit
	}
	return m.tracedDeliver * m.deliverUSDUnit
}

type traceNode struct {
	key      string
	node     FlowNode
	kind     string // address, pool, bond, contract, protocol
	chain    string
	address  string
	category string
	expanded bool
	depth    int
}

type traceLot struct {
	origin  string
	at      time.Time
	amount  float64
	tainted float64
	conf    float64
	reason  string
}

type traceBook struct {
	lots []*traceLot
}

func (b *traceBook) total() (amount, tainted float64) {
	for _, lot := range b.lots {
		amount += lot.amount
		tainted += lot.tainted
	}
	return amount, tainted
}

func (b *traceBook) compact() {
	kept := b.lots[:0]
	for _, lot := range b.lots {
		if lot.amount > traceEpsilon {
			kept = append(kept, lot)
		}
	}
	b.lots = kept
}

type tracer struct {
	app        *App
	query      TraceQuery
	stop       map[string]bool
	nodes      map[string]*traceNode
	movements  map[string]*traceMovement
	seeds      []string
	seedSet    map[string]bool
	fullSeed   bool
	warnings   []string
	gaps       []string
	expansions int
	branchCut  map[string]bool
	minCut     map[string]bool
	limitCut   map[string]bool
	// forwardBooks holds each address's lots after a forward simulation, so
	// funds still held at the end become sinks.
	forwardBooks map[string]*traceBook
	arrivals     map[string]string
}

// Trace follows funds from the request's seeds.
func (a *App) Trace(ctx context.Context, req TraceRequest) (TraceResponse, error) {
	started := time.Now()
	query, err := normalizeTraceRequest(req)
	if err != nil {
		return TraceResponse{}, err
	}
	t := &tracer{
		app:       a,
		query:     query,
		stop:      map[string]bool{},
		nodes:     map[string]*traceNode{},
		movements: map[string]*traceMovement{},
		seedSet:   map[string]bool{},
		fullSeed:  query.Amount <= 0,
		branchCut: map[string]bool{},
		minCut:    map[string]bool{},
		limitCut:  map[string]bool{},
	}
	for _, category := range query.StopCategories {
		t.stop[strings.ToLower(category)] = true
	}
	if err := t.resolveSeeds(ctx); err != nil {
		return TraceResponse{}, err
	}
	progress := buildProgressFromContext(ctx)

	pending := append([]string{}, t.seeds...)
	for round := 0; len(pending) > 0; round++ {
		progress.set("tracing", round, query.MaxDepth+1, fmt.Sprintf("expanding %d addresses", len(pending)))
		if err := t.expand(ctx, pending); err != nil {
			return TraceResponse{}, err
		}
		t.simulate()
		pending = t.nextExpansions()
		if ctx.Err() != nil {
			return TraceResponse{}, ctx.Err()
		}
	}

	resp := t.result()
	if req.IncludeHoldings {
		progress.set("holdings", 0, 1, "looking up holdings at the endpoints")
		t.attachHoldings(ctx, &resp)
	}
	resp.Stats = map[string]any{
		"expanded_addresses": t.expansions,
		"movements":          len(t.movements),
		"elapsed_ms":         time.Since(started).Milliseconds(),
	}
	logInfo(ctx, "trace_completed", map[string]any{
		"direction":  query.Direction,
		"policy":     query.Policy,
		"seeds":      len(t.seeds),
		"edges":      len(resp.Edges),
		"sinks":      len(resp.Sinks),
		"frontier":   len(resp.Frontier),
		"expanded":   t.expansions,
		"elapsed_ms": time.Since(started).Milliseconds(),
	})
	return resp, nil
}

func normalizeTraceRequest(req TraceRequest) (TraceQuery, error) {
	query := TraceQuery{
		Seeds:        req.Seeds,
		Amount:       req.Amount,
		Asset:        normalizeAsset(req.Asset),
		Direction:    strings.ToLower(strings.TrimSpace(req.Direction)),
		Policy:       strings.ToLower(strings.TrimSpace(req.Policy)),
		MaxDepth:     req.MaxDepth,
		MaxBranches:  req.MaxBranches,
		MinUSDAtTime: math.Max(0, req.MinUSDAtTime),
		FlowTypes:    req.FlowTypes,
	}
	if len(query.Seeds) == 0 {
		return query, fmt.Errorf("at least one seed address or tx_id is required")
	}
	if query.Direction == "" {
		query.Direction = TraceForward
	}
	if query.Direction != TraceForward && query.Direction != TraceBackward {
		return query, fmt.Errorf("direction must be forward or backward")
	}
	if query.Policy == "" {
		query.Policy = TracePolicyFIFO
	}
	switch query.Policy {
	case TracePolicyFIFO, TracePolicyHaircut, TracePolicyLargestOut:
	default:
		return query, fmt.Errorf("policy must be fifo, haircut or largest_out")
	}
	if query.MaxDepth <= 0 {
		query.MaxDepth = traceDefaultMaxDepth
	}
	query.MaxDepth = min(query.MaxDepth, traceMaxDepthLimit)
	if query.MaxBranches <= 0 {
		query.MaxBranches = traceDefaultMaxBranches
	}
	query.MaxBranches = min(query.MaxBranches, traceMaxBranchesLimit)
	if query.Amount < 0 || math.IsNaN(query.Amount) || math.IsInf(query.Amount, 0) {
		return query, fmt.Errorf("amount must be a positive number")
	}
	if query.Amount > 0 {
		if query.Asset == "" {
			return query, fmt.Errorf("asset is required with amount")
		}
		if len(query.Seeds) != 1 {
			return query, fmt.Errorf("amount applies to a single seed")
		}
	}
	if req.StopCategories == nil {
		query.StopCategories = append([]string{}, traceDefaultStopCategories...)
	} else {
		for _, category := range req.StopCategories {
			if category = strings.ToLower(strings.TrimSpace(category)); category != "" {
				query.StopCategories = append(query.StopCategories, category)
			}
		}
	}
	if len(query.FlowTypes) == 0 {
		query.FlowTypes = []string{"liquidity", "swaps", "bonds", "transfers"}
	}
	if strings.TrimSpace(req.StartTime) != "" {
		start, err := parseActorTrackerTime(req.StartTime)
		if err != nil {
			return query, fmt.Errorf("invalid start_time: %w", err)
		}
		query.StartTime = start
	}
	if strings.TrimSpace(req.EndTime) != "" {
		end, err := parseActorTrackerTime(req.EndTime)
		if err != nil {
			return query, fmt.Errorf("invalid end_time: %w", err)
		}
		query.EndTime = end
	}
	return query, nil
}

// resolveSeeds turns seeds into address keys. A tx_id seed uses the action's
// sender, and when no start time was given, the action's time.
func (t *tracer) resolveSeeds(ctx context.Context) error {
	var resolved []TraceSeed
	for _, seed := range t.query.Seeds {
		if txID := cleanTxID(seed.TxID); txID != "" {
			lookup, err := t.app.lookupActionByTxID(ctx, txID)
			if err != nil {
				return fmt.Errorf("look up %s: %w", txID, err)
			}
			if len(lookup.Actions) == 0 {
				return fmt.Errorf("no THORChain action found for %s", txID)
			}
			action := lookup.Actions[len(lookup.Actions)-1].midgardAction
			if len(action.In) == 0 || strings.TrimSpace(action.In[0].Address) == "" {
				return fmt.Errorf("action %s has no sender", txID)
			}
			sender := action.In[0].Address
			at := parseMidgardActionTime(action.Date)
			// A transaction seed follows that transaction's own deposit: the
			// window starts (forward) or ends (backward) just before it.
			if t.query.Direction == TraceBackward {
				if t.query.EndTime.IsZero() {
					t.query.EndTime = at.Add(-time.Second)
				}
				if t.query.StartTime.IsZero() {
					t.query.StartTime = t.query.EndTime.Add(-traceDefaultWindow)
				}
			} else if t.query.StartTime.IsZero() {
				t.query.StartTime = at.Add(-time.Second)
			}
			chain := ""
			if len(action.In[0].Coins) > 0 {
				coin := action.In[0].Coins[0]
				chain = chainFromAsset(coin.Asset)
				if len(t.query.Seeds) == 1 && t.query.Amount <= 0 {
					if units := traceAssetUnits(coin.Amount); units > 0 {
						t.query.Amount, t.query.Asset, t.fullSeed = units, normalizeAsset(coin.Asset), false
					}
				}
			}
			resolved = append(resolved, TraceSeed{Chain: chain, Address: sender, TxID: txID})
			continue
		}
		if strings.TrimSpace(seed.Address) == "" {
			return fmt.Errorf("each seed needs an address or tx_id")
		}
		resolved = append(resolved, seed)
	}
	if t.query.StartTime.IsZero() {
		return fmt.Errorf("start_time is required for address seeds")
	}
	if t.query.EndTime.IsZero() {
		t.query.EndTime = t.query.StartTime.Add(traceDefaultWindow)
		if now := time.Now().UTC(); t.query.EndTime.After(now) {
			t.query.EndTime = now
		}
	}
	if !t.query.EndTime.After(t.query.StartTime) {
		return fmt.Errorf("end_time must be after start_time")
	}
	t.query.Seeds = resolved
	for _, seed := range resolved {
		raw := seed.Address
		if chain := strings.ToUpper(strings.TrimSpace(seed.Chain)); chain != "" {
			raw = chain + "|" + raw
		}
		frontier := normalizeFrontierAddress(raw)
		if frontier.Address == "" {
			return fmt.Errorf("unrecognised seed address %q", seed.Address)
		}
		key := frontierKey(frontier.Chain, frontier.Address)
		if t.seedSet[key] {
			continue
		}
		t.seedSet[key] = true
		t.seeds = append(t.seeds, key)
		t.nodes[key] = &traceNode{
			key:     key,
			kind:    "address",
			chain:   normalizeChain(frontier.Chain, frontier.Address),
			address: frontier.Address,
			depth:   0,
			node: FlowNode{
				ID:      key,
				Kind:    "external_address",
				Label:   shortAddress(frontier.Address),
				Chain:   normalizeChain(frontier.Chain, frontier.Address),
				Metrics: map[string]any{"address": frontier.Address},
			},
		}
	}
	return nil
}

// expand fetches one hop of flows around each address and records them as
// movements.
func (t *tracer) expand(ctx context.Context, keys []string) error {
	var addresses []string
	for _, key := range keys {
		node := t.nodes[key]
		if node == nil || node.expanded {
			continue
		}
		node.expanded = true
		t.expansions++
		addresses = append(addresses, encodeFrontierAddress(frontierAddress{Address: node.address, Chain: node.chain}))
	}
	for start := 0; start < len(addresses); start += actorTrackerExpandAddrCap {
		chunk := addresses[start:min(start+actorTrackerExpandAddrCap, len(addresses))]
		resp, err := t.app.expandActorTrackerOneHop(withoutBuildLiveHoldings(ctx), ActorTrackerExpandRequest{
			Addresses:       chunk,
			StartTime:       t.query.StartTime.Format(time.RFC3339),
			EndTime:         t.query.EndTime.Format(time.RFC3339),
			FlowTypes:       t.query.FlowTypes,
			IncludeUnpriced: true,
		})
		if err != nil {
			return fmt.Errorf("expand %d addresses: %w", len(chunk), err)
		}
		t.ingest(resp)
	}
	return nil
}

func traceCoverageWarning(warning string) bool {
	lower := strings.ToLower(warning)
	for _, marker := range []string{"truncated", "failed", "unavailable", "skipped", "backing off", "deferred"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (t *tracer) ingest(resp ActorTrackerResponse) {
	for _, warning := range resp.Warnings {
		if traceCoverageWarning(warning) {
			t.gaps = appendUniqueString(t.gaps, warning)
		} else {
			t.warnings = appendUniqueString(t.warnings, warning)
		}
	}
	keyByID := map[string]string{}
	for _, node := range resp.Nodes {
		key, kind, chain, address := traceNodeIdentity(node)
		if key == "" {
			continue
		}
		keyByID[node.ID] = key
		existing := t.nodes[key]
		if existing == nil {
			existing = &traceNode{key: key, kind: kind, chain: chain, address: address, depth: -1}
			t.nodes[key] = existing
		}
		if existing.node.ID == "" || (existing.node.Kind == "external_address" && node.Kind != "external_address") || strings.HasPrefix(existing.node.Label, shortAddress(existing.address)) {
			copyNode := node
			copyNode.Metrics = map[string]any{}
			for k, v := range node.Metrics {
				copyNode.Metrics[k] = v
			}
			copyNode.ID = key
			existing.node = copyNode
		}
		if category := getString(node.Metrics, "label_category"); category != "" {
			existing.category = strings.ToLower(category)
		}
	}
	for _, edge := range resp.Edges {
		if edge.ActionClass == "ownership" {
			continue
		}
		from, to := keyByID[edge.From], keyByID[edge.To]
		if from == "" || to == "" || from == to && edge.ActionClass != "swaps" {
			continue
		}
		for _, tx := range edge.Transactions {
			for _, movement := range traceMovementsForTransaction(edge, tx, from, to) {
				t.addMovement(movement)
			}
		}
	}
}

// addMovement records a movement once. The same payout can arrive both as a
// swap's output and as a plain transfer from a vault the protocol directory
// no longer lists; the swap, which knows the real sender, wins.
func (t *tracer) addMovement(m *traceMovement) {
	if _, ok := t.movements[m.id]; ok {
		return
	}
	if t.arrivals == nil {
		t.arrivals = map[string]string{}
	}
	arrival := strings.Join([]string{m.txID, m.to, m.deliverAsset, strconv.FormatFloat(m.deliverAmount, 'f', 8, 64)}, "|")
	if existingID, ok := t.arrivals[arrival]; ok && m.txID != "" {
		existing := t.movements[existingID]
		if existing == nil || existing.from == m.from || existing.actionClass == "swaps" || m.actionClass != "swaps" {
			return
		}
		delete(t.movements, existingID)
	}
	t.arrivals[arrival] = m.id
	t.movements[m.id] = m
}

// traceNodeIdentity keys a graph node by what it is, so the same address seen
// from different hops (actor address, external address) merges.
func traceNodeIdentity(node FlowNode) (key, kind, chain, address string) {
	address = normalizeAddress(getString(node.Metrics, "address"))
	switch node.Kind {
	case "actor":
		return "", "", "", ""
	case "pool":
		pool := normalizeAsset(firstNonEmpty(getString(node.Metrics, "pool"), getString(node.Metrics, "asset"), node.Label))
		return "pool:" + pool, "pool", node.Chain, ""
	case "node":
		return "node:" + address, "bond", "THOR", address
	case "contract_address":
		return "contract:" + address, "contract", node.Chain, address
	case "actor_address", "external_address", "bond_address", "explorer_target":
		if address == "" {
			return "", "", "", ""
		}
		chain = normalizeChain(node.Chain, address)
		return frontierKey(chain, address), "address", chain, address
	default:
		if address == "" {
			return "", "", "", ""
		}
		return node.Kind + ":" + address, "protocol", node.Chain, address
	}
}

func traceAssetUnits(amountRaw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(amountRaw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value / 1e8
}

func traceUSDUnit(asset FlowAssetValue) (float64, bool) {
	units := traceAssetUnits(asset.AmountRaw)
	if units <= 0 || asset.USDAtTime <= 0 || asset.PriceSource == "" {
		return 0, false
	}
	return asset.USDAtTime / units, true
}

func traceMovementsForTransaction(edge FlowEdge, tx FlowEdgeTransaction, from, to string) []*traceMovement {
	confidence := edge.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1
	}
	base := traceMovement{
		txID:           cleanTxID(tx.TxID),
		inboundTxID:    cleanTxID(tx.InboundTxID),
		time:           tx.Time,
		height:         tx.Height,
		from:           from,
		to:             to,
		actionClass:    edge.ActionClass,
		actionKey:      edge.ActionKey,
		actionLabel:    edge.ActionLabel,
		edgeConfidence: confidence,
		edgeReason:     firstNonEmpty(edge.ConfidenceReason, fmt.Sprintf("%s edge confidence %.2f", edge.ActionLabel, confidence)),
	}
	var inputs, outputs []FlowAssetValue
	for _, asset := range tx.Assets {
		switch asset.Direction {
		case "in":
			inputs = append(inputs, asset)
		default:
			outputs = append(outputs, asset)
		}
	}
	var out []*traceMovement
	add := func(consume, deliver FlowAssetValue, consumeShare float64, index int) {
		m := base
		m.consumeAsset = normalizeAsset(consume.Asset)
		m.consumeAmount = traceAssetUnits(consume.AmountRaw) * consumeShare
		m.consumeUSDUnit, m.consumePriced = traceUSDUnit(consume)
		m.deliverAsset = normalizeAsset(deliver.Asset)
		m.deliverAmount = traceAssetUnits(deliver.AmountRaw)
		m.deliverUSDUnit, m.deliverPriced = traceUSDUnit(deliver)
		if m.deliverAmount <= 0 || m.consumeAmount <= 0 {
			return
		}
		m.id = strings.Join([]string{m.txID, from, to, m.deliverAsset, deliver.AmountRaw, strconv.Itoa(index)}, "|")
		out = append(out, &m)
	}
	if edge.ActionClass == "swaps" && len(inputs) > 0 && len(outputs) > 0 {
		// One input feeds every output in proportion to the output's value.
		weights := make([]float64, len(outputs))
		totalWeight := 0.0
		for i, output := range outputs {
			weights[i] = output.USDAtTime
			totalWeight += weights[i]
		}
		for i, output := range outputs {
			share := 1 / float64(len(outputs))
			if totalWeight > 0 {
				share = weights[i] / totalWeight
			}
			add(inputs[0], output, share, i)
		}
		return out
	}
	for i, asset := range append(outputs, inputs...) {
		add(asset, asset, 1, i)
	}
	return out
}

func (t *tracer) sortedMovements() []*traceMovement {
	out := make([]*traceMovement, 0, len(t.movements))
	for _, m := range t.movements {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].time.Equal(out[j].time) {
			return out[i].time.Before(out[j].time)
		}
		return out[i].id < out[j].id
	})
	return out
}

func (t *tracer) expandedNode(key string) bool {
	node := t.nodes[key]
	return node != nil && node.expanded
}

func (t *tracer) simulate() {
	for _, node := range t.nodes {
		node.depth = -1
	}
	for _, key := range t.seeds {
		t.nodes[key].depth = 0
	}
	movements := t.sortedMovements()
	for _, m := range movements {
		m.tracedConsume, m.tracedDeliver, m.confidence, m.confidenceReason, m.sources = 0, 0, 0, "", nil
	}
	if t.query.Direction == TraceBackward {
		t.simulateBackward(movements)
	} else {
		t.simulateForward(movements)
	}
}

func (t *tracer) inWindow(m *traceMovement) bool {
	return !m.time.Before(t.query.StartTime) && !m.time.After(t.query.EndTime)
}

func (t *tracer) simulateForward(movements []*traceMovement) {
	books := map[string]*traceBook{}
	book := func(key, asset string) *traceBook {
		id := key + "\x00" + asset
		if books[id] == nil {
			books[id] = &traceBook{}
		}
		return books[id]
	}
	if !t.fullSeed {
		book(t.seeds[0], t.query.Asset).lots = append(book(t.seeds[0], t.query.Asset).lots, &traceLot{
			origin: "seed", at: t.query.StartTime, amount: t.query.Amount, tainted: t.query.Amount, conf: 1, reason: traceSeedReason,
		})
	}
	outflows := map[string][]*traceMovement{}
	for _, m := range movements {
		if t.inWindow(m) {
			id := m.from + "\x00" + m.consumeAsset
			outflows[id] = append(outflows[id], m)
		}
	}
	processed := map[string]bool{}
	for _, m := range movements {
		if !t.inWindow(m) {
			continue
		}
		var fraction, conf float64
		var reason string
		switch {
		case t.fullSeed && t.seedSet[m.from]:
			fraction, conf, reason = 1, 1, traceSeedReason
		case t.expandedNode(m.from):
			var future []*traceMovement
			for _, other := range outflows[m.from+"\x00"+m.consumeAsset] {
				if !processed[other.id] {
					future = append(future, other)
				}
			}
			fraction, conf, reason = t.consume(book(m.from, m.consumeAsset), m, future)
		}
		processed[m.id] = true
		if fraction > traceEpsilon && m.edgeConfidence < conf {
			conf, reason = m.edgeConfidence, m.edgeReason
		}
		m.tracedConsume = fraction * m.consumeAmount
		m.tracedDeliver = fraction * m.deliverAmount
		m.confidence, m.confidenceReason = conf, reason
		target := book(m.to, m.deliverAsset)
		target.lots = append(target.lots, &traceLot{origin: m.id, at: m.time, amount: m.deliverAmount, tainted: m.tracedDeliver, conf: conf, reason: reason})
		if m.tracedDeliver > traceEpsilon && m.from != m.to {
			if to := t.nodes[m.to]; to != nil && (to.depth < 0 || to.depth > t.nodes[m.from].depth+1) {
				to.depth = t.nodes[m.from].depth + 1
			}
		}
	}
	t.forwardBooks = books
}

// consume removes one outflow from an address's lots under the policy and
// returns the traced fraction of it.
func (t *tracer) consume(b *traceBook, m *traceMovement, future []*traceMovement) (float64, float64, string) {
	amount := m.consumeAmount
	total, tainted := b.total()
	if tainted <= traceEpsilon {
		takeLots(b, amount, 0, false)
		return 0, 0, ""
	}
	var taken float64
	switch t.query.Policy {
	case TracePolicyHaircut:
		taken = math.Min(tainted, amount*tainted/math.Max(total, amount))
	case TracePolicyLargestOut:
		taken = largestOutShare(tainted, m, future)
	default:
		taken = fifoTaintedShare(b, amount)
	}
	conf, reason := lotConfidence(b)
	takeLots(b, amount, taken, t.query.Policy == TracePolicyHaircut)
	fraction := math.Min(1, taken/amount)
	if fraction > traceEpsilon && fraction < 1-1e-6 {
		conf *= traceMixingFactor
		reason = fmt.Sprintf("mixed with untraced funds at %s; %s allocation", t.label(m.from), strings.ReplaceAll(t.query.Policy, "_", " "))
	}
	return fraction, conf, reason
}

func fifoTaintedShare(b *traceBook, amount float64) float64 {
	remaining, taken := amount, 0.0
	for _, lot := range b.lots {
		if remaining <= traceEpsilon {
			break
		}
		part := math.Min(remaining, lot.amount)
		if lot.amount > 0 {
			taken += part * lot.tainted / lot.amount
		}
		remaining -= part
	}
	return taken
}

// largestOutShare assigns the tainted balance to the remaining outflows in
// descending size and returns this outflow's share.
func largestOutShare(tainted float64, m *traceMovement, future []*traceMovement) float64 {
	ordered := append([]*traceMovement{}, future...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].consumeAmount != ordered[j].consumeAmount {
			return ordered[i].consumeAmount > ordered[j].consumeAmount
		}
		return ordered[i].time.Before(ordered[j].time)
	})
	left := tainted
	for _, other := range ordered {
		share := math.Min(left, other.consumeAmount)
		if other.id == m.id {
			return share
		}
		left -= share
		if left <= traceEpsilon {
			return 0
		}
	}
	return 0
}

// takeLots removes amount from the lots, of which taken is traced. FIFO takes
// from the oldest lots; haircut takes from every lot in proportion.
func takeLots(b *traceBook, amount, taken float64, proportional bool) {
	total, tainted := b.total()
	if proportional && total > traceEpsilon {
		ratio := math.Min(1, amount/total)
		taintRatio := 0.0
		if tainted > traceEpsilon {
			taintRatio = math.Min(1, taken/tainted)
		}
		for _, lot := range b.lots {
			lot.amount -= lot.amount * ratio
			lot.tainted -= lot.tainted * taintRatio
			lot.tainted = math.Min(lot.tainted, lot.amount)
		}
		b.compact()
		return
	}
	untaken := amount - taken
	for _, lot := range b.lots {
		if taken <= traceEpsilon {
			break
		}
		part := math.Min(taken, lot.tainted)
		lot.tainted -= part
		lot.amount -= part
		taken -= part
	}
	for _, lot := range b.lots {
		if untaken <= traceEpsilon {
			break
		}
		part := math.Min(untaken, lot.amount-lot.tainted)
		lot.amount -= part
		untaken -= part
	}
	b.compact()
}

func lotConfidence(b *traceBook) (float64, string) {
	conf, reason := 1.0, ""
	for _, lot := range b.lots {
		if lot.tainted > traceEpsilon && lot.conf < conf {
			conf, reason = lot.conf, lot.reason
		}
	}
	if reason == "" {
		for _, lot := range b.lots {
			if lot.tainted > traceEpsilon {
				return lot.conf, lot.reason
			}
		}
	}
	return conf, reason
}

// simulateBackward decomposes every outflow of an expanded address into the
// inflows it spent, then walks demand upstream from the seeds' inflows.
func (t *tracer) simulateBackward(movements []*traceMovement) {
	books := map[string]*traceBook{}
	byID := map[string]*traceMovement{}
	for _, m := range movements {
		byID[m.id] = m
	}
	book := func(key, asset string) *traceBook {
		id := key + "\x00" + asset
		if books[id] == nil {
			books[id] = &traceBook{}
		}
		return books[id]
	}
	for _, m := range movements {
		if !t.inWindow(m) {
			continue
		}
		if t.expandedNode(m.from) {
			m.sources = decomposeOutflow(book(m.from, m.consumeAsset), m.consumeAmount, t.query.Policy)
		}
		target := book(m.to, m.deliverAsset)
		target.lots = append(target.lots, &traceLot{origin: m.id, at: m.time, amount: m.deliverAmount})
	}

	type demand struct {
		movement *traceMovement
		amount   float64
		depth    int
		conf     float64
		reason   string
	}
	var queue []demand
	for _, seed := range t.seeds {
		if t.fullSeed {
			for _, m := range movements {
				if m.to == seed && m.from != seed && t.inWindow(m) {
					queue = append(queue, demand{movement: m, amount: m.deliverAmount, depth: 1, conf: 1, reason: traceSeedReason})
				}
			}
			continue
		}
		// Explain the amount still held at the end of the window.
		for _, portion := range decomposeOutflow(book(seed, t.query.Asset), t.query.Amount, t.query.Policy) {
			if m := byID[portion.origin]; m != nil {
				queue = append(queue, demand{movement: m, amount: portion.amount, depth: 1, conf: 1, reason: traceSeedReason})
			}
		}
	}
	for len(queue) > 0 {
		d := queue[0]
		queue = queue[1:]
		m := d.movement
		if d.amount <= traceEpsilon {
			continue
		}
		fraction := math.Min(1, d.amount/m.deliverAmount)
		conf, reason := d.conf, d.reason
		if m.edgeConfidence < conf {
			conf, reason = m.edgeConfidence, m.edgeReason
		}
		addedDeliver := fraction * m.deliverAmount
		m.tracedDeliver = math.Min(m.deliverAmount, m.tracedDeliver+addedDeliver)
		m.tracedConsume = math.Min(m.consumeAmount, m.tracedConsume+fraction*m.consumeAmount)
		if m.confidence == 0 || conf < m.confidence {
			m.confidence, m.confidenceReason = conf, reason
		}
		source := t.nodes[m.from]
		if source == nil {
			continue
		}
		if source.depth < 0 || source.depth > d.depth {
			source.depth = d.depth
		}
		if !source.expanded || t.seedSet[m.from] || d.depth >= t.query.MaxDepth {
			continue
		}
		consumed := 0.0
		for _, portion := range m.sources {
			consumed += portion.amount
		}
		mixed := len(m.sources) > 1
		for _, portion := range m.sources {
			upstream := byID[portion.origin]
			if upstream == nil || consumed <= 0 {
				continue
			}
			share := fraction * m.consumeAmount * portion.amount / consumed
			nextConf, nextReason := conf, reason
			if mixed {
				nextConf *= traceMixingFactor
				nextReason = fmt.Sprintf("spent from several inflows at %s; %s attribution", t.label(m.from), strings.ReplaceAll(t.query.Policy, "_", " "))
			}
			queue = append(queue, demand{movement: upstream, amount: share, depth: d.depth + 1, conf: nextConf, reason: nextReason})
		}
	}
	t.forwardBooks = nil
}

// decomposeOutflow removes amount from the lots and reports which lots (by
// origin) it came from. Funds beyond the known lots came from before the
// window and have origin "".
func decomposeOutflow(b *traceBook, amount float64, policy string) []traceSourcePortion {
	var portions []traceSourcePortion
	remaining := amount
	switch policy {
	case TracePolicyHaircut:
		total, _ := b.total()
		if total > traceEpsilon {
			ratio := math.Min(1, amount/total)
			for _, lot := range b.lots {
				part := lot.amount * ratio
				if part > traceEpsilon {
					portions = append(portions, traceSourcePortion{origin: lot.origin, amount: part})
				}
				lot.amount -= part
				remaining -= part
			}
		}
	default:
		order := append([]*traceLot{}, b.lots...)
		if policy == TracePolicyLargestOut {
			sort.SliceStable(order, func(i, j int) bool { return order[i].amount > order[j].amount })
		}
		for _, lot := range order {
			if remaining <= traceEpsilon {
				break
			}
			part := math.Min(remaining, lot.amount)
			if part > traceEpsilon {
				portions = append(portions, traceSourcePortion{origin: lot.origin, amount: part})
			}
			lot.amount -= part
			remaining -= part
		}
	}
	b.compact()
	if remaining > traceEpsilon {
		portions = append(portions, traceSourcePortion{origin: "", amount: remaining})
	}
	return portions
}

func (t *tracer) label(key string) string {
	if node := t.nodes[key]; node != nil {
		if node.node.Label != "" {
			return node.node.Label
		}
		if node.address != "" {
			return shortAddress(node.address)
		}
	}
	return key
}

// counterpartyKey is the address a traced movement leads to: its recipient
// going forward, its sender going backward.
func (t *tracer) counterpartyKey(m *traceMovement) (from, to string) {
	if t.query.Direction == TraceBackward {
		return m.to, m.from
	}
	return m.from, m.to
}

// nextExpansions picks the addresses the trace should expand next, applying
// the stop categories, depth, branch, and value limits.
func (t *tracer) nextExpansions() []string {
	type branch struct {
		key string
		usd float64
		amt float64
	}
	bySource := map[string]map[string]*branch{}
	for _, m := range t.movements {
		if m.tracedDeliver <= traceEpsilon {
			continue
		}
		source, target := t.counterpartyKey(m)
		if source == target {
			continue
		}
		if bySource[source] == nil {
			bySource[source] = map[string]*branch{}
		}
		b := bySource[source][target]
		if b == nil {
			b = &branch{key: target}
			bySource[source][target] = b
		}
		b.usd += m.tracedUSD()
		b.amt += m.tracedDeliver
	}
	t.branchCut = map[string]bool{}
	t.minCut = map[string]bool{}
	var next []string
	seen := map[string]bool{}
	for _, branches := range bySource {
		ordered := make([]*branch, 0, len(branches))
		for _, b := range branches {
			ordered = append(ordered, b)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].usd != ordered[j].usd {
				return ordered[i].usd > ordered[j].usd
			}
			if ordered[i].amt != ordered[j].amt {
				return ordered[i].amt > ordered[j].amt
			}
			return ordered[i].key < ordered[j].key
		})
		for rank, b := range ordered {
			node := t.nodes[b.key]
			if node == nil || node.kind != "address" || node.expanded || t.stop[node.category] {
				continue
			}
			if node.depth < 0 || node.depth >= t.query.MaxDepth {
				continue
			}
			if rank >= t.query.MaxBranches {
				t.branchCut[b.key] = true
				continue
			}
			if t.query.MinUSDAtTime > 0 && b.usd < t.query.MinUSDAtTime {
				t.minCut[b.key] = true
				continue
			}
			if !seen[b.key] {
				seen[b.key] = true
				next = append(next, b.key)
			}
		}
	}
	// A recipient reached by one branch but cut by another is still followed.
	for _, key := range next {
		delete(t.branchCut, key)
		delete(t.minCut, key)
	}
	sort.Strings(next)
	if room := traceMaxExpansions - t.expansions; len(next) > room {
		for _, key := range next[max(0, room):] {
			t.limitCut[key] = true
		}
		next = next[:max(0, room)]
		t.warnings = appendUniqueString(t.warnings, fmt.Sprintf("trace stopped expanding after %d addresses", traceMaxExpansions))
	}
	return next
}
