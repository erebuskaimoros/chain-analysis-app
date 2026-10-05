package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newCaseTestApp(t *testing.T) *App {
	t.Helper()
	app, err := New(Config{DBPath: filepath.Join(t.TempDir(), "cases.db"), RequestTimeout: time.Second, MidgardTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// bitgetTraceFixture is one traced swap: 1 ETH from the exploiter becoming
// 0.0316689 BTC at the destination, stopped at the hop limit.
func bitgetTraceFixture() (TraceRequest, TraceResponse) {
	at := time.Date(2026, 9, 28, 3, 55, 51, 0, time.UTC)
	seed, dest := "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3", "BTC|bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"
	req := TraceRequest{Seeds: []TraceSeed{{Chain: "ETH", Address: "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"}}, StartTime: "2026-09-28T03:00:00Z", MaxDepth: 1}
	resp := TraceResponse{
		Query: TraceQuery{Seeds: req.Seeds, StartTime: at.Add(-time.Hour), EndTime: at.Add(3 * time.Hour), Direction: TraceForward, Policy: TracePolicyFIFO, MaxDepth: 1, MaxBranches: 8},
		Nodes: []FlowNode{
			{ID: seed, Kind: "external_address", Label: "Exploiter | ETH", Chain: "ETH", Metrics: map[string]any{"address": "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"}},
			{ID: dest, Kind: "external_address", Label: "bc1qvq…cqj68f", Chain: "BTC", Metrics: map[string]any{"address": "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"}},
		},
		Edges: []TraceEdge{{
			FlowEdge:     FlowEdge{ID: seed + "->" + dest + "|midgard.swap", From: seed, To: dest, ActionClass: "swaps", ActionLabel: "Swap", Confidence: 1},
			TracedAssets: []TraceAmount{{Asset: "BTC.BTC", Amount: 0.0316689, USDAtTime: 2641}},
			TracedTransactions: []TraceTransaction{{
				TxID: "9CD94A8E5734DD6E4C77D0EB400F1C64935BE17AFC062185CB26264004E55CA9", InboundTxID: "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F",
				Time: at, Asset: "BTC.BTC", Amount: 0.0316689, TracedAmount: 0.0316689, InputAsset: "ETH.ETH", InputAmount: 1, TracedInputAmount: 1,
				TracedUSDAtTime: 2655.07, Priced: true, Fraction: 1, Confidence: 1, ConfidenceReason: "THORChain swap links the deposit to the payout",
			}},
		}},
		Sinks: []TraceEndpoint{},
		Frontier: []TraceEndpoint{{
			NodeID: dest, Chain: "BTC", Address: "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f", Label: "bc1qvq…cqj68f", Kind: "address",
			Reason: "max_depth", Depth: 1, Assets: []TraceAmount{{Asset: "BTC.BTC", Amount: 0.0316689, USDAtTime: 2641}}, TracedUSDAtTime: 2641, Confidence: 1,
		}},
		CoverageGaps: []string{"ETH tracker flow truncated for 0xf7bc92…b396c3"},
		Method:       []string{"FIFO: each payment spends the oldest funds received first."},
		Totals:       TraceTotals{SeedAssets: []TraceAmount{{Asset: "ETH.ETH", Amount: 1, USDAtTime: 2655.07}}, SeedUSD: 2655.07, FrontierUSD: 2641},
	}
	return req, resp
}

func TestCasesCollectItemsAndValidateRefs(t *testing.T) {
	app := newCaseTestApp(t)
	ctx := context.Background()
	if _, err := app.CreateCase(ctx, CaseUpsertRequest{Title: "  "}); err == nil {
		t.Fatal("expected a title to be required")
	}
	c, err := app.CreateCase(ctx, CaseUpsertRequest{Title: "Bitget hack", NotesMD: "Reported 2026-09-28."})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req, resp := bitgetTraceFixture()
	runID, err := saveTraceRun(ctx, app.db, req, resp)
	if err != nil {
		t.Fatalf("save trace: %v", err)
	}
	for _, item := range []CaseItemRequest{
		{Kind: "address", Ref: "eth|0xF7BC92103F23EF312658CD9B81DC2713F7B396C3", Note: "exploiter"},
		{Kind: "tx", Ref: "0xa732e09aab76a571d768c2b5b4e2f0e1e5b1a9c3d4e5f60718293a4b5c6d7e8f"},
		{Kind: "trace_run", Ref: strconv.FormatInt(runID, 10)},
	} {
		if _, err := app.AddCaseItem(ctx, c.ID, item); err != nil {
			t.Fatalf("add %s: %v", item.Kind, err)
		}
	}
	if _, err := app.AddCaseItem(ctx, c.ID, CaseItemRequest{Kind: "trace_run", Ref: "999"}); err == nil {
		t.Fatal("expected a missing trace run to be rejected")
	}
	if _, err := app.AddCaseItem(ctx, c.ID, CaseItemRequest{Kind: "wallet", Ref: "x"}); err == nil {
		t.Fatal("expected an unknown kind to be rejected")
	}
	// Pinning again updates the note instead of duplicating the item.
	if _, err := app.AddCaseItem(ctx, c.ID, CaseItemRequest{Kind: "address", Ref: "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3", Note: "the exploiter"}); err != nil {
		t.Fatalf("re-pin: %v", err)
	}
	got, err := app.GetCase(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %+v", got.Items)
	}
	if got.Items[0].Ref != "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3" || got.Items[0].Note != "the exploiter" {
		t.Fatalf("expected the normalised address with its updated note, got %+v", got.Items[0])
	}
	if got.Items[1].Ref != "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F" {
		t.Fatalf("expected the tx hash in THORChain's form, got %q", got.Items[1].Ref)
	}
	if got.Items[2].Title == "" {
		t.Fatalf("expected the trace run item to carry its title, got %+v", got.Items[2])
	}
	if err := app.DeleteCaseItem(ctx, c.ID, got.Items[1].ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	if err := app.DeleteCase(ctx, c.ID); err != nil {
		t.Fatalf("delete case: %v", err)
	}
	if _, err := app.GetCase(ctx, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected the case to be gone, got %v", err)
	}
	var orphans int
	_ = app.db.QueryRow(`SELECT COUNT(*) FROM case_items WHERE case_id = ?`, c.ID).Scan(&orphans)
	if orphans != 0 {
		t.Fatalf("expected the case's items to be deleted, got %d", orphans)
	}
}

func TestCaseExportsMarkdownReportAndFlowsCSV(t *testing.T) {
	app := newCaseTestApp(t)
	ctx := context.Background()
	c, _ := app.CreateCase(ctx, CaseUpsertRequest{Title: "Bitget hack", NotesMD: "Reported 2026-09-28."})
	req, resp := bitgetTraceFixture()
	runID, _ := saveTraceRun(ctx, app.db, req, resp)
	if _, err := app.AddCaseItem(ctx, c.ID, CaseItemRequest{Kind: "trace_run", Ref: strconv.FormatInt(runID, 10)}); err != nil {
		t.Fatalf("pin trace: %v", err)
	}
	if _, err := app.AddCaseItem(ctx, c.ID, CaseItemRequest{Kind: "address", Ref: "BTC|bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"}); err != nil {
		t.Fatalf("pin address: %v", err)
	}

	report, err := app.ExportCaseMarkdown(ctx, c.ID)
	if err != nil {
		t.Fatalf("markdown: %v", err)
	}
	for _, want := range []string{
		"# Bitget hack",
		"Reported 2026-09-28.",
		"## Trace ",
		"### Method",
		"| [bc1qvq…cqj68f](https://mempool.space/address/bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f) | BTC | hop limit | 0.03166890 BTC.BTC | $2,641 | 100% | — |",
		"in [A732E09A…6D7E8F](https://etherscan.io/tx/0xa732e09aab76a571d768c2b5b4e2f0e1e5b1a9c3d4e5f60718293a4b5c6d7e8f)",
		"out [9CD94A8E…E55CA9](https://mempool.space/tx/9cd94a8e5734dd6e4c77d0eb400f1c64935be17afc062185cb26264004e55ca9)",
		"1.0000 ETH.ETH → 0.03166890 BTC.BTC",
		`[Exploiter \| ETH](https://etherscan.io/address/0xf7bc92103f23ef312658cd9b81dc2713f7b396c3)`,
		"- Gap: ETH tracker flow truncated",
		"## Label sources",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("expected the report to contain %q:\n%s", want, report)
		}
	}

	data, err := app.ExportCaseCSV(ctx, c.ID)
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected a header and one flow, got %v (%v)", rows, err)
	}
	row := map[string]string{}
	for i, name := range rows[0] {
		row[name] = rows[1][i]
	}
	if row["from_address"] != "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3" || row["to_chain"] != "BTC" || row["traced_amount"] != "0.0316689" ||
		row["inbound_tx_id"] == "" || row["traced_usd_at_time"] != "2655.07" {
		t.Fatalf("unexpected CSV row %v", row)
	}
}

func TestFormatUSDText(t *testing.T) {
	for value, want := range map[float64]string{0: "—", 999.4: "$999", 1000: "$1,000", 7731948.2: "$7,731,948", -2500: "-$2,500"} {
		if got := formatUSDText(value); got != want {
			t.Fatalf("formatUSDText(%v) = %q, want %q", value, got, want)
		}
	}
}
