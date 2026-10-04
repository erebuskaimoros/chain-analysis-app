package app

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Golden graph tests replay recorded upstream traffic through the full actor
// graph pipeline and compare a canonical form of the result with a checked-in
// snapshot. They protect projection behavior during refactors.
//
//	go test ./internal/app -run Golden            replay and compare
//	go test ./internal/app -run Golden -update    replay and rewrite graph.json
//	CHAIN_ANALYSIS_RECORD=1 go test ./internal/app -run Golden -timeout 30m
//	                                              re-record from live upstreams
//
// Recording reads API keys from the repository .env file. Cassettes store
// credentials redacted, so replay runs offline with placeholder keys.

var updateGolden = flag.Bool("update", false, "rewrite golden graph snapshots from replay")

const goldenDir = "testdata/golden"

type goldenCase struct {
	Description string               `json:"description"`
	Actors      []ActorUpsertRequest `json:"actors"`
	Request     ActorTrackerRequest  `json:"request"`
}

type goldenMeta struct {
	RecordedAt  string   `json:"recorded_at"`
	KeysPresent []string `json:"keys_present"`
	Requests    int      `json:"requests"`
}

// goldenKeyFields maps .env names to the Config fields that carry API keys.
var goldenKeyFields = map[string]func(*Config, string){
	"CHAIN_ANALYSIS_ETHERSCAN_API_KEY": func(c *Config, v string) { c.EtherscanAPIKey = v },
	"CHAIN_ANALYSIS_NODEREAL_API_KEY":  func(c *Config, v string) { c.NodeRealAPIKey = v },
	"CHAIN_ANALYSIS_AVACLOUD_API_KEY":  func(c *Config, v string) { c.AvaCloudAPIKey = v },
	"CHAIN_ANALYSIS_TRONGRID_API_KEY":  func(c *Config, v string) { c.TronGridAPIKey = v },
}

// goldenEnvOverrides are cleared so every golden run uses the built-in
// endpoint defaults regardless of the developer's shell or .env.
var goldenEnvOverrides = []string{
	"THORNODE_ENDPOINTS", "MIDGARD_ENDPOINTS", "MAYANODE_ENDPOINTS", "MAYA_MIDGARD_ENDPOINTS",
	"CHAIN_ANALYSIS_LEGACY_ACTION_ENDPOINTS", "CHAIN_ANALYSIS_CHAIN_TRACKERS",
	"CHAIN_ANALYSIS_CHAIN_TRACKER_CANDIDATES", "CHAIN_ANALYSIS_UTXO_TRACKERS",
	"CHAIN_ANALYSIS_BLOCKSCOUT_API_URLS", "CHAIN_ANALYSIS_BLOCKSCOUT_API_KEYS",
	"CHAIN_ANALYSIS_COSMOS_TRACKERS", "CHAIN_ANALYSIS_ETHERSCAN_API_URL",
	"CHAIN_ANALYSIS_ETHPLORER_API_URL", "CHAIN_ANALYSIS_ETHPLORER_API_KEY",
	"CHAIN_ANALYSIS_AVACLOUD_BASE_URL", "CHAIN_ANALYSIS_NODEREAL_BSC_URL",
	"CHAIN_ANALYSIS_SOLANA_RPC_URL", "CHAIN_ANALYSIS_TRONGRID_URL",
	"CHAIN_ANALYSIS_XRP_RPC_URL", "CHAIN_ANALYSIS_RADIX_GATEWAY_URL",
	"CHAIN_ANALYSIS_TIMEOUT_SECONDS", "CHAIN_ANALYSIS_MIDGARD_TIMEOUT_SECONDS",
	"CHAIN_ANALYSIS_ETHERSCAN_API_KEY", "CHAIN_ANALYSIS_NODEREAL_API_KEY",
	"CHAIN_ANALYSIS_AVACLOUD_API_KEY", "CHAIN_ANALYSIS_TRONGRID_API_KEY",
}

const goldenReplayKey = "golden-replay-placeholder-key"

func TestGoldenActorGraphs(t *testing.T) {
	record := os.Getenv("CHAIN_ANALYSIS_RECORD") == "1"
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("read %s: %v", goldenDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(goldenDir, name)
			gc := loadGoldenCase(t, dir)
			if record {
				recordGoldenCase(t, dir, gc)
			}

			got, misses := runGoldenCase(t, dir, gc)
			if len(misses) > 0 {
				t.Fatalf("replay missed %d recorded requests; re-record with CHAIN_ANALYSIS_RECORD=1. First: %s", len(misses), misses[0])
			}
			graphPath := filepath.Join(dir, "graph.json")
			if record || *updateGolden {
				if err := os.WriteFile(graphPath, got, 0o644); err != nil {
					t.Fatalf("write %s: %v", graphPath, err)
				}
				return
			}
			want, err := os.ReadFile(graphPath)
			if err != nil {
				t.Fatalf("read %s (run with -update to create it): %v", graphPath, err)
			}
			if string(want) != string(got) {
				t.Fatalf("golden graph mismatch for %s:\n%s\nRun with -update after reviewing the change.", name, firstGoldenDiff(string(want), string(got)))
			}
		})
	}
}

func loadGoldenCase(t *testing.T, dir string) goldenCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "case.json"))
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var gc goldenCase
	if err := json.Unmarshal(raw, &gc); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	return gc
}

func recordGoldenCase(t *testing.T, dir string, gc goldenCase) {
	t.Helper()
	keys := readGoldenDotEnvKeys(t, filepath.Join("..", "..", ".env"))
	secrets := make([]string, 0, len(keys))
	present := make([]string, 0, len(keys))
	for name, value := range keys {
		secrets = append(secrets, value)
		present = append(present, name)
	}
	sort.Strings(present)

	cassette, err := newHTTPCassette(filepath.Join(dir, "cassette.json.gz"), cassetteRecord, nil, secrets)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	started := time.Now()
	if _, err := buildGoldenGraph(t, gc, cassette, keys); err != nil {
		t.Fatalf("record build: %v", err)
	}
	if err := cassette.Save(); err != nil {
		t.Fatalf("save cassette: %v", err)
	}
	_, recorded, _ := cassette.Stats()
	meta := goldenMeta{
		RecordedAt:  time.Now().UTC().Format(time.RFC3339),
		KeysPresent: present,
		Requests:    recorded,
	}
	raw, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	t.Logf("recorded %d requests in %s", recorded, time.Since(started).Round(time.Second))
}

func runGoldenCase(t *testing.T, dir string, gc goldenCase) ([]byte, []string) {
	t.Helper()
	var meta goldenMeta
	if raw, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	keys := map[string]string{}
	for _, name := range meta.KeysPresent {
		keys[name] = goldenReplayKey
	}
	cassette, err := newHTTPCassette(filepath.Join(dir, "cassette.json.gz"), cassetteReplay, nil, []string{goldenReplayKey})
	if err != nil {
		t.Fatalf("open cassette (record with CHAIN_ANALYSIS_RECORD=1): %v", err)
	}
	resp, err := buildGoldenGraph(t, gc, cassette, keys)
	if err != nil {
		t.Fatalf("replay build: %v", err)
	}
	_, _, misses := cassette.Stats()
	return canonicalGoldenGraph(t, resp), misses
}

func buildGoldenGraph(t *testing.T, gc goldenCase, transport *httpCassette, keys map[string]string) (ActorTrackerResponse, error) {
	t.Helper()
	for _, name := range goldenEnvOverrides {
		t.Setenv(name, "")
	}
	cfg := LoadConfigFromEnv()
	cfg.DBPath = filepath.Join(t.TempDir(), "golden.db")
	cfg.LastRunLogPath = filepath.Join(t.TempDir(), "last-run.log")
	cfg.HTTPTransport = transport
	cfg.RequestTimeout = 120 * time.Second
	cfg.MidgardTimeout = 60 * time.Second
	// Live holdings run inside the build under wall-clock budgets; lift them so
	// record and replay issue the same lookups.
	cfg.LiveHoldingsTimeout = 10 * time.Minute
	for name, value := range keys {
		if set, ok := goldenKeyFields[name]; ok {
			set(&cfg, value)
		}
	}

	app, err := New(cfg)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	defer app.Close()
	if transport.mode == cassetteReplay {
		// Provider spacing protects live upstreams; replay has none.
		app.trackerThrottle = nil
	}

	ctx := context.Background()
	req := gc.Request
	req.ActorIDs = nil
	for _, actor := range gc.Actors {
		created, err := app.UpsertActor(ctx, 0, actor)
		if err != nil {
			return ActorTrackerResponse{}, fmt.Errorf("create actor %q: %w", actor.Name, err)
		}
		req.ActorIDs = append(req.ActorIDs, created.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	return app.buildActorTracker(ctx, req)
}

// canonicalGoldenGraph removes wall-clock and live-holdings fields and sorts
// collections so equal graphs serialize identically.
func canonicalGoldenGraph(t *testing.T, resp ActorTrackerResponse) []byte {
	t.Helper()
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if query, ok := doc["query"].(map[string]any); ok {
		delete(query, "requested_at")
	}
	for _, actor := range asGoldenList(doc["actors"]) {
		delete(actor, "created_at")
		delete(actor, "updated_at")
		for _, address := range asGoldenList(actor["addresses"]) {
			delete(address, "created_at")
		}
	}
	for _, node := range asGoldenList(doc["nodes"]) {
		if metrics, ok := node["metrics"].(map[string]any); ok {
			for key := range metrics {
				if strings.HasPrefix(key, "live_holdings") {
					delete(metrics, key)
				}
			}
		}
	}
	// Actor→address ownership edges are stamped with the build's wall-clock
	// time rather than a chain time.
	for _, edge := range asGoldenList(doc["edges"]) {
		if edge["action_class"] != "ownership" {
			continue
		}
		for _, tx := range asGoldenList(edge["transactions"]) {
			delete(tx, "time")
		}
	}
	if warnings, ok := doc["warnings"].([]any); ok {
		sort.Slice(warnings, func(i, j int) bool { return fmt.Sprint(warnings[i]) < fmt.Sprint(warnings[j]) })
	}
	sortGoldenList(doc, "nodes", "id")
	sortGoldenList(doc, "edges", "id")
	sortGoldenList(doc, "supporting_actions", "tx_id", "action_key", "from_node", "to_node", "amount_raw")

	out, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		t.Fatalf("marshal canonical graph: %v", err)
	}
	return append(out, '\n')
}

func asGoldenList(value any) []map[string]any {
	items, _ := value.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func sortGoldenList(doc map[string]any, field string, keys ...string) {
	items, ok := doc[field].([]any)
	if !ok {
		return
	}
	sortKey := func(item any) string {
		m, _ := item.(map[string]any)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprint(m[key]))
		}
		return strings.Join(parts, "\x00")
	}
	sort.SliceStable(items, func(i, j int) bool { return sortKey(items[i]) < sortKey(items[j]) })
}

func firstGoldenDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n- %s\n+ %s", i+1, w, g)
		}
	}
	return "no line difference"
}

func readGoldenDotEnvKeys(t *testing.T, path string) map[string]string {
	t.Helper()
	keys := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		t.Logf("no .env at %s; recording without API keys", path)
		return keys
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, known := goldenKeyFields[name]; known && value != "" {
			keys[name] = value
		}
	}
	return keys
}
