package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A symmetric add-liquidity action has one inbound leg per side (RUNE from a
// THOR address, BCH from a BCH address). When both addresses are traced, each
// one's history returns the same action. Each address must keep its own leg
// into the pool regardless of which listing is processed first.
const (
	sharedActionRuneAddress = "thor1egxvam70a86jafa8gcg8q9n8wp4lyp3vqyfg3w"
	sharedActionBCHAddress  = "qz7262r7uufxk89ematxrf6yquk7zfwrjqm97vskzw"
)

func sharedSymmetricAddLiquidity() midgardAction {
	return midgardAction{
		Date:   "1641821954000000000",
		Height: "3900000",
		Type:   "addLiquidity",
		Status: "success",
		Pools:  []string{"BCH.BCH"},
		In: []midgardActionLeg{
			{
				Address: sharedActionRuneAddress,
				TxID:    "0CC81DDB48F14BFB3BE75B4DB784AB9DE4E0A637570F482DFD363040E032566A",
				Coins:   []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "50000000000"}},
			},
			{
				Address: sharedActionBCHAddress,
				TxID:    "862C6153A4456C9896EBA9C2B982B67466AB92A8D9FDA2B1E01D721F1B4DD18D",
				Coins:   []midgardActionCoin{{Asset: "BCH.BCH", Amount: "1000000000"}},
			},
		},
	}
}

func TestBuildActorTrackerKeepsEachTracedAddressLegOfSharedAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/actions":
			actions := []midgardAction{}
			switch normalizeAddress(r.URL.Query().Get("address")) {
			case normalizeAddress(sharedActionRuneAddress), normalizeAddress(sharedActionBCHAddress):
				actions = []midgardAction{sharedSymmetricAddLiquidity()}
			}
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: actions})
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{{
				Asset: "BCH.BCH", Status: "available", AssetDepth: "100000000000", RuneDepth: "5000000000000", AssetPriceUSD: "400",
			}})
		case r.URL.Path == "/thorchain/inbound_addresses", r.URL.Path == "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "shared-action.db"),
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	actor, err := upsertActor(context.Background(), app.db, 0, ActorUpsertRequest{
		Name:  "Treasury LP",
		Color: "#123456",
		Addresses: []ActorAddressInput{
			{Address: sharedActionRuneAddress, Label: "TreasuryLP RUNE"},
			{Address: sharedActionBCHAddress, Label: "TreasuryLP BCH"},
		},
	})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}

	resp, err := app.buildActorTracker(context.Background(), ActorTrackerRequest{
		ActorIDs:  []int64{actor.ID},
		StartTime: "2022-01-01T00:00:00Z",
		EndTime:   "2022-02-01T00:00:00Z",
		MaxHops:   1,
		FlowTypes: []string{"liquidity"},
	})
	if err != nil {
		t.Fatalf("build actor tracker: %v", err)
	}

	legIntoPool := func(address string) *FlowEdge {
		for i := range resp.Edges {
			edge := &resp.Edges[i]
			if edge.ActionClass == "liquidity" && strings.Contains(edge.From, normalizeAddress(address)) && strings.HasPrefix(edge.To, "pool:BCH.BCH") {
				return edge
			}
		}
		return nil
	}
	for _, address := range []string{sharedActionRuneAddress, sharedActionBCHAddress} {
		edge := legIntoPool(address)
		if edge == nil {
			ids := make([]string, 0, len(resp.Edges))
			for _, e := range resp.Edges {
				ids = append(ids, e.ID)
			}
			t.Fatalf("expected a liquidity edge from %s into the BCH pool; edges: %v", address, ids)
		}
		if len(edge.TxIDs) != 1 {
			t.Fatalf("expected the shared action counted once on %s's leg, got tx ids %v", address, edge.TxIDs)
		}
	}
}

// sharedActionPeerAddress is a second traced THOR address. It sorts after
// sharedActionRuneAddress, so seeds are processed RUNE address first.
const sharedActionPeerAddress = "thor1zx4pnmzc9tqvjk8a5ypj7m6fwl3xh0qd2e8r7c"

// newSharedActionTestApp serves each address's Midgard history from
// historyByAddress and stubs the other upstreams a graph build touches.
func newSharedActionTestApp(t *testing.T, historyByAddress map[string][]midgardAction) *App {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/actions":
			actions := historyByAddress[normalizeAddress(r.URL.Query().Get("address"))]
			if actions == nil {
				actions = []midgardAction{}
			}
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: actions})
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{{
				Asset: "BCH.BCH", Status: "available", AssetDepth: "100000000000", RuneDepth: "5000000000000", AssetPriceUSD: "400",
			}})
		case r.URL.Path == "/thorchain/inbound_addresses", r.URL.Path == "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "shared-action.db"),
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func upsertSharedActionActor(t *testing.T, app *App, addresses ...string) Actor {
	t.Helper()
	inputs := make([]ActorAddressInput, 0, len(addresses))
	for _, address := range addresses {
		inputs = append(inputs, ActorAddressInput{Address: address})
	}
	actor, err := upsertActor(context.Background(), app.db, 0, ActorUpsertRequest{
		Name:      "Shared Action Actor",
		Color:     "#123456",
		Addresses: inputs,
	})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}
	return actor
}

// An action whose only segment connects two traced addresses is returned by
// both addresses' histories and touches both frontiers. Re-projecting it for
// the second frontier must not add the segment again: the graph must hold one
// edge transaction with the action's amounts, whichever address goes first.
func TestBuildActorTrackerCountsActionBetweenTracedAddressesOnce(t *testing.T) {
	type assetAmount struct{ asset, direction, amount string }
	cases := []struct {
		name     string
		from, to string
		flowType string
		action   midgardAction
		txID     string
		want     []assetAmount
	}{
		{
			name:     "transfer from first processed address",
			from:     sharedActionRuneAddress,
			to:       sharedActionPeerAddress,
			flowType: "transfers",
			txID:     "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9",
			action: midgardAction{
				Date: "1641821954000000000", Height: "3900001", Type: "send", Status: "success",
				In:  []midgardActionLeg{{Address: sharedActionRuneAddress, TxID: "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "1234500000"}}}},
				Out: []midgardActionLeg{{Address: sharedActionPeerAddress, TxID: "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "1234500000"}}}},
			},
			want: []assetAmount{{asset: "THOR.RUNE", amount: "1234500000"}},
		},
		{
			name:     "transfer to first processed address",
			from:     sharedActionPeerAddress,
			to:       sharedActionRuneAddress,
			flowType: "transfers",
			txID:     "B1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9",
			action: midgardAction{
				Date: "1641821955000000000", Height: "3900002", Type: "send", Status: "success",
				In:  []midgardActionLeg{{Address: sharedActionPeerAddress, TxID: "B1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "777000000"}}}},
				Out: []midgardActionLeg{{Address: sharedActionRuneAddress, TxID: "B1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "777000000"}}}},
			},
			want: []assetAmount{{asset: "THOR.RUNE", amount: "777000000"}},
		},
		{
			name:     "swap from first processed address",
			from:     sharedActionBCHAddress,
			to:       sharedActionRuneAddress,
			flowType: "swaps",
			txID:     "C1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9",
			action: midgardAction{
				Date: "1641821956000000000", Height: "3900003", Type: "swap", Status: "success", Pools: []string{"BCH.BCH"},
				In:  []midgardActionLeg{{Address: sharedActionBCHAddress, TxID: "C1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "BCH.BCH", Amount: "1000000000"}}}},
				Out: []midgardActionLeg{{Address: sharedActionRuneAddress, Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "50000000000"}}}},
			},
			want: []assetAmount{{asset: "BCH.BCH", direction: "in", amount: "1000000000"}, {asset: "THOR.RUNE", direction: "out", amount: "50000000000"}},
		},
		{
			name:     "swap to first processed address",
			from:     sharedActionRuneAddress,
			to:       sharedActionBCHAddress,
			flowType: "swaps",
			txID:     "D1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9",
			action: midgardAction{
				Date: "1641821957000000000", Height: "3900004", Type: "swap", Status: "success", Pools: []string{"BCH.BCH"},
				In:  []midgardActionLeg{{Address: sharedActionRuneAddress, TxID: "E1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "THOR.RUNE", Amount: "50000000000"}}}},
				Out: []midgardActionLeg{{Address: sharedActionBCHAddress, TxID: "D1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F9", Coins: []midgardActionCoin{{Asset: "BCH.BCH", Amount: "990000000"}}}},
			},
			want: []assetAmount{{asset: "THOR.RUNE", direction: "in", amount: "50000000000"}, {asset: "BCH.BCH", direction: "out", amount: "990000000"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newSharedActionTestApp(t, map[string][]midgardAction{
				normalizeAddress(tc.from): {tc.action},
				normalizeAddress(tc.to):   {tc.action},
			})
			actor := upsertSharedActionActor(t, app, tc.from, tc.to)

			resp, err := app.buildActorTracker(context.Background(), ActorTrackerRequest{
				ActorIDs:  []int64{actor.ID},
				StartTime: "2022-01-01T00:00:00Z",
				EndTime:   "2022-02-01T00:00:00Z",
				MaxHops:   1,
				FlowTypes: []string{tc.flowType},
			})
			if err != nil {
				t.Fatalf("build actor tracker: %v", err)
			}

			var flowEdges []FlowEdge
			for _, edge := range resp.Edges {
				if edge.ActionClass != "ownership" {
					flowEdges = append(flowEdges, edge)
				}
			}
			if len(flowEdges) != 1 {
				ids := make([]string, 0, len(flowEdges))
				for _, edge := range flowEdges {
					ids = append(ids, edge.ID)
				}
				t.Fatalf("expected one flow edge, got %d: %v", len(flowEdges), ids)
			}
			edge := flowEdges[0]
			if !strings.Contains(edge.From, normalizeAddress(tc.from)) || !strings.Contains(edge.To, normalizeAddress(tc.to)) {
				t.Fatalf("expected edge %s -> %s, got %s -> %s", tc.from, tc.to, edge.From, edge.To)
			}
			if len(edge.Transactions) != 1 || len(edge.TxIDs) != 1 || edge.TxIDs[0] != tc.txID {
				t.Fatalf("expected one edge transaction %s, got tx ids %v and %d transactions", tc.txID, edge.TxIDs, len(edge.Transactions))
			}
			tx := edge.Transactions[0]
			if len(tx.Assets) != len(tc.want) {
				t.Fatalf("expected %d assets on the edge transaction, got %+v", len(tc.want), tx.Assets)
			}
			for _, want := range tc.want {
				found := false
				for _, got := range tx.Assets {
					if got.Asset == want.asset && got.Direction == want.direction {
						found = true
						if got.AmountRaw != want.amount {
							t.Fatalf("%s %s amount = %s, want %s (counted more than once?)", want.direction, want.asset, got.AmountRaw, want.amount)
						}
					}
				}
				if !found {
					t.Fatalf("missing %s %s on edge transaction: %+v", want.direction, want.asset, tx.Assets)
				}
			}

			if len(resp.SupportingActions) != 1 {
				t.Fatalf("expected one supporting action, got %d: %+v", len(resp.SupportingActions), resp.SupportingActions)
			}
			if got := resp.SupportingActions[0]; got.TxID != tc.txID || got.FromNode != edge.From || got.ToNode != edge.To {
				t.Fatalf("supporting action %+v does not match edge %s", got, edge.ID)
			}
			// The second frontier's copy must be dropped before reaching the
			// graph, not merely collapsed by the swap canonical-key check.
			if deduped, _ := resp.Stats["swap_deduped"].(int); deduped != 0 {
				t.Fatalf("expected no duplicate swap segments to reach the graph, swap_deduped=%d", deduped)
			}
		})
	}
}

// One-hop expansion over several addresses shares the action handling of the
// full build: each expanded address keeps its own leg of a shared action.
func TestExpandActorTrackerKeepsEachExpandedAddressLegOfSharedAction(t *testing.T) {
	shared := sharedSymmetricAddLiquidity()
	app := newSharedActionTestApp(t, map[string][]midgardAction{
		normalizeAddress(sharedActionRuneAddress): {shared},
		normalizeAddress(sharedActionBCHAddress):  {shared},
	})
	actor := upsertSharedActionActor(t, app, sharedActionRuneAddress, sharedActionBCHAddress)

	resp, err := app.expandActorTrackerOneHop(context.Background(), ActorTrackerExpandRequest{
		ActorIDs:  []int64{actor.ID},
		Addresses: []string{sharedActionRuneAddress, sharedActionBCHAddress},
		StartTime: "2022-01-01T00:00:00Z",
		EndTime:   "2022-02-01T00:00:00Z",
		FlowTypes: []string{"liquidity"},
	})
	if err != nil {
		t.Fatalf("expand actor tracker: %v", err)
	}
	for _, address := range []string{sharedActionRuneAddress, sharedActionBCHAddress} {
		var leg *FlowEdge
		for i := range resp.Edges {
			edge := &resp.Edges[i]
			if edge.ActionClass == "liquidity" && strings.Contains(edge.From, normalizeAddress(address)) && strings.HasPrefix(edge.To, "pool:BCH.BCH") {
				leg = edge
			}
		}
		if leg == nil {
			t.Fatalf("expected a liquidity edge from %s into the BCH pool", address)
		}
		if len(leg.TxIDs) != 1 || len(leg.Transactions) != 1 {
			t.Fatalf("expected %s's leg counted once, got tx ids %v", address, leg.TxIDs)
		}
	}
}
