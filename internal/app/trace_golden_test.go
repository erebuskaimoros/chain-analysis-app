package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Trace golden tests replay recorded upstream traffic through the trace
// engine. A case runs its traces in order on one app, so later traces reuse
// the ledger that earlier ones filled.
//
//	go test ./internal/app -run TestTraceGolden            replay and compare
//	go test ./internal/app -run TestTraceGolden -update    rewrite trace.json
//	CHAIN_ANALYSIS_RECORD=1 go test ./internal/app -run TestTraceGolden -timeout 30m

const traceGoldenDir = "testdata/trace_golden"

type traceGoldenCase struct {
	Description string `json:"description"`
	Traces      []struct {
		Name    string       `json:"name"`
		Request TraceRequest `json:"request"`
	} `json:"traces"`
}

// traceGoldenChecks hold each case's acceptance criteria, checked on replay.
var traceGoldenChecks = map[string]func(t *testing.T, results map[string]TraceResponse, elapsed time.Duration){
	"bitget_exploiter": checkBitgetTrace,
}

func TestTraceGolden(t *testing.T) {
	record := os.Getenv("CHAIN_ANALYSIS_RECORD") == "1"
	entries, err := os.ReadDir(traceGoldenDir)
	if err != nil {
		t.Fatalf("read %s: %v", traceGoldenDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(traceGoldenDir, name)
			raw, err := os.ReadFile(filepath.Join(dir, "case.json"))
			if err != nil {
				t.Fatalf("read case: %v", err)
			}
			var tc traceGoldenCase
			if err := json.Unmarshal(raw, &tc); err != nil {
				t.Fatalf("decode case: %v", err)
			}
			if record {
				recordTraceGoldenCase(t, dir, tc)
			}
			results, elapsed, misses := runTraceGoldenCase(t, dir, tc, cassetteReplay, nil)
			if len(misses) > 0 {
				t.Fatalf("replay missed %d recorded requests; re-record with CHAIN_ANALYSIS_RECORD=1. First: %s", len(misses), misses[0])
			}
			if check := traceGoldenChecks[name]; check != nil {
				check(t, results, elapsed)
			}
			got := canonicalTraceGolden(t, tc, results)
			snapshot := filepath.Join(dir, "trace.json")
			if record || *updateGolden {
				if err := os.WriteFile(snapshot, got, 0o644); err != nil {
					t.Fatalf("write %s: %v", snapshot, err)
				}
				return
			}
			want, err := os.ReadFile(snapshot)
			if err != nil {
				t.Fatalf("read %s (run with -update to create it): %v", snapshot, err)
			}
			if string(want) != string(got) {
				t.Fatalf("trace golden mismatch for %s:\n%s\nRun with -update after reviewing the change.", name, firstGoldenDiff(string(want), string(got)))
			}
		})
	}
}

func recordTraceGoldenCase(t *testing.T, dir string, tc traceGoldenCase) {
	t.Helper()
	keys := readGoldenDotEnvKeys(t, filepath.Join("..", "..", ".env"))
	started := time.Now()
	_, _, recorded := runTraceGoldenCase(t, dir, tc, cassetteRecord, keys)
	present := make([]string, 0, len(keys))
	for name := range keys {
		present = append(present, name)
	}
	sort.Strings(present)
	meta := goldenMeta{RecordedAt: time.Now().UTC().Format(time.RFC3339), KeysPresent: present, Requests: len(recorded)}
	raw, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	t.Logf("recorded %d requests in %s", len(recorded), time.Since(started).Round(time.Second))
}

// runTraceGoldenCase runs every trace of a case. In replay mode it returns
// the cassette's misses; in record mode, the recorded request count as a
// slice length.
func runTraceGoldenCase(t *testing.T, dir string, tc traceGoldenCase, mode cassetteMode, recordKeys map[string]string) (map[string]TraceResponse, time.Duration, []string) {
	t.Helper()
	keys := recordKeys
	secrets := []string{goldenReplayKey}
	if mode == cassetteReplay {
		var meta goldenMeta
		if raw, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
			_ = json.Unmarshal(raw, &meta)
		}
		keys = map[string]string{}
		for _, name := range meta.KeysPresent {
			keys[name] = goldenReplayKey
		}
	} else {
		secrets = secrets[:0]
		for _, value := range recordKeys {
			secrets = append(secrets, value)
		}
	}
	cassette, err := newHTTPCassette(filepath.Join(dir, "cassette.json.gz"), mode, nil, secrets)
	if err != nil {
		t.Fatalf("open cassette (record with CHAIN_ANALYSIS_RECORD=1): %v", err)
	}
	app, err := newGoldenApp(t, cassette, keys)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	results := map[string]TraceResponse{}
	started := time.Now()
	for _, trace := range tc.Traces {
		resp, err := app.Trace(ctx, trace.Request)
		if err != nil {
			t.Fatalf("trace %s: %v", trace.Name, err)
		}
		results[trace.Name] = resp
	}
	elapsed := time.Since(started)
	if mode == cassetteRecord {
		if err := cassette.Save(); err != nil {
			t.Fatalf("save cassette: %v", err)
		}
		_, recorded, _ := cassette.Stats()
		return results, elapsed, make([]string, recorded)
	}
	_, _, misses := cassette.Stats()
	return results, elapsed, misses
}

func canonicalTraceGolden(t *testing.T, tc traceGoldenCase, results map[string]TraceResponse) []byte {
	t.Helper()
	doc := map[string]any{}
	for _, trace := range tc.Traces {
		raw, err := json.Marshal(results[trace.Name])
		if err != nil {
			t.Fatalf("marshal %s: %v", trace.Name, err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("unmarshal %s: %v", trace.Name, err)
		}
		if stats, ok := value["stats"].(map[string]any); ok {
			delete(stats, "elapsed_ms")
		}
		for _, field := range []string{"warnings", "coverage_gaps"} {
			if list, ok := value[field].([]any); ok {
				sort.Slice(list, func(i, j int) bool { return fmt.Sprint(list[i]) < fmt.Sprint(list[j]) })
			}
		}
		doc[trace.Name] = value
	}
	out, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		t.Fatalf("marshal canonical trace: %v", err)
	}
	return append(out, '\n')
}

const (
	bitgetSeedKey        = "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"
	bitgetDestinationKey = "BTC|bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"
	bitgetTracedBTC      = 87.82301390
)

func traceAssetAmount(assets []TraceAmount, asset string) float64 {
	total := 0.0
	for _, a := range assets {
		if a.Asset == asset {
			total += a.Amount
		}
	}
	return total
}

// checkBitgetTrace is acceptance case B: every swap is found, the BTC
// arrives at one address, and every edge carries its transaction IDs.
func checkBitgetTrace(t *testing.T, results map[string]TraceResponse, elapsed time.Duration) {
	t.Helper()
	if elapsed > 2*time.Second {
		t.Errorf("replayed traces took %s, want under 2s", elapsed.Round(time.Millisecond))
	}
	depth1 := results["fifo_depth1"]
	// The seed also sends 119.34 ETH on to another wallet (ending 164c, no
	// THORChain history); the swaps must all sit on one edge to the BTC
	// destination.
	var swaps []TraceEdge
	for _, edge := range depth1.Edges {
		if edge.From == bitgetSeedKey && edge.ActionClass == "swaps" {
			swaps = append(swaps, edge)
		}
	}
	if len(swaps) != 1 || swaps[0].To != bitgetDestinationKey || len(swaps[0].TracedTransactions) != 31 {
		var got []string
		for _, edge := range swaps {
			got = append(got, fmt.Sprintf("%s (%d txs)", edge.To, len(edge.TracedTransactions)))
		}
		t.Fatalf("expected 31 swaps on one edge to the BTC destination, got %s", strings.Join(got, ", "))
	}
	for _, tx := range swaps[0].TracedTransactions {
		if tx.TxID == "" || tx.InboundTxID == "" {
			t.Fatalf("expected both the ETH deposit and BTC payout hashes on every swap, got %+v", tx)
		}
	}
	var destination *TraceEndpoint
	for i := range depth1.Frontier {
		if depth1.Frontier[i].NodeID == bitgetDestinationKey {
			destination = &depth1.Frontier[i]
		}
	}
	if destination == nil {
		t.Fatalf("expected the BTC destination at the hop limit, got %+v", depth1.Frontier)
	}
	btc := traceAssetAmount(destination.Assets, "BTC.BTC")
	if math.Abs(btc-bitgetTracedBTC)/bitgetTracedBTC > 0.01 {
		t.Fatalf("expected about %.2f BTC traced to the destination, got %.8f", bitgetTracedBTC, btc)
	}

	depth3 := results["fifo_depth3"]
	for _, edge := range depth3.Edges {
		if len(edge.TxIDs) == 0 {
			t.Fatalf("edge %s has no transaction IDs", edge.ID)
		}
	}
	// What reaches the destination is either passed on or still held there.
	in, out, held := 0.0, 0.0, 0.0
	for _, edge := range depth3.Edges {
		if edge.To == bitgetDestinationKey {
			in += traceAssetAmount(edge.TracedAssets, "BTC.BTC")
		}
		if edge.From == bitgetDestinationKey {
			out += traceAssetAmount(edge.TracedInputAssets, "BTC.BTC")
			if len(edge.TracedInputAssets) == 0 {
				out += traceAssetAmount(edge.TracedAssets, "BTC.BTC")
			}
		}
	}
	for _, sink := range depth3.Sinks {
		if sink.NodeID == bitgetDestinationKey {
			held += traceAssetAmount(sink.Assets, "BTC.BTC")
		}
	}
	if math.Abs(in-bitgetTracedBTC)/bitgetTracedBTC > 0.01 || math.Abs(in-out-held) > 1e-6 {
		t.Fatalf("expected the destination's %.8f traced BTC to be passed on or held, got in %.8f, out %.8f, held %.8f", bitgetTracedBTC, in, out, held)
	}
}
