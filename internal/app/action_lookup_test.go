package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLookupActionByTxIDSuppressesShadowSendLookupAction(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.URL.Query().Get("txid")); got != "OUTTX" {
			t.Errorf("unexpected txid query %q", got)
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: []midgardAction{
				{
					Date:   "1769206297553545093",
					Height: "24649420",
					Type:   "swap",
					Status: "success",
					In: []midgardActionLeg{{
						Address: "thor1sender",
						TxID:    "INTX",
						Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
					}},
					Out: []midgardActionLeg{{
						Address: "0xrecipient",
						TxID:    "OUTTX",
						Coins:   []midgardActionCoin{{Amount: "58247165323", Asset: "BSC.USDT-0X55D398326F99059FF775485246999027B3197955"}},
					}},
					Pools: []string{"BSC.USDT-0X55D398326F99059FF775485246999027B3197955"},
				},
				{
					Date:   "1769206297553545093",
					Height: "24649420",
					Type:   "send",
					Status: "success",
					In: []midgardActionLeg{{
						Address: "thor1sender",
						TxID:    "INTX",
						Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
					}},
					Out: []midgardActionLeg{
						{
							Address: "thor1transit",
							TxID:    "INTX",
							Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
						},
						{
							Address: "0xrecipient",
							TxID:    "OUTTX",
							Coins:   []midgardActionCoin{{Amount: "58247165323", Asset: "BSC.USDT-0X55D398326F99059FF775485246999027B3197955"}},
						},
					},
				},
			},
		})
	}))
	defer upstream.Close()

	app := &App{
		cfg: Config{
			RequestTimeout: 5 * time.Second,
			MidgardTimeout: 5 * time.Second,
		},
		mid: NewThorClient([]string{upstream.URL}, 5*time.Second),
	}

	result, err := app.LookupActionByTxID(context.Background(), "OUTTX")
	if err != nil {
		t.Fatalf("lookup action: %v", err)
	}
	if result.TxID != "OUTTX" {
		t.Fatalf("unexpected tx_id %q", result.TxID)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected shadow send to be suppressed, got %#v", result.Actions)
	}
	if got := strings.ToLower(strings.TrimSpace(result.Actions[0].Type)); got != "swap" {
		t.Fatalf("expected canonical action to remain swap, got %q", got)
	}
}

func TestLookupActionByTxIDFallsBackToLegacySource(t *testing.T) {
	midgard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{}})
	}))
	defer midgard.Close()

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.URL.Query().Get("txid")); got != "LEGACYTX" {
			t.Errorf("unexpected txid query %q", got)
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: []midgardAction{
				{
					Date:   "1637020800000000000",
					Height: "4473241",
					Type:   "send",
					Status: "success",
					In: []midgardActionLeg{{
						Address: "thor1sender",
						TxID:    "LEGACYTX",
						Coins:   []midgardActionCoin{{Amount: "250000000000000", Asset: "THOR.RUNE"}},
					}},
					Out: []midgardActionLeg{{
						Address: "thor1recipient",
						TxID:    "LEGACYTX",
						Coins:   []midgardActionCoin{{Amount: "250000000000000", Asset: "THOR.RUNE"}},
					}},
				},
			},
		})
	}))
	defer legacy.Close()

	app := &App{
		cfg: Config{
			RequestTimeout:        5 * time.Second,
			MidgardTimeout:        5 * time.Second,
			LegacyActionEndpoints: []string{legacy.URL},
		},
		mid:           NewThorClient([]string{midgard.URL}, 5*time.Second),
		legacyActions: NewThorClient([]string{legacy.URL}, 5*time.Second),
	}

	result, err := app.LookupActionByTxID(context.Background(), "legacytx")
	if err != nil {
		t.Fatalf("lookup action: %v", err)
	}
	if result.TxID != "LEGACYTX" {
		t.Fatalf("unexpected tx_id %q", result.TxID)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected legacy lookup action, got %#v", result.Actions)
	}
	if got := result.Actions[0].Height; got != "4473241" {
		t.Fatalf("unexpected legacy action height %q", got)
	}
}

func TestCanonicalizeMidgardLookupActionsKeepsStandaloneSend(t *testing.T) {
	actions := canonicalizeMidgardLookupActions([]midgardAction{
		{
			Date:   "1769206297553545093",
			Height: "24649420",
			Type:   "send",
			Status: "success",
			In: []midgardActionLeg{{
				Address: "thor1sender",
				TxID:    "INTX",
				Coins:   []midgardActionCoin{{Amount: "100000000", Asset: "THOR.RUNE"}},
			}},
			Out: []midgardActionLeg{{
				Address: "thor1recipient",
				TxID:    "INTX",
				Coins:   []midgardActionCoin{{Amount: "100000000", Asset: "THOR.RUNE"}},
			}},
		},
	})
	if len(actions) != 1 {
		t.Fatalf("expected standalone send to be preserved, got %#v", actions)
	}
}
