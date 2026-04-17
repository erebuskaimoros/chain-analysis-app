package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataCachesDeduplicateRequestsAndCloneResults(t *testing.T) {
	var inboundCalls atomic.Int32
	var poolCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/thorchain/inbound_addresses":
			inboundCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"chain":   "ETH",
					"address": "0xinbound",
					"router":  "0xrouter",
				},
			})
		case "/pools":
			poolCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			_ = json.NewEncoder(w).Encode([]MidgardPool{
				{
					Asset:         "ETH.USDC",
					Status:        "available",
					AssetDepth:    "400000000",
					RuneDepth:     "200000000",
					AssetPriceUSD: "1",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		cfg: Config{
			RequestTimeout: time.Second,
			MidgardTimeout: time.Second,
		},
		thor: NewThorClient([]string{server.URL}, time.Second),
		mid:  NewThorClient([]string{server.URL}, time.Second),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := app.loadProtocolDirectory(ctx)
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := app.buildPriceBook(ctx)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected cache load error: %v", err)
		}
	}

	if got := inboundCalls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream inbound-address request, got %d", got)
	}
	if got := poolCalls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream pool request, got %d", got)
	}

	dirOne, err := app.loadProtocolDirectory(ctx)
	if err != nil {
		t.Fatalf("reload protocol directory: %v", err)
	}
	dirOne.AddressKinds["mutated"] = protocolAddress{Kind: "known", Label: "mutated"}

	dirTwo, err := app.loadProtocolDirectory(ctx)
	if err != nil {
		t.Fatalf("reload cloned protocol directory: %v", err)
	}
	if _, exists := dirTwo.AddressKinds["mutated"]; exists {
		t.Fatalf("expected protocol directory cache to return cloned maps, found mutation %#v", dirTwo.AddressKinds["mutated"])
	}

	bookOne, err := app.buildPriceBook(ctx)
	if err != nil {
		t.Fatalf("reload price book: %v", err)
	}
	bookOne.AssetUSD["MUTATED"] = 123

	bookTwo, err := app.buildPriceBook(ctx)
	if err != nil {
		t.Fatalf("reload cloned price book: %v", err)
	}
	if got := bookTwo.AssetUSD["MUTATED"]; got != 0 {
		t.Fatalf("expected price book cache to return cloned maps, got %f", got)
	}

	if got := inboundCalls.Load(); got != 1 {
		t.Fatalf("expected cached protocol directory reads to avoid new requests, got %d", got)
	}
	if got := poolCalls.Load(); got != 1 {
		t.Fatalf("expected cached price book reads to avoid new requests, got %d", got)
	}
}
