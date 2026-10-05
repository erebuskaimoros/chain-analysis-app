package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRefreshActorRecordsBaselineThenOnlyNewFlows(t *testing.T) {
	const (
		treasury = "thor1monitortreasury0000000000000000000000000"
		vendor   = "thor1monitorvendor00000000000000000000000000"
		newPeer  = "thor1monitornewpeer0000000000000000000000000"
	)
	upstream := &midgardWindowServer{}
	send := func(at time.Time, to, tx string) midgardAction {
		return testTHORSendAction(strconv.FormatInt(at.UnixNano(), 10), strconv.FormatInt(at.Unix(), 10), tx, treasury, to, "500000000")
	}
	upstream.actions = []midgardAction{send(time.Now().Add(-48*time.Hour), vendor, "TX-BASELINE")}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/actions":
			upstream.handler(w, r)
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case strings.HasPrefix(r.URL.Path, "/cosmos/bank/v1beta1/balances/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"balances": []map[string]any{{"denom": "rune", "amount": "1200000000"}}})
		case r.URL.Path == "/thorchain/nodes", r.URL.Path == "/thorchain/inbound_addresses":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "monitor.db"),
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	ctx := context.Background()
	actor, err := upsertActor(ctx, app.db, 0, ActorUpsertRequest{Name: "Treasury", Color: "#123456", Addresses: []ActorAddressInput{{Address: treasury}}})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}

	baseline, err := app.RefreshActor(ctx, actor.ID)
	if err != nil {
		t.Fatalf("baseline refresh: %v", err)
	}
	if !baseline.Baseline || len(baseline.Flows.Counterparties) != 1 || baseline.Flows.Counterparties[0].FirstSeen {
		t.Fatalf("expected a baseline snapshot with the vendor flow not marked new, got %+v", baseline.Flows)
	}
	if len(baseline.Holdings) != 1 || baseline.Holdings[0].Status != "available" {
		t.Fatalf("expected the treasury holding to be captured, got %+v", baseline.Holdings)
	}

	time.Sleep(1100 * time.Millisecond)
	upstream.mu.Lock()
	upstream.actions = append([]midgardAction{send(time.Now(), newPeer, "TX-NEW")}, upstream.actions...)
	upstream.mu.Unlock()
	time.Sleep(1100 * time.Millisecond)
	upstream.takeRequests()

	second, err := app.RefreshActor(ctx, actor.ID)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if second.Baseline || len(second.Flows.Counterparties) != 1 {
		t.Fatalf("expected only the new flow in the second snapshot, got %+v", second.Flows)
	}
	if cp := second.Flows.Counterparties[0]; !strings.Contains(cp.Key, newPeer) || !cp.FirstSeen || cp.OutUSD < 0 {
		t.Fatalf("expected the new counterparty marked first seen, got %+v", cp)
	}
	for _, req := range upstream.takeRequests() {
		if req[0] < baseline.WindowEnd.Unix()-int64(ledgerTailLag/time.Second)-1 {
			t.Fatalf("second refresh fetched history before the previous snapshot: %v", req)
		}
	}

	monitor, err := app.ActorMonitor(ctx, actor.ID)
	if err != nil {
		t.Fatalf("monitor: %v", err)
	}
	if len(monitor.Series) != 2 || monitor.SinceLastView == nil || len(monitor.SinceLastView.NewCounterparties) != 1 {
		t.Fatalf("expected two snapshots and one new counterparty since last view, got %+v", monitor)
	}
	if err := app.MarkActorViewed(ctx, actor.ID); err != nil {
		t.Fatalf("mark viewed: %v", err)
	}
	if monitor, _ = app.ActorMonitor(ctx, actor.ID); monitor.SinceLastView != nil {
		t.Fatalf("expected no changes right after viewing, got %+v", monitor.SinceLastView)
	}
}

func TestActorChangesSinceReportsAddressAndAssetDeltas(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	holding := func(address string, assets ...ActorAssetHolding) ActorHolding {
		h := ActorHolding{Chain: "THOR", Address: address, Label: address, Status: "available", Assets: assets}
		for _, asset := range assets {
			h.USD += asset.USD
		}
		return h
	}
	runeHolding := func(amount string, usd float64) ActorAssetHolding {
		return ActorAssetHolding{Asset: "THOR.RUNE", AmountRaw: amount, USD: usd}
	}
	snapshots := []ActorSnapshot{
		{TakenAt: base, Baseline: true, TotalUSD: 300, Holdings: []ActorHolding{
			holding("thor1kept", runeHolding("10000000000", 200)),
			holding("thor1gone", runeHolding("5000000000", 100)),
		}},
		{TakenAt: base.Add(time.Hour), TotalUSD: 250, Holdings: []ActorHolding{
			holding("thor1kept", runeHolding("12500000000", 250)),
		}},
	}
	changes := actorChangesSince(snapshots, "")
	if changes == nil || changes.Snapshots != 1 {
		t.Fatalf("expected one snapshot of changes, got %+v", changes)
	}
	if len(changes.HoldingDeltas) != 2 {
		t.Fatalf("expected the emptied address to be reported too, got %+v", changes.HoldingDeltas)
	}
	gone := changes.HoldingDeltas[0]
	if gone.Address != "thor1gone" || gone.DeltaUSD != -100 {
		t.Fatalf("expected the emptied address first with -100, got %+v", gone)
	}
	if len(changes.AssetDeltas) != 1 {
		t.Fatalf("expected one asset delta, got %+v", changes.AssetDeltas)
	}
	if d := changes.AssetDeltas[0]; d.BeforeAmount != 150 || d.AfterAmount != 125 || d.DeltaUSD != -50 {
		t.Fatalf("expected RUNE 150 -> 125 (-$50), got %+v", d)
	}
}

func TestActorAssetHoldingsReadsFreshAndRoundTrippedMetrics(t *testing.T) {
	fresh := []map[string]any{{"asset": "THOR.RUNE", "amount_raw": "100", "usd_spot": 1.5}}
	var roundTripped any
	raw, _ := json.Marshal(fresh)
	_ = json.Unmarshal(raw, &roundTripped)
	for name, value := range map[string]any{"fresh": fresh, "round-tripped": roundTripped} {
		assets := actorAssetHoldings(value)
		if len(assets) != 1 || assets[0].Asset != "THOR.RUNE" || assets[0].AmountRaw != "100" || assets[0].USD != 1.5 {
			t.Fatalf("%s: unexpected assets %+v", name, assets)
		}
	}
}
