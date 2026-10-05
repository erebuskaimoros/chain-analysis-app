package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"chain-analysis-app/internal/api"
	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/client"
	"chain-analysis-app/internal/domain/services"
)

// newAPIServer serves the real /api/v1 over a temp database.
func newAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	legacy, err := app.New(app.Config{DBPath: filepath.Join(t.TempDir(), "mcp.db"), RequestTimeout: time.Second, MidgardTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	mux := http.NewServeMux()
	api.NewV1(services.New(legacy)).Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = legacy.Close()
	})
	return server
}

func connect(t *testing.T, baseURL string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := newServer(client.New(baseURL)).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func textOf(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestMCPToolsListAndCaseExport(t *testing.T) {
	apiServer := newAPIServer(t)
	ctx := context.Background()
	c := client.New(apiServer.URL)
	created, err := c.CreateCase(ctx, app.CaseUpsertRequest{Title: "Bitget hack", NotesMD: "Reported 2026-09-28."})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	if _, err := c.AddCaseItem(ctx, created.ID, app.CaseItemRequest{Kind: "address", Ref: "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3", Note: "exploiter"}); err != nil {
		t.Fatalf("add item: %v", err)
	}
	session := connect(t, apiServer.URL)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = tool
	}
	for _, want := range []string{"trace_funds", "address_profile", "actor_summary", "tx_lookup", "case_list", "case_export"} {
		if names[want] == nil {
			t.Fatalf("missing tool %s; have %v", want, names)
		}
	}
	schema, _ := json.Marshal(names["trace_funds"].InputSchema)
	if !strings.Contains(string(schema), `"seeds"`) || !strings.Contains(string(schema), "largest_out") {
		t.Fatalf("expected the trace input schema to describe seeds and policies, got %s", schema)
	}

	list, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "case_list", Arguments: map[string]any{}})
	if err != nil || list.IsError || !strings.Contains(textOf(list), "Bitget hack") {
		t.Fatalf("case_list: %v %+v", err, list)
	}
	export, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "case_export", Arguments: map[string]any{"case_id": created.ID}})
	if err != nil || export.IsError {
		t.Fatalf("case_export: %v %+v", err, export)
	}
	report := textOf(export)
	if !strings.HasPrefix(report, "# Bitget hack") || !strings.Contains(report, "https://etherscan.io/address/0xf7bc92103f23ef312658cd9b81dc2713f7b396c3") {
		t.Fatalf("expected the Markdown report as text, got %q", report)
	}
}

func TestMCPToolErrorsAreToolResults(t *testing.T) {
	session := connect(t, newAPIServer(t).URL)
	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "trace_funds", Arguments: map[string]any{"seeds": []string{" "}}})
	if err != nil || !result.IsError || !strings.Contains(textOf(result), "at least one address") {
		t.Fatalf("expected a tool error for empty seeds, got %v %+v", err, result)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "case_export", Arguments: map[string]any{"case_id": 999}})
	if err != nil || !result.IsError || !strings.Contains(textOf(result), "404") {
		t.Fatalf("expected a not-found tool error, got %v %+v", err, result)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "actor_summary", Arguments: map[string]any{"actor": "Nobody"}})
	if err != nil || !result.IsError || !strings.Contains(textOf(result), "no actor") {
		t.Fatalf("expected an unknown-actor tool error, got %v %+v", err, result)
	}
}

func TestDigestTraceKeepsLargestFlowsAndLabels(t *testing.T) {
	at := time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)
	resp := app.TraceResponse{
		RunID: 3,
		Query: app.TraceQuery{Direction: "forward", StartTime: at, EndTime: at.Add(time.Hour)},
		Nodes: []app.FlowNode{
			{ID: "s", Label: "Exploiter", Chain: "ETH", Metrics: map[string]any{"address": "0xseed"}},
			{ID: "d", Label: "bc1q…dest", Chain: "BTC", Metrics: map[string]any{"address": "bc1qdest"}},
		},
	}
	edge := app.TraceEdge{FlowEdge: app.FlowEdge{From: "s", To: "d", ActionClass: "swaps", ActionLabel: "Swap"}}
	for i := 0; i < digestFlows+5; i++ {
		edge.TracedTransactions = append(edge.TracedTransactions, app.TraceTransaction{
			TxID: "OUT", InboundTxID: "IN", Time: at, Asset: "BTC.BTC", TracedAmount: 0.03, InputAsset: "ETH.ETH", TracedInputAmount: 1, TracedUSDAtTime: float64(i),
		})
	}
	resp.Edges = []app.TraceEdge{edge}
	d := digestTrace(resp)
	if d.TotalFlows != digestFlows+5 || len(d.Flows) != digestFlows || !d.TruncatedLists {
		t.Fatalf("expected the flows capped at %d of %d, got %d (truncated %v)", digestFlows, d.TotalFlows, len(d.Flows), d.TruncatedLists)
	}
	first := d.Flows[0]
	if first.USD != float64(digestFlows+4) || first.From != "Exploiter (ETH 0xseed)" || first.Traced != "1 ETH.ETH → 0.03 BTC.BTC" || first.InboundTxID != "IN" {
		t.Fatalf("unexpected largest flow %+v", first)
	}
}

func TestFormatCoin(t *testing.T) {
	if got := formatCoin("ETH.ETH", "150000000"); got != "1.5 ETH.ETH" {
		t.Fatalf("formatCoin = %q", got)
	}
}
