package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveAddressExplorerSeedsFansOutAcrossSupportedEVMChains(t *testing.T) {
	protocols := protocolDirectory{
		SupportedChains: map[string]struct{}{
			"BTC":  {},
			"ETH":  {},
			"BSC":  {},
			"BASE": {},
			"AVAX": {},
			"THOR": {},
		},
	}

	seeds, err := resolveAddressExplorerSeeds(protocols, frontierAddress{
		Address: "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
		Chain:   "ETH",
	}, false)
	if err != nil {
		t.Fatalf("resolveAddressExplorerSeeds: %v", err)
	}

	var chains []string
	for _, seed := range seeds {
		chains = append(chains, seed.Chain)
	}
	sort.Strings(chains)

	want := []string{"AVAX", "BASE", "BSC", "ETH"}
	if !reflect.DeepEqual(chains, want) {
		t.Fatalf("unexpected chains: got %v want %v", chains, want)
	}
}

func TestResolveAddressExplorerSeedsKeepsExplicitChainSingleSeed(t *testing.T) {
	protocols := protocolDirectory{
		SupportedChains: map[string]struct{}{
			"ETH":  {},
			"BASE": {},
		},
	}

	seeds, err := resolveAddressExplorerSeeds(protocols, frontierAddress{
		Address: "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
		Chain:   "BASE",
	}, true)
	if err != nil {
		t.Fatalf("resolveAddressExplorerSeeds: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed, got %#v", seeds)
	}
	if got := seeds[0]; got.Address != "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3" || got.Chain != "BASE" {
		t.Fatalf("unexpected seed: %#v", got)
	}
}

func TestResolveAddressExplorerSeedsInfersDOGEFromPlainAddress(t *testing.T) {
	protocols := protocolDirectory{
		SupportedChains: map[string]struct{}{
			"DOGE": {},
		},
	}

	seeds, err := resolveAddressExplorerSeeds(protocols, frontierAddress{
		Address: "D8U1PL31zA8Q8LucFKfVDbTWUJs2EoLGDc",
	}, false)
	if err != nil {
		t.Fatalf("resolveAddressExplorerSeeds: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed, got %#v", seeds)
	}
	if got := seeds[0]; got.Address != "D8U1PL31zA8Q8LucFKfVDbTWUJs2EoLGDc" || got.Chain != "DOGE" {
		t.Fatalf("unexpected seed: %#v", got)
	}
}

func TestBuildAddressExplorerPreviewRequiresDirectionForLargeHistory(t *testing.T) {
	address := "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3"
	app, cleanup := newAddressExplorerTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/actions":
			offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
			if offset < explorerDefaultBatchSize*midgardActionsPageLimit {
				actions := make([]midgardAction, midgardActionsPageLimit)
				base := offset / midgardActionsPageLimit * midgardActionsPageLimit
				for i := range actions {
					actions[i] = explorerTestAction(base+i, address, "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", "0x1111111111111111111111111111111111111111")
				}
				_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: actions})
				return
			}
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{}})
		case "/pools":
			_ = json.NewEncoder(w).Encode(explorerTestPools())
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	resp, err := app.buildAddressExplorer(context.Background(), AddressExplorerRequest{
		Address:   "ETH|" + address,
		Mode:      explorerModePreview,
		BatchSize: explorerDefaultBatchSize,
	})
	if err != nil {
		t.Fatalf("buildAddressExplorer: %v", err)
	}
	if !resp.DirectionRequired {
		t.Fatalf("expected preview to require direction, got %#v", resp)
	}
	if !resp.HasMore {
		t.Fatalf("expected preview to report more history")
	}
	if resp.Mode != explorerModePreview {
		t.Fatalf("unexpected mode %q", resp.Mode)
	}
	if len(resp.Nodes) != 0 || len(resp.Edges) != 0 || len(resp.SupportingActions) != 0 {
		t.Fatalf("preview should not return graph payload: %#v", resp)
	}
	if got := resp.ActiveChains; len(got) != 1 || got[0] != "ETH" {
		t.Fatalf("unexpected active chains: %v", got)
	}
}

func TestBuildAddressExplorerGraphCreatesExplorerTargetsPerActiveChain(t *testing.T) {
	address := "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3"
	app, cleanup := newAddressExplorerTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/actions":
			offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
			if offset > 0 {
				_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{}})
				return
			}
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{
				Actions: []midgardAction{
					explorerTestAction(1, address, "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", "0x1111111111111111111111111111111111111111"),
					explorerTestAction(2, address, "BASE.USDC-0XD9AAEC86B65D86F6A7B5E5E1B0C42FFA531710B6", "0x2222222222222222222222222222222222222222"),
				},
			})
		case "/pools":
			_ = json.NewEncoder(w).Encode(explorerTestPools())
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	resp, err := app.buildAddressExplorer(context.Background(), AddressExplorerRequest{
		Address:   address,
		Mode:      explorerModeGraph,
		Direction: "newest",
		BatchSize: 1,
	})
	if err != nil {
		t.Fatalf("buildAddressExplorer: %v", err)
	}
	if resp.Mode != explorerModeGraph {
		t.Fatalf("unexpected mode %q", resp.Mode)
	}
	gotActiveChains := append([]string(nil), resp.ActiveChains...)
	sort.Strings(gotActiveChains)
	if !reflect.DeepEqual(gotActiveChains, []string{"BASE", "ETH"}) {
		t.Fatalf("unexpected active chains: %v", resp.ActiveChains)
	}

	targetChains := map[string]struct{}{}
	for _, node := range resp.Nodes {
		if node.Kind != "explorer_target" {
			continue
		}
		if normalizeAddress(getString(node.Metrics, "address")) != address {
			continue
		}
		targetChains[node.Chain] = struct{}{}
	}
	if len(targetChains) != 2 {
		t.Fatalf("expected explorer targets on 2 chains, got %#v", resp.Nodes)
	}
	if _, ok := targetChains["ETH"]; !ok {
		t.Fatalf("missing ETH explorer target: %#v", resp.Nodes)
	}
	if _, ok := targetChains["BASE"]; !ok {
		t.Fatalf("missing BASE explorer target: %#v", resp.Nodes)
	}
}

func TestAddressExplorerRunStorageSeparatesRunTypes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	actorRunID, err := insertGraphRun(ctx, db, ActorTrackerRequest{
		ActorIDs:  []int64{7},
		StartTime: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		EndTime:   time.Now().UTC().Format(time.RFC3339),
	}, "Desk Alpha", 5, 4)
	if err != nil {
		t.Fatalf("insertGraphRun: %v", err)
	}
	explorerRunID, err := insertAddressExplorerRun(ctx, db, AddressExplorerRequest{
		Address:   "ETH|0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
		Mode:      explorerModeGraph,
		Direction: "newest",
		BatchSize: explorerDefaultBatchSize,
	}, "Explorer run", 3, 2)
	if err != nil {
		t.Fatalf("insertAddressExplorerRun: %v", err)
	}

	graphRuns, err := listGraphRuns(ctx, db)
	if err != nil {
		t.Fatalf("listGraphRuns: %v", err)
	}
	if len(graphRuns) != 1 || graphRuns[0].ID != actorRunID {
		t.Fatalf("unexpected actor runs: %#v", graphRuns)
	}

	explorerRuns, err := listAddressExplorerRuns(ctx, db)
	if err != nil {
		t.Fatalf("listAddressExplorerRuns: %v", err)
	}
	if len(explorerRuns) != 1 || explorerRuns[0].ID != explorerRunID {
		t.Fatalf("unexpected explorer runs: %#v", explorerRuns)
	}

	if err := deleteGraphRun(ctx, db, actorRunID); err != nil {
		t.Fatalf("deleteGraphRun: %v", err)
	}
	explorerRuns, err = listAddressExplorerRuns(ctx, db)
	if err != nil {
		t.Fatalf("listAddressExplorerRuns after actor delete: %v", err)
	}
	if len(explorerRuns) != 1 || explorerRuns[0].ID != explorerRunID {
		t.Fatalf("explorer run should survive actor delete: %#v", explorerRuns)
	}

	if err := deleteAddressExplorerRun(ctx, db, explorerRunID); err != nil {
		t.Fatalf("deleteAddressExplorerRun: %v", err)
	}
	explorerRuns, err = listAddressExplorerRuns(ctx, db)
	if err != nil {
		t.Fatalf("listAddressExplorerRuns after explorer delete: %v", err)
	}
	if len(explorerRuns) != 0 {
		t.Fatalf("expected no explorer runs, got %#v", explorerRuns)
	}
}

func TestEnsureGraphRunsSchemaBackfillsRunType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph-runs-migration.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE graph_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_json TEXT NOT NULL,
			actor_names TEXT NOT NULL DEFAULT '',
			node_count INTEGER NOT NULL DEFAULT 0,
			edge_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy graph_runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO graph_runs(request_json, actor_names, node_count, edge_count, created_at)
		VALUES ('{}', 'Legacy run', 1, 2, ?)
	`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert legacy graph_run: %v", err)
	}

	if err := ensureGraphRunsSchema(ctx, db); err != nil {
		t.Fatalf("ensureGraphRunsSchema: %v", err)
	}

	var runType string
	if err := db.QueryRowContext(ctx, `SELECT run_type FROM graph_runs LIMIT 1`).Scan(&runType); err != nil {
		t.Fatalf("query run_type: %v", err)
	}
	if runType != GraphRunTypeActorTracker {
		t.Fatalf("unexpected run_type %q", runType)
	}
}

func TestInitSchemaMigratesLegacyGraphRunsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph-runs-init-migration.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE graph_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_json TEXT NOT NULL,
			actor_names TEXT NOT NULL DEFAULT '',
			node_count INTEGER NOT NULL DEFAULT 0,
			edge_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy graph_runs: %v", err)
	}

	if err := initSchema(ctx, db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	var runType string
	if err := db.QueryRowContext(ctx, `SELECT run_type FROM graph_runs LIMIT 1`).Scan(&runType); err != nil && err != sql.ErrNoRows {
		t.Fatalf("query run_type after initSchema: %v", err)
	}
}

func newAddressExplorerTestApp(t *testing.T, handler http.Handler) (*App, func()) {
	t.Helper()

	server := httptest.NewServer(handler)
	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "address-explorer-test.db"),
		StaticDir:         "internal/web/static",
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		server.Close()
		t.Fatalf("new app: %v", err)
	}

	cleanup := func() {
		_ = app.Close()
		server.Close()
	}
	return app, cleanup
}

func explorerTestAction(index int, address, asset, counterparty string) midgardAction {
	ts := time.Unix(1_760_000_000+int64(index), 0).UTC()
	return midgardAction{
		Date:   strconv.FormatInt(ts.UnixNano(), 10),
		Height: strconv.Itoa(20_000_000 + index),
		Type:   "send",
		Status: "success",
		In: []midgardActionLeg{{
			Address: address,
			TxID:    fmt.Sprintf("INTX-%03d", index),
			Coins: []midgardActionCoin{{
				Asset:  asset,
				Amount: strconv.Itoa(100_000_000 + index),
			}},
		}},
		Out: []midgardActionLeg{{
			Address: counterparty,
			TxID:    fmt.Sprintf("OUTTX-%03d", index),
			Coins: []midgardActionCoin{{
				Asset:  asset,
				Amount: strconv.Itoa(100_000_000 + index),
			}},
		}},
	}
}

func explorerTestPools() []MidgardPool {
	return []MidgardPool{
		{
			Asset:         "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48",
			Status:        "available",
			AssetDepth:    "100000000",
			RuneDepth:     "100000000",
			AssetPriceUSD: "1",
		},
		{
			Asset:         "BASE.USDC-0XD9AAEC86B65D86F6A7B5E5E1B0C42FFA531710B6",
			Status:        "available",
			AssetDepth:    "100000000",
			RuneDepth:     "100000000",
			AssetPriceUSD: "1",
		},
	}
}
