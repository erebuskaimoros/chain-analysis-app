package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	testBNBWallet    = "bnb18066ns6adt024wnvpf2hav9466g95av4996gp7"
	testBNBVault     = "bnb1gaflxr8zdntmz8nmklwre5dqunl5u7dtkxlacx"
	testBNBMainVault = "bnb15jc5c2le9wjrmvhw4ny4flclkw99606t8j4w83"
	testBNBExchange  = "bnb165q9dz39mqh789zuuuqwkv22plut6f4nzy9jc9"
	testBNBSwapper   = "bnb1ju6eehwue87tayz5u7pvuh33eke8l84dumf0mp"
	testTHORSwitch   = "thor10cfzswitchtarget0000000000000000000wjrc"
)

func TestParseThorchainMemo(t *testing.T) {
	cases := []struct {
		memo   string
		kind   string
		target string
		legacy bool
		ok     bool
	}{
		{memo: "SWAP:BNB.BUSD-BD1::83644493893", kind: "swap"},
		{memo: "swap:BNB.BNB:" + testBNBWallet, kind: "swap", target: testBNBWallet},
		{memo: "=:BNB.BNB:" + testBNBWallet + ":0", kind: "swap", target: testBNBWallet},
		{memo: "STAKE:BNB.BNB", kind: "add"},
		{memo: "WITHDRAW:BNB.BNB:10000", kind: "withdraw"},
		{memo: "OUTBOUND:F5308217", kind: "outbound", target: "F5308217", legacy: true},
		{memo: "OUT:48406A5D", kind: "outbound", target: "48406A5D"},
		{memo: "REFUND:ABC", kind: "refund", target: "ABC"},
		{memo: "SWITCH:" + testTHORSwitch, kind: "switch", target: testTHORSwitch},
		{memo: "YGGDRASIL+:123", kind: "vault"},
		{memo: "330563132", ok: false},
		{memo: "", ok: false},
	}
	for _, tc := range cases {
		got, ok := parseThorchainMemo(tc.memo)
		wantOK := tc.kind != ""
		if ok != wantOK {
			t.Fatalf("%q: ok=%v want %v", tc.memo, ok, wantOK)
		}
		if !ok {
			continue
		}
		if got.Kind != tc.kind || got.Target != tc.target || got.Legacy != tc.legacy {
			t.Fatalf("%q: got %#v", tc.memo, got)
		}
	}
}

func TestBNBBeaconAmountRaw(t *testing.T) {
	for raw, want := range map[string]string{
		"134.68000000": "13468000000",
		"3260014":      "326001400000000",
		"1E-8":         "1",
		"0":            "",
	} {
		if got := bnbBeaconAmountRaw(json.Number(raw)); got != want {
			t.Fatalf("%s: got %q want %q", raw, got, want)
		}
	}
}

func bnbBeaconTestRow(hash string, ts int64, from, to, value, asset, memo string) map[string]any {
	return map[string]any{
		"txHash":      hash,
		"blockHeight": ts / 1000,
		"txType":      "TRANSFER",
		"timeStamp":   ts,
		"fromAddr":    from,
		"toAddr":      to,
		"value":       json.Number(value),
		"txAsset":     asset,
		"code":        0,
		"memo":        memo,
		"hasChildren": 0,
	}
}

func newBNBBeaconTestApp(t *testing.T, handler http.HandlerFunc) *App {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &App{
		cfg:           Config{BNBBeaconExplorerURL: server.URL},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}
}

func TestFetchBNBBeaconTransfersLinksThorchainMemos(t *testing.T) {
	const (
		bepswapIn  = "F5308217EE6C9131C3FDD991AC32CD40E3DBDD9CA2449105A19C426ABCF0E4BC"
		bepswapOut = "A1A1A1A1EE6C9131C3FDD991AC32CD40E3DBDD9CA2449105A19C426ABCF0E4BC"
	)
	var lookups int
	app := newBNBBeaconTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/txs":
			if r.URL.Query().Get("address") != testBNBWallet || r.URL.Query().Get("rows") != "100" {
				t.Errorf("unexpected txs query: %s", r.URL.RawQuery)
			}
			rows := []map[string]any{
				// Exchange deposit with a numeric memo: a plain transfer.
				bnbBeaconTestRow("135CA7E5CFBA3651B35865630667B3E30320C5AF9E775CFF1526E13C08D3B1B2", 1699447399711, testBNBWallet, testBNBExchange, "134.68000000", "BNB", "330563132"),
				// Mainnet outbound: vault leg only, Midgard has the swap.
				bnbBeaconTestRow("E6FFF10AC5C3B0A34EDDE88D9AED9613B30E5A6213A8CAC66E80FB48E7A6D959", 1699446815815, testBNBMainVault, testBNBWallet, "67.30133036", "BNB", "OUT:48406A5D484EF9558C9956134F45C4ED5C2B7D90173C15F7C8759E687C8A039A"),
				// RUNE switch: linked straight to the native THOR address.
				bnbBeaconTestRow("5317C4E8B1A1E5E7B2FB1E7AB0D3A1C76E8B0E0B8DCE6E1E4FBB49C0B0B0B0B0", 1660000000000, testBNBWallet, testBNBMainVault, "3260014.00000000", "RUNE-B1A", "SWITCH:"+testTHORSwitch),
				// BEPSwap outbound: the inbound it answers is looked up.
				bnbBeaconTestRow(bepswapOut, 1611643500000, testBNBVault, testBNBWallet, "4.20000000", "BUSD-BD1", "OUTBOUND:"+bepswapIn),
				// Failed transfers are skipped.
				func() map[string]any {
					row := bnbBeaconTestRow("FA11ED00EE6C9131C3FDD991AC32CD40E3DBDD9CA2449105A19C426ABCF0E4BC", 1600000000000, testBNBWallet, testBNBExchange, "1", "BNB", "")
					row["code"] = 393
					return row
				}(),
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"txNums": len(rows), "txArray": rows})
		case "/api/v1/tx":
			lookups++
			if r.URL.Query().Get("txHash") != bepswapIn {
				t.Errorf("unexpected tx lookup: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(bnbBeaconTestRow(bepswapIn, 1611643440374, testBNBSwapper, testBNBVault, "505.00000000", "RUNE-B1A", "SWAP:BNB.BUSD-BD1:"+testBNBWallet+":83644493893"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	transfers, truncated, err := app.fetchBNBBeaconTransfers(context.Background(), testBNBWallet, time.Unix(0, 0).UTC(), time.Now().UTC(), 3)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if lookups != 1 {
		t.Fatalf("expected 1 outbound lookup, got %d", lookups)
	}
	byKey := map[string]externalTransfer{}
	for _, transfer := range transfers {
		byKey[transfer.ActionKey+"|"+transfer.From+"|"+transfer.To] = transfer
	}
	if len(transfers) != 5 {
		t.Fatalf("expected 5 transfers, got %d: %#v", len(transfers), transfers)
	}

	plain, ok := byKey[bnbBeaconActionTransfer+"|"+testBNBWallet+"|"+testBNBExchange]
	if !ok || plain.Asset != "BNB.BNB" || plain.AmountRaw != "13468000000" || plain.ProtocolSide != "" || plain.Memo != "330563132" {
		t.Fatalf("unexpected plain transfer: %#v", plain)
	}
	out, ok := byKey[bnbBeaconActionOutbound+"|"+testBNBMainVault+"|"+testBNBWallet]
	if !ok || out.ProtocolSide != protocolSideFrom {
		t.Fatalf("unexpected mainnet outbound: %#v", out)
	}
	switchLink, ok := byKey[bnbBeaconActionSwitch+"|"+testBNBWallet+"|"+testTHORSwitch]
	if !ok || switchLink.Asset != "BNB.RUNE-B1A" || switchLink.AmountRaw != "326001400000000" {
		t.Fatalf("unexpected switch link: %#v", switchLink)
	}
	if _, ok := byKey[bnbBeaconActionOutbound+"|"+testBNBVault+"|"+testBNBWallet]; !ok {
		t.Fatalf("missing BEPSwap outbound vault leg: %#v", transfers)
	}
	swap, ok := byKey[bnbBeaconActionSwap+"|"+testBNBSwapper+"|"+testBNBWallet]
	if !ok || swap.TxID != bepswapIn || swap.Asset != "BNB.RUNE-B1A" || swap.AmountRaw != "50500000000" {
		t.Fatalf("unexpected BEPSwap swap link: %#v", swap)
	}
}

func TestFetchBNBBeaconTransfersDepositAndMultiSend(t *testing.T) {
	const multiHash = "B0B0B0B0EE6C9131C3FDD991AC32CD40E3DBDD9CA2449105A19C426ABCF0E4BC"
	app := newBNBBeaconTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/txs":
			multi := bnbBeaconTestRow(multiHash, 1600000100000, testBNBVault, "", "", "", "")
			delete(multi, "value")
			multi["hasChildren"] = 1
			rows := []map[string]any{
				bnbBeaconTestRow("D0D0D0D0EE6C9131C3FDD991AC32CD40E3DBDD9CA2449105A19C426ABCF0E4BC", 1600000000000, testBNBSwapper, testBNBVault, "505", "RUNE-B1A", "SWAP:BNB.BNB:"+testBNBWallet),
				multi,
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"txNums": len(rows), "txArray": rows})
		case "/api/v1/sub-tx-list":
			if r.URL.Query().Get("txHash") != multiHash {
				t.Errorf("unexpected sub-tx query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalNum": 2,
				"subTxDtoList": []map[string]any{
					{"fromAddr": testBNBVault, "toAddr": testBNBSwapper, "value": json.Number("1.5"), "txAsset": "BNB"},
					{"fromAddr": testBNBVault, "toAddr": testBNBWallet, "value": json.Number("2"), "txAsset": "BNB"},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	transfers, _, err := app.fetchBNBBeaconTransfers(context.Background(), testBNBSwapper, time.Unix(0, 0).UTC(), time.Now().UTC(), 1)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	keys := map[string]externalTransfer{}
	for _, transfer := range transfers {
		keys[transfer.ActionKey+"|"+transfer.From+"|"+transfer.To+"|"+transfer.AmountRaw] = transfer
	}
	if len(transfers) != 3 {
		t.Fatalf("expected deposit, swap link and one multi-send leg, got %#v", transfers)
	}
	deposit, ok := keys[bnbBeaconActionDeposit+"|"+testBNBSwapper+"|"+testBNBVault+"|50500000000"]
	if !ok || deposit.ProtocolSide != protocolSideTo || deposit.ActionLabel != "THORChain Deposit (SWAP)" {
		t.Fatalf("unexpected deposit: %#v", transfers)
	}
	if _, ok := keys[bnbBeaconActionSwap+"|"+testBNBSwapper+"|"+testBNBWallet+"|50500000000"]; !ok {
		t.Fatalf("missing memo-destination swap link: %#v", transfers)
	}
	leg, ok := keys[bnbBeaconActionTransfer+"|"+testBNBVault+"|"+testBNBSwapper+"|150000000"]
	if !ok || leg.TxID != multiHash || leg.Time.UnixMilli() != 1600000100000 {
		t.Fatalf("unexpected multi-send leg: %#v", transfers)
	}
}

func TestFetchBNBBeaconTransfersWalksBackByEndTime(t *testing.T) {
	var endTimes []string
	app := newBNBBeaconTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		endTimes = append(endTimes, r.URL.Query().Get("endTime"))
		end, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		rows := []map[string]any{}
		for i := 0; i < 100 && end-int64(i) > 1000; i++ {
			ts := end - int64(i)
			rows = append(rows, bnbBeaconTestRow("H"+strconv.FormatInt(ts, 10), ts, testBNBWallet, testBNBExchange, "1", "BNB", ""))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"txNums": 1000, "txArray": rows})
	})

	end := time.UnixMilli(100000).UTC()
	transfers, truncated, err := app.fetchBNBBeaconTransfers(context.Background(), testBNBWallet, time.Unix(0, 0).UTC(), end, 2)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncation after maxPages full pages")
	}
	if len(endTimes) != 2 || endTimes[0] != "100000" || endTimes[1] != "99901" {
		t.Fatalf("unexpected endTime walk: %v", endTimes)
	}
	if len(transfers) != 199 {
		t.Fatalf("expected 199 distinct transfers, got %d", len(transfers))
	}
}

func TestProjectBNBBeaconTransfersMarkVaultsAndCrossToTHOR(t *testing.T) {
	builder := &graphBuilder{
		prices:           priceBook{HasPoolData: true, PoolAssets: map[string]struct{}{"BTC.BTC": {}}},
		allowedFlowTypes: flowTypeSet([]string{"transfers", "swaps"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	deposit := externalTransfer{
		Chain: "BNB", Asset: "BNB.RUNE-B1A", AmountRaw: "50500000000",
		From: testBNBSwapper, To: testBNBVault, TxID: "D0D0", Time: time.Unix(1600000000, 0).UTC(),
		ActionKey: bnbBeaconActionDeposit, ActionLabel: "THORChain Deposit (SWAP)", ProtocolSide: protocolSideTo, Confidence: 0.99,
	}
	segments, next := builder.projectExternalTransfer(deposit, 1)
	if len(segments) != 1 {
		t.Fatalf("expected the BEP2 deposit to project despite the delisted pool, got %#v", segments)
	}
	if segments[0].Target.Kind != "inbound" || segments[0].Target.Label != "THORChain BNB Vault "+shortAddress(testBNBVault) {
		t.Fatalf("expected a labeled vault target, got %#v", segments[0].Target)
	}
	if len(next) != 1 || next[0].Address != testBNBSwapper || next[0].Chain != "BNB" {
		t.Fatalf("expected only the wallet to expand, got %#v", next)
	}

	switchLink := externalTransfer{
		Chain: "BNB", Asset: "BNB.RUNE-B1A", AmountRaw: "326001400000000",
		From: testBNBWallet, To: testTHORSwitch, TxID: "5317", Time: time.Unix(1660000000, 0).UTC(),
		ActionKey: bnbBeaconActionSwitch, ActionLabel: "RUNE Switch (BEP2 → Native)", Confidence: 0.95,
	}
	segments, next = builder.projectExternalTransfer(switchLink, 1)
	if len(segments) != 1 || segments[0].Target.Chain != "THOR" || segments[0].Source.Chain != "BNB" {
		t.Fatalf("expected a BNB → THOR switch edge, got %#v", segments)
	}
	if len(next) != 2 {
		t.Fatalf("expected both wallets to expand, got %#v", next)
	}

	swapLink := externalTransfer{
		Chain: "BNB", Asset: "BNB.RUNE-B1A", AmountRaw: "50500000000",
		From: testBNBSwapper, To: testBNBWallet, TxID: "D0D0", Time: time.Unix(1600000000, 0).UTC(),
		ActionKey: bnbBeaconActionSwap, ActionLabel: "THORChain Swap (memo destination)", Confidence: 0.9,
	}
	segments, _ = builder.projectExternalTransfer(swapLink, 1)
	if len(segments) != 1 || segments[0].ActionClass != "swaps" {
		t.Fatalf("expected a swap-class link, got %#v", segments)
	}
	builder.allowedFlowTypes = flowTypeSet([]string{"transfers"})
	if segments, _ = builder.projectExternalTransfer(swapLink, 1); len(segments) != 0 {
		t.Fatalf("expected swap links to respect the flow-type filter, got %#v", segments)
	}
}

func TestBNBBeaconChainInference(t *testing.T) {
	if got := normalizeChain("", "BNB18066NS6ADT024WNVPF2HAV9466G95AV4996GP7"); got != "BNB" {
		t.Fatalf("normalizeChain: got %q", got)
	}
	if got := normalizeAddress("BNB18066NS6ADT024WNVPF2HAV9466G95AV4996GP7"); got != testBNBWallet {
		t.Fatalf("normalizeAddress: got %q", got)
	}
	cfg := Config{ChainTrackerProviders: defaultChainTrackerProviders(), ChainTrackerCandidates: defaultChainTrackerCandidates()}
	if got := cfg.trackerProvidersForChain("BNB"); len(got) != 1 || got[0] != "bnbbeacon" {
		t.Fatalf("providers: got %v", got)
	}
	if got := defiLlamaCoin("BNB.RUNE-B1A"); got != "coingecko:thorchain" {
		t.Fatalf("defiLlamaCoin: got %q", got)
	}
}

// TestBNBBeaconLiveArchive checks the real explorer archive. It needs network
// access, so it only runs with CHAIN_ANALYSIS_LIVE_BNB=1.
func TestBNBBeaconLiveArchive(t *testing.T) {
	if os.Getenv("CHAIN_ANALYSIS_LIVE_BNB") != "1" {
		t.Skip("set CHAIN_ANALYSIS_LIVE_BNB=1 to query explorer.bnbchain.org")
	}
	app := &App{
		cfg:           Config{BNBBeaconExplorerURL: "https://explorer.bnbchain.org"},
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		trackerHealth: newTrackerHealthStore(),
	}
	transfers, truncated, err := app.fetchBNBBeaconTransfers(context.Background(), testBNBWallet, time.Unix(0, 0).UTC(), time.Now().UTC(), 2)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	counts := map[string]int{}
	for _, transfer := range transfers {
		counts[transfer.ActionKey]++
		if transfer.ActionKey == bnbBeaconActionSwitch {
			t.Logf("switch: %s -> %s %s %s", transfer.From, transfer.To, transfer.AmountRaw, transfer.Asset)
		}
	}
	t.Logf("transfers=%d truncated=%v by action=%v", len(transfers), truncated, counts)
	if len(transfers) == 0 || counts[bnbBeaconActionSwitch] == 0 || counts[bnbBeaconActionOutbound] == 0 {
		t.Fatalf("expected transfers, a switch link and outbounds, got %v", counts)
	}
}
