package app

import (
	"context"
	"encoding/json"
	"os"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleIndexDisablesCaching(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<!doctype html><html><body>ok</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	app := &App{
		cfg: Config{
			StaticDir: dir,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected cache control %q", got)
	}
	if got := strings.TrimSpace(rec.Body.String()); got == "" {
		t.Fatal("expected index body")
	}
}

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

func TestHandleActionByTxIDFallsBackToLegacySource(t *testing.T) {
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
			t.Fatalf("unexpected txid query %q", got)
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

	req := httptest.NewRequest(http.MethodGet, "/api/actions/legacytx", nil)
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
	if resp.TxID != "LEGACYTX" {
		t.Fatalf("unexpected tx_id %q", resp.TxID)
	}
	if len(resp.Actions) != 1 {
		t.Fatalf("expected legacy lookup action, got %#v", resp.Actions)
	}
	if got := resp.Actions[0].Height; got != "4473241" {
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

func TestExtractRebondLinkFromMidgardBondActionParsesNativeRebondMetadata(t *testing.T) {
	action := midgardAction{
		Height: "23034924",
		Type:   "rebond",
		In: []midgardActionLeg{{
			Address: "thor1oldbond",
			TxID:    "rebondtx",
		}},
		Metadata: midgardActionMetadata{
			Rebond: &midgardRebondMetadata{
				Memo:           "REBOND:thor1validator:thor1newbond:31018731723188",
				NodeAddress:    "thor1validator",
				NewBondAddress: "thor1newbond",
			},
		},
	}

	link, ok := extractRebondLinkFromMidgardBondAction(action)
	if !ok {
		t.Fatal("expected native rebond metadata to produce a link")
	}
	if link.Height != 23034924 {
		t.Fatalf("unexpected height %d", link.Height)
	}
	if link.TxID != "REBONDTX" {
		t.Fatalf("unexpected tx id %q", link.TxID)
	}
	if link.NodeAddress != "thor1validator" {
		t.Fatalf("unexpected node address %q", link.NodeAddress)
	}
	if link.OldBondAddress != "thor1oldbond" {
		t.Fatalf("unexpected old bond address %q", link.OldBondAddress)
	}
	if link.NewBondAddress != "thor1newbond" {
		t.Fatalf("unexpected new bond address %q", link.NewBondAddress)
	}
}

func TestExtractRebondLinkFromMidgardBondActionIgnoresBondProviderTx(t *testing.T) {
	action := midgardAction{
		Height: "22796618",
		Type:   "bond",
		In: []midgardActionLeg{{
			Address: "thor1sender",
			TxID:    "bondtx",
		}},
		Metadata: midgardActionMetadata{
			Bond: &midgardBondMetadata{
				Memo:        "BOND:thor1node:thor1provider:2000",
				NodeAddress: "thor1node",
				Provider:    "thor1provider",
				Fee:         "2000",
			},
		},
	}

	if link, ok := extractRebondLinkFromMidgardBondAction(action); ok {
		t.Fatalf("expected bond provider tx to be ignored, got %#v", link)
	}
}

func TestExtractRebondLinksFromMidgardBondActionsReplacesStaleTxRows(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	app := &App{db: db}
	ctx := context.Background()
	if err := insertRebondLink(ctx, db, RebondLink{
		Height:         1,
		TxID:           "BONDTX",
		NodeAddress:    "thor1node",
		OldBondAddress: "thor1old",
		NewBondAddress: "thor1new",
		Data:           map[string]any{"memo": "BOND:thor1node:thor1new"},
	}); err != nil {
		t.Fatalf("insert stale rebond link: %v", err)
	}

	app.extractRebondLinksFromMidgardBondActions(ctx, []midgardAction{{
		Type:   "bond",
		Status: "success",
		In: []midgardActionLeg{{
			Address: "thor1sender",
			TxID:    "bondtx",
		}},
		Metadata: midgardActionMetadata{
			Bond: &midgardBondMetadata{
				Memo:        "BOND:thor1node:thor1provider:2000",
				NodeAddress: "thor1node",
				Provider:    "thor1provider",
				Fee:         "2000",
			},
		},
	}})

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rebond_links WHERE tx_id = ?`, "BONDTX").Scan(&count); err != nil {
		t.Fatalf("count rebond links: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected stale rebond link to be removed, got %d rows", count)
	}
}

func TestHandleWalletBondsIncludesRebondActionsAndLinks(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.URL.Query().Get("address")); got != "thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu" {
			t.Fatalf("unexpected address query %q", got)
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: []midgardAction{{
				Date:   "1759108019365569722",
				Height: "23034924",
				Type:   "rebond",
				Status: "success",
				In: []midgardActionLeg{{
					Address: "thor1mlucvrd56xrhac4zqqx6yku84a6e5edj6k8una",
					TxID:    "F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA",
					Coins:   []midgardActionCoin{{Amount: "31018731723188", Asset: "THOR.RUNE"}},
				}},
				Out: []midgardActionLeg{{
					Address: "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0",
					TxID:    "F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA",
					Coins:   []midgardActionCoin{{Amount: "31018731723188", Asset: "THOR.RUNE"}},
				}},
				Metadata: midgardActionMetadata{
					Rebond: &midgardRebondMetadata{
						Memo:           "REBOND:thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk:thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu:31018731723188",
						NodeAddress:    "thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk",
						NewBondAddress: "thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu",
					},
				},
			}},
		})
	}))
	defer upstream.Close()

	app := &App{
		cfg: Config{
			RequestTimeout: 5 * time.Second,
			MidgardTimeout: 5 * time.Second,
		},
		db:            db,
		mid:           NewThorClient([]string{upstream.URL}, 5*time.Second),
		trackerHealth: newTrackerHealthStore(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/wallets/thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu/bonds?max_pages=1", nil)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Actions     []midgardAction `json:"actions"`
		RebondLinks []RebondLink    `json:"rebond_links"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Actions) != 1 {
		t.Fatalf("expected rebond action in response, got %#v", resp.Actions)
	}
	if got := strings.ToLower(strings.TrimSpace(resp.Actions[0].Type)); got != "rebond" {
		t.Fatalf("expected rebond action type, got %q", got)
	}
	if len(resp.RebondLinks) != 1 {
		t.Fatalf("expected one rebond link, got %#v", resp.RebondLinks)
	}
	if resp.RebondLinks[0].OldBondAddress != "thor1mlucvrd56xrhac4zqqx6yku84a6e5edj6k8una" {
		t.Fatalf("unexpected old bond address %q", resp.RebondLinks[0].OldBondAddress)
	}
	if resp.RebondLinks[0].NewBondAddress != "thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu" {
		t.Fatalf("unexpected new bond address %q", resp.RebondLinks[0].NewBondAddress)
	}
}
