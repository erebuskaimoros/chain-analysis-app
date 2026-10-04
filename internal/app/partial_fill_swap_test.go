package app

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A streaming swap that only partly fills is reported by Midgard as a swap
// action (filled input, refund leg to the sender, and the swap output) plus a
// separate refund action that shares the swap's inbound tx ID. Shapes mirror
// the Bitget exploiter's swap at 2026-09-28 05:03:28 UTC.
const (
	partialFillSender    = "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"
	partialFillRecipient = "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"
	partialFillInTx      = "BB6ACE87F2B8EAF2D9C1F4A9AA7A47C3E3A3C8F8E6F00A1B2C3D4E5F60718293"
	partialFillRefundTx  = "C0A2FA764F0FB59C9E0CDA6C71B0D5C1E2B8F3E4A5B6C7D8E9F0A1B2C3D4E5F6"
	partialFillOutTx     = "B98FF49AC3A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B"
)

func partialFillSwapActions() []midgardAction {
	outLegs := []midgardActionLeg{
		{
			Address: partialFillSender,
			TxID:    partialFillRefundTx,
			Coins:   []midgardActionCoin{{Asset: "ETH.ETH", Amount: "5384605959"}},
		},
		{
			Address: partialFillRecipient,
			TxID:    partialFillOutTx,
			Coins:   []midgardActionCoin{{Asset: "BTC.BTC", Amount: "144399139"}},
		},
	}
	return []midgardAction{
		{
			Date:   "1790572052422214975",
			Height: "28011399",
			Type:   "refund",
			Status: "success",
			In: []midgardActionLeg{{
				Address: partialFillSender,
				TxID:    partialFillInTx,
				Coins:   []midgardActionCoin{{Asset: "ETH.ETH", Amount: "5384615376"}},
			}},
			Out: outLegs,
		},
		{
			Date:   "1790571808344367972",
			Height: "28011360",
			Type:   "swap",
			Status: "success",
			In: []midgardActionLeg{{
				Address: partialFillSender,
				TxID:    partialFillInTx,
				Coins:   []midgardActionCoin{{Asset: "ETH.ETH", Amount: "4615384624"}},
			}},
			Out:   outLegs,
			Pools: []string{"ETH.ETH", "BTC.BTC"},
		},
	}
}

func TestBuildActorTrackerKeepsPartiallyFilledSwap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/actions":
			actions := []midgardAction{}
			if normalizeAddress(r.URL.Query().Get("address")) == normalizeAddress(partialFillSender) {
				actions = partialFillSwapActions()
			}
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: actions})
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Path == "/thorchain/inbound_addresses", r.URL.Path == "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "partial-fill.db"),
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
		Name:      "Exploiter",
		Color:     "#d94b4b",
		Addresses: []ActorAddressInput{{Address: partialFillSender, ChainHint: "ETH"}},
	})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}

	resp, err := app.buildActorTracker(context.Background(), ActorTrackerRequest{
		ActorIDs:  []int64{actor.ID},
		StartTime: "2026-09-28T03:00:00Z",
		EndTime:   "2026-09-28T07:00:00Z",
		MaxHops:   1,
		FlowTypes: []string{"swaps"},
	})
	if err != nil {
		t.Fatalf("build actor tracker: %v", err)
	}

	var swapEdge *FlowEdge
	for i := range resp.Edges {
		edge := &resp.Edges[i]
		if edge.ActionClass != "swaps" {
			continue
		}
		if strings.Contains(edge.From, strings.ToLower(partialFillSender)) && strings.Contains(edge.To, partialFillRecipient) {
			swapEdge = edge
		}
		if strings.Contains(edge.From, strings.ToLower(partialFillSender)) && strings.Contains(edge.To, strings.ToLower(partialFillSender)) {
			t.Fatalf("refund leg back to the sender must not become a swap edge: %#v", edge)
		}
	}
	if swapEdge == nil {
		t.Fatalf("expected the partially filled swap sender -> recipient, got edges %#v (stats %v)", resp.Edges, resp.Stats)
	}

	amounts := map[string]string{}
	for _, asset := range swapEdge.Assets {
		amounts[asset.Asset] = asset.AmountRaw
	}
	if got := amounts["BTC.BTC"]; !sameRawAmount(got, "144399139") {
		t.Fatalf("expected swap output 144399139 BTC sats, got %q (assets %#v)", got, swapEdge.Assets)
	}
	if got := amounts["ETH.ETH"]; !sameRawAmount(got, "4615384624") {
		t.Fatalf("expected filled input 4615384624 ETH, got %q (assets %#v)", got, swapEdge.Assets)
	}
	if got := intStat(resp.Stats, "swap_emitted"); got != 1 {
		t.Fatalf("expected swap_emitted=1, got %d", got)
	}
}

func sameRawAmount(got, want string) bool {
	g, ok := new(big.Int).SetString(strings.TrimSpace(got), 10)
	if !ok {
		return false
	}
	w, _ := new(big.Int).SetString(want, 10)
	return g.Cmp(w) == 0
}
