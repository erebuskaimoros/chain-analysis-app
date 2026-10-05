package app

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsdAtTimeFallsBackThroughSources(t *testing.T) {
	at := time.Date(2026, 9, 28, 5, 3, 0, 0, time.UTC)
	history := newPriceHistory()
	history.set("ETH.ETH", priceIntervalHour, at.Unix()-at.Unix()%3600, 2650, priceSourcePoolHistory)
	prices := priceBook{AssetUSD: map[string]float64{"BTC.BTC": 100000}}

	cases := []struct {
		asset, amount string
		wantUSD       float64
		wantSource    string
		wantPriced    bool
	}{
		{"ETH.ETH", "10000000000", 265000, priceSourcePoolHistory, true},
		{"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", "500000000", 5, priceSourceStablePeg, true},
		{"BTC.BTC", "100000000", 100000, priceSourceSpot, true},
		{"ETH.SPAM-0X1234567890ABCDEF1234567890ABCDEF12345678", "100000000", 0, "", false},
	}
	for _, tc := range cases {
		usd, source, priced := usdAtTime(history, prices, tc.asset, tc.amount, at)
		if priced != tc.wantPriced || source != tc.wantSource || math.Abs(usd-tc.wantUSD) > 0.01 {
			t.Errorf("%s: got usd=%v source=%q priced=%v, want %v %q %v", tc.asset, usd, source, priced, tc.wantUSD, tc.wantSource, tc.wantPriced)
		}
	}
}

func TestBelowMinUSDUsesTransactionTimeAndExcludesUnpriced(t *testing.T) {
	b := &graphBuilder{minUSD: 100}
	if !b.belowMinUSD(projectedSegment{Priced: true, USDAtTime: 50, USDSpot: 500}) {
		t.Fatal("expected a flow worth $50 at the time to be filtered even though it is worth $500 today")
	}
	if b.belowMinUSD(projectedSegment{Priced: true, USDAtTime: 150, USDSpot: 5}) {
		t.Fatal("expected a flow worth $150 at the time to pass")
	}
	if !b.belowMinUSD(projectedSegment{Priced: false}) {
		t.Fatal("expected unpriced flows to be filtered by default")
	}
	b.includeUnpriced = true
	if b.belowMinUSD(projectedSegment{Priced: false}) {
		t.Fatal("expected include_unpriced to keep unpriced flows")
	}
}

func TestPreloadPriceHistoryFetchesOnceAndCaches(t *testing.T) {
	day := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	var depthCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history/depths/ETH.ETH" {
			http.NotFound(w, r)
			return
		}
		depthCalls.Add(1)
		if r.URL.Query().Get("interval") != "hour" {
			t.Errorf("expected hourly prices for a single day, got %s", r.URL.RawQuery)
		}
		var intervals []map[string]string
		for h := 0; h < 24; h++ {
			start := day.Add(time.Duration(h) * time.Hour).Unix()
			intervals = append(intervals, map[string]string{"startTime": strconv.FormatInt(start, 10), "assetPriceUSD": strconv.Itoa(2600 + h)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"intervals": intervals})
	}))
	defer server.Close()

	app, err := New(Config{DBPath: filepath.Join(t.TempDir(), "prices.db"), MidgardEndpoints: []string{server.URL}, RequestTimeout: 5 * time.Second, MidgardTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	prices := priceBook{PoolAssets: map[string]struct{}{"ETH.ETH": {}}}
	needs := priceNeeds{}
	needs.add("ETH.ETH", day.Add(5*time.Hour+3*time.Minute))

	history := newPriceHistory()
	app.preloadPriceHistory(context.Background(), history, prices, needs)
	usd, source, ok := history.priceAt("ETH.ETH", day.Add(5*time.Hour+3*time.Minute))
	if !ok || usd != 2605 || source != priceSourcePoolHistory {
		t.Fatalf("expected the 05:00 hourly price, got %v %q %v", usd, source, ok)
	}

	// A new build reads the stored points instead of fetching again.
	app.preloadPriceHistory(context.Background(), newPriceHistory(), prices, needs)
	if got := depthCalls.Load(); got != 1 {
		t.Fatalf("expected one Midgard history request, got %d", got)
	}
	if n := pricePointCount(context.Background(), app.db, "ETH.ETH"); n != 24 {
		t.Fatalf("expected 24 stored hourly points, got %d", n)
	}
}
