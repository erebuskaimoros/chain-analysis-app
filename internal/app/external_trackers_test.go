package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHumanPartsToRaw(t *testing.T) {
	cases := []struct {
		whole string
		frac  string
		want  int64
	}{
		{whole: "973", frac: "76247216", want: 97376247216},
		{whole: "1,234", frac: "5", want: 123450000000},
		{whole: "0", frac: "00000001", want: 1},
	}

	for _, tc := range cases {
		if got := humanPartsToRaw(tc.whole, tc.frac, 8); got != tc.want {
			t.Fatalf("humanPartsToRaw(%q, %q) = %d, want %d", tc.whole, tc.frac, got, tc.want)
		}
	}
}

func TestInferUTXOTransfersOutgoing(t *testing.T) {
	got := inferUTXOTransfers(
		"BTC",
		"bc1watcher",
		"tx1",
		123,
		time.Unix(100, 0).UTC(),
		map[string]int64{"bc1watcher": 2000},
		map[string]int64{"bc1dest": 1500, "bc1watcher": 400},
	)
	if len(got) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(got))
	}
	if got[0].From != "bc1watcher" || got[0].To != "bc1dest" || got[0].AmountRaw != "1500" {
		t.Fatalf("unexpected outgoing transfer: %#v", got[0])
	}
}

func TestInferUTXOTransfersIncomingInferred(t *testing.T) {
	got := inferUTXOTransfers(
		"DOGE",
		"DWatcher",
		"tx2",
		456,
		time.Unix(200, 0).UTC(),
		map[string]int64{"DSourceA": 300, "DSourceB": 700},
		map[string]int64{"DWatcher": 1000},
	)
	if len(got) != 2 {
		t.Fatalf("expected 2 inferred transfers, got %d", len(got))
	}
	if got[0].To != "DWatcher" || got[1].To != "DWatcher" {
		t.Fatalf("unexpected incoming transfers: %#v", got)
	}
}

func TestCosmosCoinToAssetAmount(t *testing.T) {
	asset, amount := cosmosCoinToAssetAmount("GAIA", "uatom", "1234567")
	if asset != "GAIA.ATOM" || amount != "123456700" {
		t.Fatalf("unexpected GAIA coin mapping: asset=%q amount=%q", asset, amount)
	}
}

func TestFetchAddressLiveHoldingsUTXO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/address/bc1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chain_stats": map[string]any{
					"funded_txo_sum": 1200000000,
					"spent_txo_sum":  200000000,
				},
				"mempool_stats": map[string]any{
					"funded_txo_sum": 50000000,
					"spent_txo_sum":  10000000,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			UtxoTrackerURLs: map[string]string{"BTC": server.URL},
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "BTC", "bc1watch", priceBook{
		AssetUSD: map[string]float64{"BTC.BTC": 100000},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if holdings[0].Asset != "BTC.BTC" {
		t.Fatalf("unexpected asset: %q", holdings[0].Asset)
	}
	if holdings[0].AmountRaw != "1040000000" {
		t.Fatalf("unexpected raw amount: %q", holdings[0].AmountRaw)
	}
	if holdings[0].USDSpot <= 0 {
		t.Fatalf("expected positive USD spot, got %f", holdings[0].USDSpot)
	}
}

func TestFetchAddressLiveHoldingsUTXOFallsBackAcrossConfiguredEndpoints(t *testing.T) {
	var badHits atomic.Int64
	var goodHits atomic.Int64

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badHits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"overloaded"}`))
	}))
	defer badServer.Close()

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goodHits.Add(1)
		switch r.URL.Path {
		case "/address/bc1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chain_stats": map[string]any{
					"funded_txo_sum": 900000000,
					"spent_txo_sum":  100000000,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer goodServer.Close()

	app := &App{
		cfg: Config{
			UtxoTrackerURLs: map[string]string{
				"BTC": badServer.URL + "|" + goodServer.URL,
			},
		},
		httpClient:    goodServer.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "BTC", "bc1watch", priceBook{
		AssetUSD: map[string]float64{"BTC.BTC": 100000},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings with endpoint fallback: %v", err)
	}
	if len(holdings) != 1 || holdings[0].AmountRaw != "800000000" {
		t.Fatalf("unexpected holdings after endpoint fallback: %#v", holdings)
	}
	if badHits.Load() == 0 {
		t.Fatal("expected failing endpoint to be attempted")
	}
	if goodHits.Load() == 0 {
		t.Fatal("expected healthy endpoint to be attempted")
	}
}

func TestFetchAddressLiveHoldingsDOGEFallsBackToDogedHTML(t *testing.T) {
	var apiHits atomic.Int64
	var htmlHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/address/D8U1PL31zA8Q8LucFKfVDbTWUJs2EoLGDc":
			apiHits.Add(1)
			http.NotFound(w, r)
		case "/address/D8U1PL31zA8Q8LucFKfVDbTWUJs2EoLGDc":
			htmlHits.Add(1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `
<html><body><script>
var balances = JSON.parse('{"main":{"utxos":[{"satsAmount":125000000},{"satsAmount":350000000}]}}');
</script></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			UtxoTrackerURLs: map[string]string{"DOGE": server.URL},
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "DOGE", "D8U1PL31zA8Q8LucFKfVDbTWUJs2EoLGDc", priceBook{
		AssetUSD: map[string]float64{"DOGE.DOGE": 0.1},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if apiHits.Load() == 0 {
		t.Fatal("expected API endpoint to be attempted first")
	}
	if htmlHits.Load() == 0 {
		t.Fatal("expected HTML fallback endpoint to be attempted")
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if holdings[0].Asset != "DOGE.DOGE" {
		t.Fatalf("unexpected asset: %q", holdings[0].Asset)
	}
	if holdings[0].AmountRaw != "475000000" {
		t.Fatalf("unexpected raw amount: %q", holdings[0].AmountRaw)
	}
	if holdings[0].USDSpot <= 0 {
		t.Fatalf("expected positive USD spot, got %f", holdings[0].USDSpot)
	}
}

func TestFetchAddressLiveHoldingsEtherscan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "1230000000000000000",
			})
		case "addresstokenbalance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result": []map[string]any{
					{
						"TokenAddress":  "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
						"TokenSymbol":   "USDC",
						"TokenDivisor":  "6",
						"TokenQuantity": "2500000",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	usdcAsset := "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48"
	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "ETH", "0xwatch", priceBook{
		AssetUSD: map[string]float64{
			"ETH.ETH": 2000,
			usdcAsset: 1,
		},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings))
	}
	byAsset := map[string]liveHoldingValue{}
	for _, holding := range holdings {
		byAsset[holding.Asset] = holding
	}
	if got := byAsset["ETH.ETH"].AmountRaw; got != "123000000" {
		t.Fatalf("unexpected ETH raw amount: %q", got)
	}
	if got := byAsset[usdcAsset].AmountRaw; got != "250000000" {
		t.Fatalf("unexpected USDC raw amount: %q", got)
	}
}

func TestFetchAddressLiveHoldingsEtherscanTokenLookupFailureStillReturnsNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "balance" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "1",
			"message": "OK",
			"result":  "1230000000000000000",
		})
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "ETH", "0xwatch", priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if got := holdings[0].Asset; got != "ETH.ETH" {
		t.Fatalf("unexpected asset: %q", got)
	}
	if got := holdings[0].AmountRaw; got != "123000000" {
		t.Fatalf("unexpected raw amount: %q", got)
	}
}

func TestFetchAddressLiveHoldingsEtherscanEmptyTokenBalancesSkipsFallbackLookups(t *testing.T) {
	var (
		addresstokenbalanceHits atomic.Int64
		tokentxHits             atomic.Int64
		tokenbalanceHits        atomic.Int64
		ethplorerHits           atomic.Int64
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getAddressInfo/"):
			ethplorerHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"tokens": []any{}})
		case r.URL.Query().Get("action") == "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "1230000000000000000",
			})
		case r.URL.Query().Get("action") == "addresstokenbalance":
			addresstokenbalanceHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		case r.URL.Query().Get("action") == "tokentx":
			tokentxHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		case r.URL.Query().Get("action") == "tokenbalance":
			tokenbalanceHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "0",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
			EthplorerAPIURL: server.URL,
			EthplorerAPIKey: "test-key",
		},
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerFeatures: newTrackerFeatureStore(),
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "ETH", "0xwatch", priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if len(holdings) != 1 || holdings[0].Asset != "ETH.ETH" {
		t.Fatalf("unexpected holdings: %#v", holdings)
	}
	if addresstokenbalanceHits.Load() != 1 {
		t.Fatalf("expected addresstokenbalance once, got %d", addresstokenbalanceHits.Load())
	}
	if tokentxHits.Load() != 0 {
		t.Fatalf("expected no tokentx fallback call, got %d", tokentxHits.Load())
	}
	if tokenbalanceHits.Load() != 0 {
		t.Fatalf("expected no tokenbalance fallback call, got %d", tokenbalanceHits.Load())
	}
	if ethplorerHits.Load() != 0 {
		t.Fatalf("expected no ethplorer fallback call, got %d", ethplorerHits.Load())
	}
}

func TestFetchAddressLiveHoldingsEtherscanFallsBackToTokenBalanceQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "1230000000000000000",
			})
		case "addresstokenbalance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "0",
				"message": "NOTOK",
				"result":  "endpoint requires paid plan",
			})
		case "tokentx":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result": []map[string]any{
					{
						"contractAddress": "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
						"tokenSymbol":     "USDC",
						"tokenName":       "USD Coin",
						"tokenDecimal":    "6",
					},
				},
			})
		case "tokenbalance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "2500000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	usdcAsset := "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48"
	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "ETH", "0xwatch", priceBook{
		AssetUSD: map[string]float64{
			"ETH.ETH": 2000,
			usdcAsset: 1,
		},
	})
	if err != nil {
		t.Fatalf("fetch address live holdings: %v", err)
	}
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings))
	}
	byAsset := map[string]liveHoldingValue{}
	for _, holding := range holdings {
		byAsset[holding.Asset] = holding
	}
	if got := byAsset[usdcAsset].AmountRaw; got != "250000000" {
		t.Fatalf("unexpected USDC raw amount: %q", got)
	}
}

func TestFetchAddressLiveHoldingsEtherscanCachesUnsupportedTokenBalanceEndpoint(t *testing.T) {
	var (
		addresstokenbalanceHits atomic.Int64
		tokentxHits             atomic.Int64
		tokenbalanceHits        atomic.Int64
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "1230000000000000000",
			})
		case "addresstokenbalance":
			addresstokenbalanceHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "0",
				"message": "NOTOK",
				"result":  "endpoint requires paid plan",
			})
		case "tokentx":
			tokentxHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result": []map[string]any{
					{
						"contractAddress": "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
						"tokenSymbol":     "USDC",
						"tokenName":       "USD Coin",
						"tokenDecimal":    "6",
					},
				},
			})
		case "tokenbalance":
			tokenbalanceHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "2500000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerFeatures: newTrackerFeatureStore(),
	}

	for i := 0; i < 2; i++ {
		holdings, err := app.fetchAddressLiveHoldings(context.Background(), "ETH", "0xwatch", priceBook{
			AssetUSD: map[string]float64{
				"ETH.ETH": 2000,
				"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48": 1,
			},
		})
		if err != nil {
			t.Fatalf("fetch address live holdings call %d: %v", i+1, err)
		}
		if len(holdings) != 2 {
			t.Fatalf("expected 2 holdings on call %d, got %#v", i+1, holdings)
		}
	}

	if addresstokenbalanceHits.Load() != 1 {
		t.Fatalf("expected unsupported addresstokenbalance probe to happen once, got %d", addresstokenbalanceHits.Load())
	}
	if tokentxHits.Load() != 2 {
		t.Fatalf("expected tokentx fallback twice, got %d", tokentxHits.Load())
	}
	if tokenbalanceHits.Load() != 2 {
		t.Fatalf("expected tokenbalance fallback twice, got %d", tokenbalanceHits.Load())
	}
}

func TestFetchAddressLiveHoldingsTHORIncludesBankLPAndBond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/bank/v1beta1/balances/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"balances": []map[string]any{
					{"denom": "rune", "amount": "100000000"},
					{"denom": "btc-btc", "amount": "50000000"},
					{"denom": "x/ruji", "amount": "800000000"},
				},
			})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"bond_providers": map[string]any{
						"providers": []map[string]any{
							{"bond_address": "thor1watch", "bond": "300000000"},
						},
					},
				},
			})
		case "/member/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pools": []map[string]any{
					{"pool": "BASE.ETH", "liquidityUnits": "25000000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		thor:          NewThorClient([]string{server.URL}, 10*time.Second),
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	prices := priceBook{
		AssetUSD: map[string]float64{
			"THOR.RUNE": 2,
			"THOR.RUJI": 0.25,
			"BTC.BTC":   10000,
			"BASE.ETH":  1000,
		},
		PoolAssets: map[string]struct{}{
			"BASE.ETH": {},
		},
		PoolSnapshots: map[string]MidgardPool{
			protocolPoolSnapshotKey(sourceProtocolTHOR, "BASE.ETH"): {
				Asset:          "BASE.ETH",
				AssetDepth:     "400000000",
				RuneDepth:      "800000000",
				LiquidityUnits: "100000000",
			},
		},
		HasPoolData: true,
	}

	holdings, err := app.fetchAddressLiveHoldings(context.Background(), "THOR", "thor1watch", prices)
	if err != nil {
		t.Fatalf("fetch THOR live holdings: %v", err)
	}
	if len(holdings) != 4 {
		t.Fatalf("expected 4 compacted holdings, got %#v", holdings)
	}

	byAsset := map[string]liveHoldingValue{}
	for _, holding := range holdings {
		byAsset[holding.Asset] = holding
	}

	if got := byAsset["THOR.RUNE"].AmountRaw; got != "600000000" {
		t.Fatalf("expected aggregated THOR.RUNE amount 600000000, got %q", got)
	}
	if got := byAsset["BASE.ETH"].AmountRaw; got != "100000000" {
		t.Fatalf("expected LP BASE.ETH share 100000000, got %q", got)
	}
	if got := byAsset["BTC.BTC"].AmountRaw; got != "50000000" {
		t.Fatalf("expected BTC bank balance 50000000, got %q", got)
	}
	if got := byAsset["THOR.RUJI"].AmountRaw; got != "800000000" {
		t.Fatalf("expected RUJI bank balance 800000000, got %q", got)
	}

	totalUSD := 0.0
	for _, holding := range holdings {
		totalUSD += holding.USDSpot
	}
	if totalUSD != 6014 {
		t.Fatalf("expected total USD 6014, got %f", totalUSD)
	}
}

func TestEnrichNodesWithLiveHoldingsTHORIncludesBankLPAndBond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{
				{
					Asset:          "ETH.USDC",
					Status:         "available",
					AssetDepth:     "400000000",
					RuneDepth:      "200000000",
					LiquidityUnits: "100000000",
					AssetPriceUSD:  "1",
				},
				{
					Asset:          "BASE.ETH",
					Status:         "available",
					AssetDepth:     "400000000",
					RuneDepth:      "800000000",
					LiquidityUnits: "100000000",
					AssetPriceUSD:  "1000",
				},
			})
		case "/cosmos/bank/v1beta1/balances/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"balances": []map[string]any{
					{"denom": "rune", "amount": "100000000"},
					{"denom": "btc-btc", "amount": "50000000"},
					{"denom": "x/ruji", "amount": "800000000"},
				},
			})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"bond_providers": map[string]any{
						"providers": []map[string]any{
							{"bond_address": "thor1watch", "bond": "300000000"},
						},
					},
				},
			})
		case "/member/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pools": []map[string]any{
					{"pool": "BASE.ETH", "liquidityUnits": "25000000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		thor:          NewThorClient([]string{server.URL}, 10*time.Second),
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "external_address:thor1watch:external:1",
			Kind:    "external_address",
			Label:   "thor1watch",
			Chain:   "THOR",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1watch"},
		},
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{
			"THOR.RUNE": 2,
			"THOR.RUJI": 0.25,
			"BTC.BTC":   10000,
			"BASE.ETH":  1000,
		},
		PoolAssets: map[string]struct{}{
			"BASE.ETH": {},
		},
		PoolSnapshots: map[string]MidgardPool{
			protocolPoolSnapshotKey(sourceProtocolTHOR, "BASE.ETH"): {
				Asset:          "BASE.ETH",
				Status:         "available",
				AssetDepth:     "400000000",
				RuneDepth:      "800000000",
				LiquidityUnits: "100000000",
			},
		},
		HasPoolData: true,
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := nodes[0].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected THOR live holdings available=true, got %#v", got)
	}
	if got, ok := nodes[0].Metrics["live_holdings_usd_spot"].(float64); !ok || got != 6014 {
		t.Fatalf("expected THOR live_holdings_usd_spot=6014, got %#v", nodes[0].Metrics["live_holdings_usd_spot"])
	}
}

func TestEnrichNodesWithLiveHoldingsKeepsLargeTHORBatchWithinBudget(t *testing.T) {
	const nodeCount = 40
	const lookupLatency = 250 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Path == "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case strings.HasPrefix(r.URL.Path, "/cosmos/bank/v1beta1/balances/thor1batch"):
			time.Sleep(lookupLatency)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"balances": []map[string]any{
					{"denom": "rune", "amount": "100000000"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/member/thor1batch"):
			time.Sleep(lookupLatency)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pools": []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout: 20 * time.Second,
			MidgardTimeout: 20 * time.Second,
		},
		thor:            NewThorClient([]string{server.URL}, 20*time.Second),
		mid:             NewThorClient([]string{server.URL}, 20*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := make([]FlowNode, 0, nodeCount)
	for i := 0; i < nodeCount; i++ {
		address := fmt.Sprintf("thor1batch%02d", i)
		nodes = append(nodes, FlowNode{
			ID:      "external_address:" + address + ":external:1",
			Kind:    "external_address",
			Label:   address,
			Chain:   "THOR",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": address},
		})
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"THOR.RUNE": 2},
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings for large THOR batch: %#v", warnings)
	}
	for i, node := range nodes {
		if got := node.Metrics["live_holdings_available"]; got != true {
			t.Fatalf("expected THOR node %d live holdings available=true, got %#v", i, got)
		}
		if got := node.Metrics["live_holdings_status"]; got != "available" {
			t.Fatalf("expected THOR node %d live holdings status=available, got %#v", i, got)
		}
		if got, ok := node.Metrics["live_holdings_usd_spot"].(float64); !ok || got != 2 {
			t.Fatalf("expected THOR node %d live_holdings_usd_spot=2, got %#v", i, node.Metrics["live_holdings_usd_spot"])
		}
	}
}

func TestEnrichNodesWithLiveHoldingsInlineModeSkipsAddressLookups(t *testing.T) {
	var bankHits atomic.Int64
	var memberHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Path == "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case strings.HasPrefix(r.URL.Path, "/cosmos/bank/v1beta1/balances/"):
			bankHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"balances": []map[string]any{}})
		case strings.HasPrefix(r.URL.Path, "/member/"):
			memberHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"pools": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout: 20 * time.Second,
			MidgardTimeout: 20 * time.Second,
		},
		thor:            NewThorClient([]string{server.URL}, 20*time.Second),
		mid:             NewThorClient([]string{server.URL}, 20*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "external_address:thor1inlineonly:external:1",
			Kind:    "external_address",
			Label:   "thor1inlineonly",
			Chain:   "THOR",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1inlineonly"},
		},
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"THOR.RUNE": 2},
	}, protocolDirectory{}, false)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings for inline-only live holdings: %#v", warnings)
	}
	if got := bankHits.Load(); got != 0 {
		t.Fatalf("expected inline-only mode to skip bank lookups, got %d", got)
	}
	if got := memberHits.Load(); got != 0 {
		t.Fatalf("expected inline-only mode to skip LP lookups, got %d", got)
	}
	if _, ok := nodes[0].Metrics["live_holdings_status"]; ok {
		t.Fatalf("expected inline-only mode to leave address live holdings unset, got %#v", nodes[0].Metrics["live_holdings_status"])
	}
}

func TestRefreshActorTrackerLiveHoldingsTHORAddressNodesPopulateLiveValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{
				{
					Asset:          "BASE.ETH",
					Status:         "available",
					AssetDepth:     "400000000",
					RuneDepth:      "800000000",
					LiquidityUnits: "100000000",
					AssetPriceUSD:  "1000",
				},
			})
		case "/cosmos/bank/v1beta1/balances/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"balances": []map[string]any{
					{"denom": "rune", "amount": "100000000"},
					{"denom": "x/ruji", "amount": "200000000"},
				},
			})
		case "/member/thor1watch":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pools": []map[string]any{
					{"pool": "BASE.ETH", "liquidityUnits": "25000000"},
				},
			})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"node_address": "thor1validator",
					"status":       "Active",
					"total_bond":   "500000000",
					"bond_providers": map[string]any{
						"providers": []map[string]any{
							{"bond_address": "thor1watch", "bond": "300000000"},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		thor:          NewThorClient([]string{server.URL}, 10*time.Second),
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "actor_address:thor1watch:actor_address:1",
			Kind:    "actor_address",
			Label:   "Treasury Addr thor1watch",
			Chain:   "THOR",
			Stage:   "actor_address",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1watch"},
		},
		{
			ID:      "external_address:thor1watch:external:1",
			Kind:    "external_address",
			Label:   "thor1watch",
			Chain:   "THOR",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1watch"},
		},
		{
			ID:      "bond_address:thor1watch:node_bond:1",
			Kind:    "bond_address",
			Label:   "Bond Wallet thor1watch",
			Chain:   "THOR",
			Stage:   "node_bond",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1watch"},
		},
	}

	warnings, err := app.refreshActorTrackerLiveHoldings(context.Background(), nodes)
	if err != nil {
		t.Fatalf("refresh THOR address live holdings: %v", err)
	}
	for _, node := range nodes {
		if got := node.Metrics["live_holdings_available"]; got != true {
			t.Fatalf("expected %s live holdings available=true, got %#v", node.Kind, got)
		}
		if got := node.Metrics["live_holdings_status"]; got != "available" {
			t.Fatalf("expected %s live holdings status=available, got %#v", node.Kind, got)
		}
		if got, ok := node.Metrics["live_holdings_usd_spot"].(float64); !ok || got <= 0 {
			t.Fatalf("expected %s live_holdings_usd_spot to be positive, got %#v", node.Kind, node.Metrics["live_holdings_usd_spot"])
		}
	}
	if len(warnings) > 1 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestEnrichNodesWithLiveHoldingsValidatorNodeUsesTotalBond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"node_address": "thor1validator",
					"status":       "Active",
					"total_bond":   "450000000",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		thor:          NewThorClient([]string{server.URL}, 10*time.Second),
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "node:thor1validator:node_bond:1",
			Kind:    "node",
			Label:   "Validator thor1validator",
			Chain:   "THOR",
			Stage:   "node_bond",
			Depth:   1,
			Metrics: map[string]any{"address": "thor1validator"},
		},
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"THOR.RUNE": 2},
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := nodes[0].Metrics["node_total_bond"]; got != "450000000" {
		t.Fatalf("expected node_total_bond=450000000, got %#v", got)
	}
	if got := nodes[0].Metrics["node_status"]; got != "Active" {
		t.Fatalf("expected node_status=Active, got %#v", got)
	}
	if got := nodes[0].Label; got != "Validator thor1validator" {
		t.Fatalf("expected validator label, got %#v", got)
	}
	if got := nodes[0].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected validator live holdings available=true, got %#v", got)
	}
	if got := nodes[0].Metrics["live_holdings_status"]; got != "available" {
		t.Fatalf("expected validator live holdings status=available, got %#v", got)
	}
	if got, ok := nodes[0].Metrics["live_holdings_usd_spot"].(float64); !ok || got != 9 {
		t.Fatalf("expected validator live_holdings_usd_spot=9, got %#v", nodes[0].Metrics["live_holdings_usd_spot"])
	}
}

func TestEnrichNodesWithLiveHoldingsWhitelistedNodeNotMarkedValidator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"node_address": "thor1whitelisted",
					"status":       "Whitelisted",
					"total_bond":   "0",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		thor:          NewThorClient([]string{server.URL}, 10*time.Second),
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{{
		ID:      "node:thor1whitelisted:node_bond:1",
		Kind:    "node",
		Label:   "Node thor1whitelisted",
		Chain:   "THOR",
		Stage:   "node_bond",
		Depth:   1,
		Metrics: map[string]any{"address": "thor1whitelisted"},
	}}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"THOR.RUNE": 2},
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := nodes[0].Metrics["node_status"]; got != "Whitelisted" {
		t.Fatalf("expected node_status=Whitelisted, got %#v", got)
	}
	if got := nodes[0].Label; got != "Whitelisted Node "+shortAddress("thor1whitelisted") {
		t.Fatalf("expected whitelisted label, got %#v", got)
	}
}

func TestEVMChainIDSupportsARB(t *testing.T) {
	if got := evmChainID("ARB"); got != "42161" {
		t.Fatalf("expected ARB chain id 42161, got %q", got)
	}
	if got := evmNativeAsset("ARB"); got != "ARB.ETH" {
		t.Fatalf("expected ARB native asset ARB.ETH, got %q", got)
	}
}

func TestInferRadixTransfersSplitsOutgoingXRDByCounterparty(t *testing.T) {
	watched := "account_rdx1watch000000000000000000000000000000000000000000000"
	alpha := "account_rdx1alpha00000000000000000000000000000000000000000000"
	beta := "account_rdx1beta000000000000000000000000000000000000000000000"
	transfers := inferRadixTransfers(watched, "XRD-TX", 42, time.Unix(1_700_000_000, 0).UTC(), []struct {
		EntityAddress   string `json:"entity_address"`
		ResourceAddress string `json:"resource_address"`
		BalanceChange   string `json:"balance_change"`
	}{
		{EntityAddress: watched, ResourceAddress: radixMainnetXRDResourceAddr, BalanceChange: "-3.00000000"},
		{EntityAddress: alpha, ResourceAddress: radixMainnetXRDResourceAddr, BalanceChange: "+1.00000000"},
		{EntityAddress: beta, ResourceAddress: radixMainnetXRDResourceAddr, BalanceChange: "+2.00000000"},
	})

	if len(transfers) != 2 {
		t.Fatalf("expected 2 inferred transfers, got %#v", transfers)
	}
	if got := decimalAmountToGraphRaw("1.23456789"); got != "123456789" {
		t.Fatalf("expected decimal normalization to 1e8 graph units, got %q", got)
	}
	if got := transfers[0].AmountRaw; got != "100000000" {
		t.Fatalf("expected first inferred amount 100000000, got %q", got)
	}
	if got := transfers[1].AmountRaw; got != "200000000" {
		t.Fatalf("expected second inferred amount 200000000, got %q", got)
	}
	if transfers[0].From != normalizeAddress(watched) || transfers[1].From != normalizeAddress(watched) {
		t.Fatalf("expected watched address as transfer source, got %#v", transfers)
	}
	if transfers[0].To != normalizeAddress(alpha) || transfers[1].To != normalizeAddress(beta) {
		t.Fatalf("expected sorted counterparties alpha/beta, got %#v", transfers)
	}
}

func TestEnrichNodesWithLiveHoldingsIncludesExternalNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "500000000000000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "external_address:0xwatch:external:1",
			Kind:    "external_address",
			Label:   "0xwatch",
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "0xwatch"},
		},
	}
	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := nodes[0].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected live holdings available=true, got %#v", got)
	}
	if got, ok := nodes[0].Metrics["live_holdings_usd_spot"].(float64); !ok || got <= 0 {
		t.Fatalf("expected positive live_holdings_usd_spot, got %#v", nodes[0].Metrics["live_holdings_usd_spot"])
	}
}

func TestEnrichNodesWithLiveHoldingsIgnoresParentCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "500000000000000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "external_address:0xwatch:external:1",
			Kind:    "external_address",
			Label:   "0xwatch",
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "0xwatch"},
		},
	}

	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	warnings := app.enrichNodesWithLiveHoldings(parentCtx, nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := nodes[0].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected live holdings available=true, got %#v", got)
	}
	if got := nodes[0].Metrics["live_holdings_status"]; got != "available" {
		t.Fatalf("expected live holdings status=available, got %#v", got)
	}
}

func TestEnrichNodesWithLiveHoldingsSlowProviderDoesNotLoseFastBucket(t *testing.T) {
	var (
		ethHits atomic.Int64
		btcHits atomic.Int64
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Path == "/address/bc1fast":
			btcHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chain_stats": map[string]any{
					"funded_txo_sum": 125000000,
					"spent_txo_sum":  25000000,
				},
				"mempool_stats": map[string]any{
					"funded_txo_sum": 0,
					"spent_txo_sum":  0,
				},
			})
		case r.URL.Query().Get("action") == "balance":
			ethHits.Add(1)
			select {
			case <-r.Context().Done():
				return
			case <-time.After(150 * time.Millisecond):
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "500000000000000000",
			})
		case r.URL.Query().Get("action") == "addresstokenbalance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  75 * time.Millisecond,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
			UtxoTrackerURLs: map[string]string{"BTC": server.URL},
		},
		mid:             NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := make([]FlowNode, 0, 7)
	for i := 0; i < 6; i++ {
		addr := "0xslow" + string(rune('a'+i))
		nodes = append(nodes, FlowNode{
			ID:      "external_address:" + addr + ":external:1",
			Kind:    "external_address",
			Label:   addr,
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": addr},
		})
	}
	nodes = append(nodes, FlowNode{
		ID:      "external_address:bc1fast:external:1",
		Kind:    "external_address",
		Label:   "bc1fast",
		Chain:   "BTC",
		Stage:   "external",
		Depth:   1,
		Metrics: map[string]any{"address": "bc1fast"},
	})

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{
			"ETH.ETH": 2000,
			"BTC.BTC": 100000,
		},
	}, protocolDirectory{}, true)
	if len(warnings) == 0 {
		t.Fatalf("expected mixed-provider warnings, got none")
	}
	if got := btcHits.Load(); got == 0 {
		t.Fatalf("expected BTC tracker to be queried, got %d hits", got)
	}
	if got := ethHits.Load(); got == 0 {
		t.Fatalf("expected ETH tracker to be queried, got %d hits", got)
	}
	if got := nodes[len(nodes)-1].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected BTC live holdings available=true, got %#v", got)
	}
	if got := nodes[len(nodes)-1].Metrics["live_holdings_status"]; got != "available" {
		t.Fatalf("expected BTC live holdings status=available, got %#v", got)
	}
	for i := 0; i < len(nodes)-1; i++ {
		if got := nodes[i].Metrics["live_holdings_status"]; got != "pending" {
			t.Fatalf("expected slow ETH node %d live holdings status=pending, got %#v", i, got)
		}
		if got := nodes[i].Metrics["live_holdings_available"]; got != false {
			t.Fatalf("expected slow ETH node %d live holdings available=false, got %#v", i, got)
		}
	}
}

func TestEnrichNodesWithLiveHoldingsBoundsTotalBatchDuration(t *testing.T) {
	var balanceHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			balanceHits.Add(1)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  150 * time.Millisecond,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:             NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := make([]FlowNode, 0, 6)
	for i := 0; i < 6; i++ {
		addr := "0xslow" + string(rune('a'+i))
		nodes = append(nodes, FlowNode{
			ID:      "external_address:" + addr + ":external:1",
			Kind:    "external_address",
			Label:   addr,
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": addr},
		})
	}

	started := time.Now()
	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)
	elapsed := time.Since(started)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected live holdings enrichment to stop within a bounded batch budget, took %s", elapsed)
	}
	if balanceHits.Load() == 0 {
		t.Fatal("expected slow balance lookups to be attempted")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for canceled live holdings lookups, got none")
	}
}

func TestEnrichNodesWithLiveHoldingsMarksBudgetSkippedNodesPending(t *testing.T) {
	var balanceHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			balanceHits.Add(1)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  150 * time.Millisecond,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:             NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := make([]FlowNode, 0, 4)
	for i := 0; i < 4; i++ {
		addr := "0xslow" + string(rune('a'+i))
		nodes = append(nodes, FlowNode{
			ID:      "external_address:" + addr + ":external:1",
			Kind:    "external_address",
			Label:   addr,
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": addr},
		})
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)

	if balanceHits.Load() == 0 {
		t.Fatal("expected slow balance lookups to be attempted")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for canceled live holdings lookups, got none")
	}
	for i, node := range nodes {
		if got := node.Metrics["live_holdings_status"]; got != "pending" {
			t.Fatalf("expected budget-skipped node %d live holdings status=pending, got %#v", i, got)
		}
		if got := node.Metrics["live_holdings_available"]; got != false {
			t.Fatalf("expected budget-skipped node %d live holdings available=false, got %#v", i, got)
		}
	}
}

func TestEnrichNodesWithLiveHoldingsMarksSkippedBackgroundPhaseNodesPending(t *testing.T) {
	var balanceHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			balanceHits.Add(1)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  150 * time.Millisecond,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:             NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerThrottle: newTrackerThrottleStore(),
	}

	nodes := []FlowNode{
		{
			ID:       "actor_address:0xpriority:actor_address:1",
			Kind:     "actor_address",
			Label:    "Priority",
			Chain:    "ETH",
			Stage:    "actor_address",
			Depth:    0,
			ActorIDs: []int64{2},
			Metrics:  map[string]any{"address": "0xpriority"},
		},
		{
			ID:      "external_address:0xbackground:external:1",
			Kind:    "external_address",
			Label:   "Background",
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "0xbackground"},
		},
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)

	if balanceHits.Load() == 0 {
		t.Fatal("expected phase-one balance lookups to be attempted")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for canceled live holdings lookups, got none")
	}
	for i, node := range nodes {
		if got := node.Metrics["live_holdings_status"]; got != "pending" {
			t.Fatalf("expected skipped node %d live holdings status=pending, got %#v", i, got)
		}
		if got := node.Metrics["live_holdings_available"]; got != false {
			t.Fatalf("expected skipped node %d live holdings available=false, got %#v", i, got)
		}
	}
}

func TestPlanAddressLookupPhasesPrioritizesActorOwnedAndExplorerNodes(t *testing.T) {
	nodes := []FlowNode{
		{
			ID:      "external-address",
			Kind:    "external_address",
			Label:   "0xexternal",
			Chain:   "ETH",
			Stage:   "external",
			Depth:   2,
			Metrics: map[string]any{"address": "0xexternal"},
		},
		{
			ID:       "actor-address",
			Kind:     "actor_address",
			Label:    "Treasury Addr",
			Chain:    "ETH",
			Stage:    "actor_address",
			Depth:    1,
			ActorIDs: []int64{2},
			Metrics:  map[string]any{"address": "0xactor"},
		},
		{
			ID:      "explorer-target",
			Kind:    "explorer_target",
			Label:   "bc1seed",
			Chain:   "BTC",
			Stage:   "seed",
			Depth:   0,
			Metrics: map[string]any{"address": "bc1seed"},
		},
		{
			ID:      "doge-external",
			Kind:    "external_address",
			Label:   "Dexternal",
			Chain:   "DOGE",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "Dexternal"},
		},
	}
	tasks := map[string]*liveHoldingAddressLookupTask{
		"ETH|0xexternal": {
			key:       "ETH|0xexternal",
			chain:     "ETH",
			address:   "0xexternal",
			provider:  "etherscan",
			bucketKey: "etherscan|ETH",
			refs:      []liveHoldingNodeRef{{index: 0}},
		},
		"ETH|0xactor": {
			key:       "ETH|0xactor",
			chain:     "ETH",
			address:   "0xactor",
			provider:  "etherscan",
			bucketKey: "etherscan|ETH",
			refs:      []liveHoldingNodeRef{{index: 1}},
		},
		"BTC|bc1seed": {
			key:       "BTC|bc1seed",
			chain:     "BTC",
			address:   "bc1seed",
			provider:  "utxo",
			bucketKey: "utxo|BTC",
			refs:      []liveHoldingNodeRef{{index: 2}},
		},
		"DOGE|Dexternal": {
			key:       "DOGE|Dexternal",
			chain:     "DOGE",
			address:   "Dexternal",
			provider:  "utxo",
			bucketKey: "utxo|DOGE",
			refs:      []liveHoldingNodeRef{{index: 3}},
		},
	}

	phases := planAddressLookupPhases(nodes, tasks)
	if len(phases) != 2 {
		t.Fatalf("expected two lookup phases, got %d", len(phases))
	}
	if got := liveHoldingTaskKeys(phases[0]); !sameStrings(got, []string{"BTC|bc1seed", "ETH|0xactor"}) {
		t.Fatalf("priority phase = %v, want explorer/actor tasks first", got)
	}
	if got := liveHoldingTaskKeys(phases[1]); !sameStrings(got, []string{"DOGE|Dexternal", "ETH|0xexternal"}) {
		t.Fatalf("background phase = %v, want external tail last", got)
	}
}

func liveHoldingTaskKeys(tasks []liveHoldingAddressLookupTask) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.key)
	}
	return out
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestEnrichNodesWithLiveHoldingsSkipsPoolSnapshotWithoutPoolNodes(t *testing.T) {
	var (
		poolHits    atomic.Int64
		balanceHits atomic.Int64
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			poolHits.Add(1)
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			balanceHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "500000000000000000",
			})
		case r.URL.Query().Get("action") == "addresstokenbalance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  time.Second,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	nodes := []FlowNode{
		{
			ID:      "external_address:0xwatch:external:1",
			Kind:    "external_address",
			Label:   "0xwatch",
			Chain:   "ETH",
			Stage:   "external",
			Depth:   1,
			Metrics: map[string]any{"address": "0xwatch"},
		},
	}

	warnings := app.enrichNodesWithLiveHoldings(context.Background(), nodes, priceBook{
		AssetUSD: map[string]float64{"ETH.ETH": 2000},
	}, protocolDirectory{}, true)

	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := poolHits.Load(); got != 0 {
		t.Fatalf("expected no pool snapshot lookups without pool nodes, got %d", got)
	}
	if got := balanceHits.Load(); got == 0 {
		t.Fatalf("expected address balance lookup to run, got %d", got)
	}
}

func TestLiveHoldingsLookupTimeoutHonorsSmallerRequestTimeout(t *testing.T) {
	app := &App{
		cfg: Config{
			RequestTimeout: 150 * time.Millisecond,
		},
	}

	if got := app.liveHoldingsLookupTimeout("etherscan", "ETH"); got != 150*time.Millisecond {
		t.Fatalf("expected etherscan timeout to clamp to request timeout, got %s", got)
	}
	if got := app.liveHoldingsLookupTimeout("utxo", "BTC"); got != 150*time.Millisecond {
		t.Fatalf("expected utxo timeout to clamp to request timeout, got %s", got)
	}
}

func TestHandleActorTrackerLiveHoldings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		case r.URL.Query().Get("action") == "balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  "500000000000000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:  time.Second,
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		mid:           NewThorClient([]string{server.URL}, 10*time.Second),
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	body, err := json.Marshal(ActorTrackerLiveHoldingsRequest{
		Nodes: []FlowNode{
			{
				ID:      "external_address:0xwatch:external:1",
				Kind:    "external_address",
				Label:   "0xwatch",
				Chain:   "ETH",
				Stage:   "external",
				Depth:   1,
				Metrics: map[string]any{"address": "0xwatch"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/actor-tracker/live-holdings", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	app.handleActorTrackerLiveHoldings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp ActorTrackerLiveHoldingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node in response, got %d", len(resp.Nodes))
	}
	if got := resp.Nodes[0].Metrics["live_holdings_available"]; got != true {
		t.Fatalf("expected refreshed live_holdings_available=true, got %#v", got)
	}
	if got := resp.Nodes[0].Metrics["live_holdings_status"]; got != "available" {
		t.Fatalf("expected refreshed live_holdings_status=available, got %#v", got)
	}
	assets, ok := resp.Nodes[0].Metrics["live_holdings_assets"].([]any)
	if !ok || len(assets) == 0 {
		t.Fatalf("expected refreshed live_holdings_assets to be present, got %#v", resp.Nodes[0].Metrics["live_holdings_assets"])
	}
}

func TestNewEVMTokenTransferDistinguishesContracts(t *testing.T) {
	first, ok := newEVMTokenTransfer("ETH", "0xSender", "0xRecipient", "0xTX1", 1, time.Unix(1, 0).UTC(), "1000000", "6", "USDC", "USD Coin", "0xaaaa", 1)
	if !ok {
		t.Fatal("expected first token transfer to be accepted")
	}
	second, ok := newEVMTokenTransfer("ETH", "0xSender", "0xRecipient", "0xTX2", 2, time.Unix(2, 0).UTC(), "1000000", "6", "USDC", "USD Coin", "0xbbbb", 1)
	if !ok {
		t.Fatal("expected second token transfer to be accepted")
	}
	if first.Asset == second.Asset {
		t.Fatalf("expected distinct token assets, got %q", first.Asset)
	}
	if first.TokenAddress != "0xaaaa" || second.TokenAddress != "0xbbbb" {
		t.Fatalf("unexpected token addresses: %#v %#v", first, second)
	}
}

func TestFetchEtherscanTransfersTokenMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		payload := map[string]any{
			"status":  "1",
			"message": "OK",
			"result":  []map[string]any{},
		}
		if action == "tokentx" {
			payload["result"] = []map[string]any{{
				"blockNumber":     "123",
				"timeStamp":       "1700000000",
				"hash":            "0xTX1",
				"from":            "0xwatch",
				"to":              "0xrecv",
				"value":           "1000000",
				"isError":         "0",
				"contractAddress": "0xabc",
				"tokenName":       "USD Coin",
				"tokenSymbol":     "USDC",
				"tokenDecimal":    "6",
			}}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, truncated, err := app.fetchEVMTransfers(context.Background(), "etherscan", "ETH", "0xwatch", time.Unix(1699999990, 0).UTC(), time.Unix(1700000010, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch etherscan transfers: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(transfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(transfers))
	}
	got := transfers[0]
	if got.Asset != "ETH.USDC-0XABC" {
		t.Fatalf("unexpected asset key: %q", got.Asset)
	}
	if got.AssetKind != "fungible_token" || got.TokenStandard != "erc20" {
		t.Fatalf("unexpected token metadata: %#v", got)
	}
	if got.TokenAddress != "0xabc" || got.TokenSymbol != "USDC" || got.TokenName != "USD Coin" || got.TokenDecimals != 6 {
		t.Fatalf("unexpected token transfer metadata: %#v", got)
	}
}

func TestFetchBlockscoutTransfersTokenMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		payload := map[string]any{
			"status":  "1",
			"message": "OK",
			"result":  []map[string]any{},
		}
		if action == "tokentx" {
			payload["result"] = []map[string]any{{
				"blockNumber":     "456",
				"timeStamp":       "1700000000",
				"hash":            "0xTX2",
				"from":            "0xwatch",
				"to":              "0xrecv",
				"value":           "2500000",
				"isError":         "0",
				"contractAddress": "0xdef",
				"tokenName":       "USD Base",
				"tokenSymbol":     "USDbC",
				"tokenDecimal":    "6",
			}}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			BlockscoutAPIURLs: map[string]string{"BASE": server.URL},
			BlockscoutAPIKeys: map[string]string{"BASE": "block-key"},
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, _, err := app.fetchEVMTransfers(context.Background(), "blockscout", "BASE", "0xwatch", time.Unix(1699999990, 0).UTC(), time.Unix(1700000010, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch blockscout transfers: %v", err)
	}
	if len(transfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(transfers))
	}
	if transfers[0].Asset != "BASE.USDBC-0XDEF" {
		t.Fatalf("unexpected asset key: %q", transfers[0].Asset)
	}
}

func TestFetchEtherscanTransfersHandlesNoTokenTransfersString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		payload := map[string]any{
			"status":  "0",
			"message": "No transactions found",
			"result":  "No transactions found",
		}
		if action == "tokentx" {
			payload["message"] = "No token transfers found"
			payload["result"] = "No token transfers found"
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, truncated, err := app.fetchEVMTransfers(context.Background(), "etherscan", "ETH", "0xwatch", time.Unix(1699999990, 0).UTC(), time.Unix(1700000010, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch etherscan transfers: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(transfers) != 0 {
		t.Fatalf("expected no transfers, got %#v", transfers)
	}
}

func TestFetchEtherscanTransfersAddsBlockRangeLookupsOncePerWindow(t *testing.T) {
	var (
		blockLookupHits    atomic.Int64
		txlistHits         atomic.Int64
		txlistInternalHits atomic.Int64
		tokentxHits        atomic.Int64
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("action") {
		case "getblocknobytime":
			blockLookupHits.Add(1)
			closest := q.Get("closest")
			result := "100"
			if closest == "after" {
				result = "200"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  result,
			})
		case "txlist":
			txlistHits.Add(1)
			if got := q.Get("startblock"); got != "100" {
				t.Fatalf("expected txlist startblock=100, got %q", got)
			}
			if got := q.Get("endblock"); got != "200" {
				t.Fatalf("expected txlist endblock=200, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		case "txlistinternal":
			txlistInternalHits.Add(1)
			if got := q.Get("startblock"); got != "100" {
				t.Fatalf("expected txlistinternal startblock=100, got %q", got)
			}
			if got := q.Get("endblock"); got != "200" {
				t.Fatalf("expected txlistinternal endblock=200, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		case "tokentx":
			tokentxHits.Add(1)
			if got := q.Get("startblock"); got != "100" {
				t.Fatalf("expected tokentx startblock=100, got %q", got)
			}
			if got := q.Get("endblock"); got != "200" {
				t.Fatalf("expected tokentx endblock=200, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:       server.Client(),
		trackerHealth:    newTrackerHealthStore(),
		trackerBlockNums: newTrackerBlockNumberStore(),
	}

	transfers, truncated, err := app.fetchEVMTransfers(
		withTrackerRequestMeta(context.Background(), "etherscan", "ETH"),
		"etherscan",
		"ETH",
		"0xwatch",
		time.Unix(1699999990, 0).UTC(),
		time.Unix(1700000010, 0).UTC(),
		1,
	)
	if err != nil {
		t.Fatalf("fetch etherscan transfers: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(transfers) != 0 {
		t.Fatalf("expected no transfers, got %#v", transfers)
	}
	if blockLookupHits.Load() != 2 {
		t.Fatalf("expected 2 block range lookups, got %d", blockLookupHits.Load())
	}
	if txlistHits.Load() != 1 || txlistInternalHits.Load() != 1 || tokentxHits.Load() != 1 {
		t.Fatalf("unexpected action hit counts: txlist=%d internal=%d tokentx=%d", txlistHits.Load(), txlistInternalHits.Load(), tokentxHits.Load())
	}
}

func TestFetchEtherscanTransfersCachesBlockRangeLookupsAcrossCalls(t *testing.T) {
	var blockLookupHits atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("action") {
		case "getblocknobytime":
			blockLookupHits.Add(1)
			result := "100"
			if q.Get("closest") == "after" {
				result = "200"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  result,
			})
		case "txlist", "txlistinternal", "tokentx":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result":  []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:       server.Client(),
		trackerHealth:    newTrackerHealthStore(),
		trackerBlockNums: newTrackerBlockNumberStore(),
	}

	ctx := withTrackerRequestMeta(context.Background(), "etherscan", "ETH")
	start := time.Unix(1699999990, 0).UTC()
	end := time.Unix(1700000010, 0).UTC()
	for i := 0; i < 2; i++ {
		transfers, truncated, err := app.fetchEVMTransfers(ctx, "etherscan", "ETH", "0xwatch", start, end, 1)
		if err != nil {
			t.Fatalf("fetch etherscan transfers call %d: %v", i+1, err)
		}
		if truncated {
			t.Fatalf("did not expect truncation on call %d", i+1)
		}
		if len(transfers) != 0 {
			t.Fatalf("expected no transfers on call %d, got %#v", i+1, transfers)
		}
	}

	if blockLookupHits.Load() != 2 {
		t.Fatalf("expected cached block range lookups to total 2, got %d", blockLookupHits.Load())
	}
}

func TestFetchEtherscanTransfersHandlesRateLimitResultString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		switch action {
		case "txlist":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "1",
				"message": "OK",
				"result": []map[string]any{{
					"blockNumber":      "123",
					"timeStamp":        "1700000000",
					"hash":             "0xTX1",
					"from":             "0xwatch",
					"to":               "0xrecv",
					"value":            "1000000000000000000",
					"isError":          "0",
					"txreceipt_status": "1",
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "0",
				"message": "NOTOK",
				"result":  "Max rate limit reached, please use API Key for higher rate limit",
			})
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			EtherscanAPIURL: server.URL,
			EtherscanAPIKey: "test-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, truncated, err := app.fetchEVMTransfers(context.Background(), "etherscan", "ETH", "0xwatch", time.Unix(1699999990, 0).UTC(), time.Unix(1700000010, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch etherscan transfers: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncation when etherscan returns rate-limit string payload")
	}
	if len(transfers) != 1 {
		t.Fatalf("expected txlist transfer to survive partial rate-limit responses, got %#v", transfers)
	}
	if transfers[0].TxID != "0xTX1" {
		t.Fatalf("unexpected transfer txid: %#v", transfers[0])
	}
}

func TestFetchAvaCloudTransfersTokenMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactions": []map[string]any{{
				"txHash":         "0xavax1",
				"blockNumber":    "99",
				"blockTimestamp": "2024-03-09T00:00:00Z",
				"txStatus":       "1",
				"erc20Transfers": []map[string]any{{
					"from":            map[string]any{"address": "0xwatch"},
					"to":              map[string]any{"address": "0xrecv"},
					"value":           "1000000",
					"contractAddress": "0xabc",
					"erc20Token": map[string]any{
						"address":  "0xabc",
						"symbol":   "USDC",
						"name":     "USD Coin",
						"decimals": "6",
					},
				}},
			}},
		})
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			AvaCloudBaseURL: server.URL,
			AvaCloudAPIKey:  "glacier-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, truncated, err := app.fetchEVMTransfers(context.Background(), "avacloud", "AVAX", "0xwatch", time.Unix(1709942000, 0).UTC(), time.Unix(1709942600, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch avacloud transfers: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
	if len(transfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(transfers))
	}
	if transfers[0].Asset != "AVAX.USDC-0XABC" || transfers[0].TokenAddress != "0xabc" {
		t.Fatalf("unexpected avacloud transfer: %#v", transfers[0])
	}
}

func TestFetchNodeRealTransfersTokenMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Handle batch JSON-RPC requests (array of objects).
		var batch []map[string]any
		if err := json.Unmarshal(body, &batch); err == nil && len(batch) > 0 {
			var responses []map[string]any
			for _, payload := range batch {
				method := payload["method"].(string)
				id := payload["id"]
				if method == "nr_getTransactionDetail" {
					responses = append(responses, map[string]any{
						"jsonrpc": "2.0",
						"id":      id,
						"result": map[string]any{
							"blockNum":       "100",
							"blockTimeStamp": "1710000000",
							"receiptsStatus": "1",
							"tokenTransfers": []map[string]any{{
								"from":      "0xwatch",
								"to":        "0xrecv",
								"value":     "1000000",
								"asset":     "USDC",
								"tokenName": "USD Coin",
								"rawContract": map[string]any{
									"address": "0xabc",
									"decimal": "6",
								},
							}},
						},
					})
				}
			}
			_ = json.NewEncoder(w).Encode(responses)
			return
		}
		// Handle single JSON-RPC requests.
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode nodereal payload: %v", err)
		}
		method := payload["method"].(string)
		switch method {
		case "nr_getTransactionByAddress":
			params := payload["params"].([]any)[0].(map[string]any)
			addressType := params["addressType"].(string)
			result := map[string]any{"transfers": []map[string]any{}}
			if addressType == "from" {
				result["transfers"] = []map[string]any{{
					"hash":           "0xnodetx",
					"category":       "20",
					"blockNum":       "100",
					"blockTimeStamp": "1710000000",
					"receiptsStatus": "1",
				}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
		default:
			t.Fatalf("unexpected nodereal method: %s", method)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			NodeRealBSCURL: server.URL,
			NodeRealAPIKey: "node-key",
		},
		httpClient:    server.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	transfers, _, err := app.fetchEVMTransfers(context.Background(), "nodereal", "BSC", "0xwatch", time.Unix(1709999900, 0).UTC(), time.Unix(1710000100, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("fetch nodereal transfers: %v", err)
	}
	if len(transfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(transfers))
	}
	if transfers[0].Asset != "BSC.USDC-0XABC" || transfers[0].TokenAddress != "0xabc" {
		t.Fatalf("unexpected nodereal transfer: %#v", transfers[0])
	}
}

func TestFetchSolanaAddressLiveHoldingsRotatesAcrossConfiguredRPCs(t *testing.T) {
	var firstHits atomic.Int64
	var secondHits atomic.Int64

	newRPCServer := func(counter *atomic.Int64) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload["method"] != "getBalance" {
				http.Error(w, "unexpected method", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"value": 1230000000,
				},
			})
		}))
	}

	firstServer := newRPCServer(&firstHits)
	defer firstServer.Close()
	secondServer := newRPCServer(&secondHits)
	defer secondServer.Close()

	app := &App{
		cfg: Config{
			SolanaRPCURL: firstServer.URL + "|" + secondServer.URL,
		},
		httpClient:    firstServer.Client(),
		trackerHealth: newTrackerHealthStore(),
	}

	for i := 0; i < 2; i++ {
		holdings, err := app.fetchSolanaAddressLiveHoldings(context.Background(), "SomeSolAddress", priceBook{
			AssetUSD: map[string]float64{"SOL.SOL": 100},
		})
		if err != nil {
			t.Fatalf("fetch solana live holdings #%d: %v", i+1, err)
		}
		if len(holdings) != 1 || holdings[0].AmountRaw != "123000000" {
			t.Fatalf("unexpected solana holdings on call %d: %#v", i+1, holdings)
		}
	}

	if firstHits.Load() != 1 {
		t.Fatalf("expected first solana RPC to receive 1 request, got %d", firstHits.Load())
	}
	if secondHits.Load() != 1 {
		t.Fatalf("expected second solana RPC to receive 1 request, got %d", secondHits.Load())
	}
}

func TestExternalTransferCacheSupersetLookup(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	transfers := []externalTransfer{{
		Chain:       "ETH",
		Asset:       "ETH.USDC-0XABC",
		AmountRaw:   "100000000",
		From:        "0xfrom",
		To:          "0xto",
		TxID:        "0xTX",
		Height:      10,
		Time:        time.Unix(100, 0).UTC(),
		ActionKey:   "tracker.evm.token_transfer",
		ActionLabel: "ETH Token Transfer",
	}}

	if err := insertExternalTransferCache(context.Background(), db, "etherscan", "ETH", "0xwatch", 50, 150, 2, false, transfers); err != nil {
		t.Fatalf("insert external transfer cache: %v", err)
	}

	got, truncated, found, err := lookupExternalTransferCache(context.Background(), db, "etherscan", "ETH", "0xwatch", 90, 110, 1, true)
	if err != nil {
		t.Fatalf("lookup external transfer cache: %v", err)
	}
	if !found || truncated {
		t.Fatalf("expected non-truncated cache hit, found=%v truncated=%v", found, truncated)
	}
	if len(got) != 1 || got[0].TxID != "0xTX" {
		t.Fatalf("unexpected cached transfers: %#v", got)
	}
}
