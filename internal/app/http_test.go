package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleActionByTxIDSuppressesShadowSendLookupAction(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.URL.Query().Get("txid")); got != "OUTTX" {
			t.Fatalf("unexpected txid query %q", got)
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

	req := httptest.NewRequest(http.MethodGet, "/api/actions/OUTTX", nil)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		TxID    string          `json:"tx_id"`
		Actions []midgardAction `json:"actions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TxID != "OUTTX" {
		t.Fatalf("unexpected tx_id %q", resp.TxID)
	}
	if len(resp.Actions) != 1 {
		t.Fatalf("expected shadow send to be suppressed, got %#v", resp.Actions)
	}
	if got := strings.ToLower(strings.TrimSpace(resp.Actions[0].Type)); got != "swap" {
		t.Fatalf("expected canonical action to remain swap, got %q", got)
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
