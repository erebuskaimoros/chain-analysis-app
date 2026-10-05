package app

import (
	"math"
	"testing"
	"time"
)

// traceFixture builds a tracer over hand-made movements, without HTTP.
type traceFixture struct {
	t     *tracer
	start time.Time
	n     int
}

func newTraceFixture(direction, policy string, maxDepth int, amount float64, asset string, seeds ...string) *traceFixture {
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	query := TraceQuery{
		StartTime: start, EndTime: start.Add(24 * time.Hour), Direction: direction, Policy: policy,
		MaxDepth: maxDepth, MaxBranches: 8, Amount: amount, Asset: asset, StopCategories: traceDefaultStopCategories,
	}
	t := &tracer{
		query: query, stop: map[string]bool{"exchange": true}, nodes: map[string]*traceNode{}, movements: map[string]*traceMovement{},
		seedSet: map[string]bool{}, fullSeed: amount <= 0, branchCut: map[string]bool{}, minCut: map[string]bool{}, limitCut: map[string]bool{},
	}
	f := &traceFixture{t: t, start: start}
	for _, seed := range seeds {
		f.address(seed, true)
		t.seeds = append(t.seeds, seed)
		t.seedSet[seed] = true
	}
	return f
}

func (f *traceFixture) address(key string, expanded bool) {
	f.t.nodes[key] = &traceNode{key: key, kind: "address", address: key, chain: "BTC", expanded: expanded, depth: -1,
		node: FlowNode{ID: key, Kind: "external_address", Label: key}}
}

func (f *traceFixture) move(minute int, from, to, asset string, amount float64) *traceMovement {
	return f.swap(minute, from, to, asset, amount, asset, amount)
}

func (f *traceFixture) swap(minute int, from, to, inAsset string, inAmount float64, outAsset string, outAmount float64) *traceMovement {
	f.n++
	class := "transfers"
	if inAsset != outAsset {
		class = "swaps"
	}
	m := &traceMovement{
		id: from + "-" + to + "-" + string(rune('a'+f.n)), txID: "TX" + string(rune('A'+f.n)), time: f.start.Add(time.Duration(minute) * time.Minute),
		from: from, to: to, actionClass: class, actionKey: class, actionLabel: class, edgeConfidence: 1,
		consumeAsset: inAsset, consumeAmount: inAmount, consumeUSDUnit: 100, consumePriced: true,
		deliverAsset: outAsset, deliverAmount: outAmount, deliverUSDUnit: 100, deliverPriced: true,
	}
	f.t.movements[m.id] = m
	return m
}

func near(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s: got %.6f, want %.6f", label, got, want)
	}
}

func TestTraceForwardPoliciesAllocateTracedFunds(t *testing.T) {
	cases := []struct {
		policy           string
		small, large     float64
		largeConfReduced bool
	}{
		{TracePolicyFIFO, 3, 7, true},
		{TracePolicyHaircut, 2, 8, true},
		{TracePolicyLargestOut, 0, 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.policy, func(t *testing.T) {
			f := newTraceFixture(TraceForward, tc.policy, 1, 10, "BTC.BTC", "S")
			f.address("U", false)
			f.address("X", false)
			f.address("Y", false)
			f.move(10, "U", "S", "BTC.BTC", 5)
			small := f.move(20, "S", "X", "BTC.BTC", 3)
			large := f.move(30, "S", "Y", "BTC.BTC", 12)
			f.t.simulate()
			near(t, "small outflow", small.tracedDeliver, tc.small)
			near(t, "large outflow", large.tracedDeliver, tc.large)
			if tc.largeConfReduced && large.confidence >= 1 {
				t.Fatalf("expected mixing to lower confidence, got %.2f (%s)", large.confidence, large.confidenceReason)
			}
			resp := f.t.result()
			total := 0.0
			for _, endpoint := range resp.Frontier {
				if endpoint.Reason != "max_depth" {
					t.Fatalf("unexpected frontier reason %q", endpoint.Reason)
				}
				total += endpoint.Assets[0].Amount
			}
			near(t, "traced value reaching the frontier", total, 10)
		})
	}
}

func TestTraceForwardSwapConvertsTracedShareAndKeepsTxIDs(t *testing.T) {
	f := newTraceFixture(TraceForward, TracePolicyFIFO, 1, 0, "", "S")
	f.address("R", false)
	m := f.swap(5, "S", "R", "ETH.ETH", 100, "BTC.BTC", 3.1)
	m.inboundTxID = "INBOUND"
	f.t.simulate()
	resp := f.t.result()
	if len(resp.Edges) != 1 {
		t.Fatalf("expected one traced edge, got %d", len(resp.Edges))
	}
	edge := resp.Edges[0]
	near(t, "traced BTC", edge.TracedAssets[0].Amount, 3.1)
	near(t, "traced ETH input", edge.TracedInputAssets[0].Amount, 100)
	if len(edge.TxIDs) != 2 || edge.TracedTransactions[0].InboundTxID != "INBOUND" {
		t.Fatalf("expected both swap tx IDs on the edge, got %v", edge.TxIDs)
	}
	if len(resp.Frontier) != 1 || resp.Frontier[0].NodeID != "R" {
		t.Fatalf("expected the recipient as the frontier, got %+v", resp.Frontier)
	}
	near(t, "seed USD", resp.Totals.SeedUSD, 10000)
}

func TestTraceForwardStopsAtExchangesAndReportsHeldFunds(t *testing.T) {
	f := newTraceFixture(TraceForward, TracePolicyFIFO, 3, 0, "", "S")
	f.address("A", true)
	f.address("EX", false)
	f.t.nodes["EX"].category = "exchange"
	f.move(5, "S", "A", "BTC.BTC", 4)
	f.move(6, "A", "EX", "BTC.BTC", 1)
	f.t.simulate()
	resp := f.t.result()
	reasons := map[string]float64{}
	for _, sink := range resp.Sinks {
		reasons[sink.NodeID+":"+sink.Reason] = sink.Assets[0].Amount
	}
	near(t, "exchange sink", reasons["EX:exchange"], 1)
	near(t, "held at A", reasons["A:held"], 3)
	if next := f.t.nextExpansions(); len(next) != 0 {
		t.Fatalf("expected nothing more to expand, got %v", next)
	}
}

func TestTraceBackwardAttributesInflowsByPolicy(t *testing.T) {
	cases := []struct {
		policy string
		p, q   float64
	}{
		{TracePolicyFIFO, 3, 1},
		{TracePolicyHaircut, 1.5, 2.5},
		{TracePolicyLargestOut, 0, 4},
	}
	for _, tc := range cases {
		t.Run(tc.policy, func(t *testing.T) {
			f := newTraceFixture(TraceBackward, tc.policy, 2, 0, "", "T")
			f.address("A", true)
			f.address("P", false)
			f.address("Q", false)
			fromP := f.move(10, "P", "A", "BTC.BTC", 3)
			fromQ := f.move(20, "Q", "A", "BTC.BTC", 5)
			f.move(30, "A", "T", "BTC.BTC", 4)
			f.t.simulate()
			near(t, "from P", fromP.tracedDeliver, tc.p)
			near(t, "from Q", fromQ.tracedDeliver, tc.q)
			resp := f.t.result()
			total := 0.0
			for _, endpoint := range append(resp.Sinks, resp.Frontier...) {
				total += endpoint.Assets[0].Amount
			}
			near(t, "attributed sources", total, 4)
		})
	}
}

func TestTraceIgnoresDuplicateVaultPayoutOfASwap(t *testing.T) {
	f := newTraceFixture(TraceForward, TracePolicyFIFO, 2, 0, "", "S")
	swap := &traceMovement{id: "swap", txID: "OUT1", from: "S", to: "R", actionClass: "swaps", consumeAsset: "ETH.ETH", deliverAsset: "BTC.BTC", consumeAmount: 1, deliverAmount: 0.03}
	payout := &traceMovement{id: "payout", txID: "OUT1", from: "VAULT", to: "R", actionClass: "transfers", consumeAsset: "BTC.BTC", deliverAsset: "BTC.BTC", consumeAmount: 0.03, deliverAmount: 0.03}
	f.t.addMovement(payout)
	f.t.addMovement(swap)
	if _, ok := f.t.movements["payout"]; ok || f.t.movements["swap"] == nil {
		t.Fatalf("expected the swap to replace the vault payout, got %v", f.t.movements)
	}
	f.t.addMovement(&traceMovement{id: "payout-again", txID: "OUT1", from: "VAULT", to: "R", actionClass: "transfers", deliverAsset: "BTC.BTC", deliverAmount: 0.03})
	if len(f.t.movements) != 1 {
		t.Fatalf("expected the payout seen later to be ignored, got %d movements", len(f.t.movements))
	}
}

func TestNormalizeTraceRequestValidates(t *testing.T) {
	if _, err := normalizeTraceRequest(TraceRequest{}); err == nil {
		t.Fatal("expected an error without seeds")
	}
	if _, err := normalizeTraceRequest(TraceRequest{Seeds: []TraceSeed{{Address: "x"}}, Policy: "lifo"}); err == nil {
		t.Fatal("expected an error for an unknown policy")
	}
	if _, err := normalizeTraceRequest(TraceRequest{Seeds: []TraceSeed{{Address: "x"}}, Amount: 1}); err == nil {
		t.Fatal("expected an error for an amount without an asset")
	}
	q, err := normalizeTraceRequest(TraceRequest{Seeds: []TraceSeed{{Address: "x"}}, MaxDepth: 99})
	if err != nil || q.MaxDepth != traceMaxDepthLimit || q.Policy != TracePolicyFIFO || q.Direction != TraceForward || len(q.StopCategories) != 3 {
		t.Fatalf("unexpected defaults %+v (%v)", q, err)
	}
}
