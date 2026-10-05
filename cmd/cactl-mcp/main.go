// Command cactl-mcp serves the chain-analysis tools over the Model Context
// Protocol on stdio, so an MCP client (such as Claude Code) can trace funds,
// profile addresses, summarise actors, look up transactions and export cases
// against a running chain-analysis server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"chain-analysis-app/internal/client"
)

const instructions = `Tools for investigating THORChain and connected-chain fund flows through a local chain-analysis server.
- trace_funds follows value forward (where did it go) or backward (where did it come from) from addresses or a THORChain transaction hash, through swaps across chains, and reports where it rests (sinks), where limits stopped it (frontier) and the flows with their transaction hashes. Values are USD at transaction time.
- address_profile says what an address is: labels, current holdings, and its counterparties over recent days.
- actor_summary reports a saved actor's (named address set's) holdings over time and what changed since it was last viewed.
- tx_lookup shows the THORChain action(s) for a transaction hash.
- case_list and case_export list investigation cases and export one as a Markdown report or a flows CSV.
Traces and profiles query public providers and can take up to a few minutes.`

func main() {
	baseURL := flag.String("url", "", "chain-analysis server URL (default $CHAIN_ANALYSIS_URL or http://localhost:8090)")
	flag.Parse()
	log.SetOutput(os.Stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server := newServer(client.New(*baseURL))
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "cactl-mcp:", err)
		os.Exit(1)
	}
}

func newServer(c *client.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "chain-analysis", Version: "0.1.0"}, &mcp.ServerOptions{Instructions: instructions})
	tools := &toolset{client: c}
	openWorld := true
	mcp.AddTool(server, &mcp.Tool{
		Name:        "trace_funds",
		Description: "Follow funds from seed addresses or a THORChain transaction hash, hop by hop through swaps across chains. Returns sinks (exchanges, sanctioned addresses, pools, bonds, or wallets still holding the value), the frontier left by hop/branch limits, the largest traced flows with transaction hashes, the method used and coverage gaps. The trace is saved; its run_id can be pinned to a case.",
		Annotations: &mcp.ToolAnnotations{Title: "Trace funds", OpenWorldHint: &openWorld},
	}, tools.traceFunds)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "address_profile",
		Description: "Profile one address: its labels (exchange, sanctioned, scam, …), what it holds now, and its counterparties and most recent transactions over the last N days (default 30), valued at transaction time.",
		Annotations: &mcp.ToolAnnotations{Title: "Address profile", ReadOnlyHint: true, OpenWorldHint: &openWorld},
	}, tools.addressProfile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "actor_summary",
		Description: "Summarise a saved actor (a named set of addresses such as a treasury) by ID or name: holdings over time and the changes since it was last viewed, by asset, address and counterparty. Set refresh to record a new snapshot first.",
		Annotations: &mcp.ToolAnnotations{Title: "Actor summary", OpenWorldHint: &openWorld},
	}, tools.actorSummary)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tx_lookup",
		Description: "Look up the THORChain or MAYAChain action(s) for a transaction hash (inbound or outbound, with or without 0x): type, status, time, input and output legs, pools and memo.",
		Annotations: &mcp.ToolAnnotations{Title: "Transaction lookup", ReadOnlyHint: true, OpenWorldHint: &openWorld},
	}, tools.txLookup)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "case_list",
		Description: "List investigation cases with their IDs and item counts.",
		Annotations: &mcp.ToolAnnotations{Title: "List cases", ReadOnlyHint: true},
	}, tools.caseList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "case_export",
		Description: "Export an investigation case as a Markdown report (summary, items, each pinned trace's method, sinks, flows with explorer links, coverage gaps, label sources) or as a CSV of every traced flow.",
		Annotations: &mcp.ToolAnnotations{Title: "Export case", ReadOnlyHint: true},
	}, tools.caseExport)
	return server
}
