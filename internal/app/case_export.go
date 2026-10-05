package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Case export renders a case as a Markdown report (summary, items, each
// pinned trace with its method, sinks, flows and coverage, and the label
// sources behind the labels) or as a CSV of every traced flow.

// explorerAddressURL links an address on its chain's public explorer.
func explorerAddressURL(chain, address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	escaped := url.PathEscape(address)
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "THOR":
		return "https://thorchain.net/address/" + escaped
	case "MAYA":
		return "https://www.explorer.mayachain.info/address/" + escaped
	case "BTC":
		return "https://mempool.space/address/" + escaped
	case "LTC":
		return "https://litecoinspace.org/address/" + escaped
	case "BCH":
		return "https://blockchair.com/bitcoin-cash/address/" + escaped
	case "DOGE":
		return "https://blockchair.com/dogecoin/address/" + escaped
	case "ETH":
		return "https://etherscan.io/address/" + escaped
	case "BSC":
		return "https://bscscan.com/address/" + escaped
	case "BASE":
		return "https://basescan.org/address/" + escaped
	case "AVAX":
		return "https://snowtrace.io/address/" + escaped
	case "GAIA":
		return "https://www.mintscan.io/cosmos/address/" + escaped
	case "SOL":
		return "https://explorer.solana.com/address/" + escaped
	case "TRON":
		return "https://tronscan.org/#/address/" + escaped
	case "XRP":
		return "https://xrpscan.com/account/" + escaped
	}
	return ""
}

// explorerTxURL links a transaction hash, given in THORChain's form (upper
// case, no 0x), on its chain's explorer.
func explorerTxURL(chain, txID string) string {
	txID = strings.TrimSpace(txID)
	if txID == "" {
		return ""
	}
	lower := strings.TrimPrefix(strings.ToLower(txID), "0x")
	upper := strings.ToUpper(txID)
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "THOR", "MAYA", "":
		return "https://thorchain.net/tx/" + url.PathEscape(upper)
	case "BTC":
		return "https://mempool.space/tx/" + lower
	case "LTC":
		return "https://litecoinspace.org/tx/" + lower
	case "BCH":
		return "https://blockchair.com/bitcoin-cash/transaction/" + lower
	case "DOGE":
		return "https://blockchair.com/dogecoin/transaction/" + lower
	case "ETH":
		return "https://etherscan.io/tx/0x" + lower
	case "BSC":
		return "https://bscscan.com/tx/0x" + lower
	case "BASE":
		return "https://basescan.org/tx/0x" + lower
	case "AVAX":
		return "https://snowtrace.io/tx/0x" + lower
	case "GAIA":
		return "https://www.mintscan.io/cosmos/tx/" + upper
	case "TRON":
		return "https://tronscan.org/#/transaction/" + lower
	case "XRP":
		return "https://xrpscan.com/tx/" + upper
	}
	return ""
}

// traceTxChains returns the chains a traced transaction's hashes live on:
// a swap's deposit on its input chain and its payout on its output chain.
func traceTxChains(edge TraceEdge, tx TraceTransaction) (inbound, payout string) {
	payout = chainFromAsset(tx.Asset)
	if tx.InboundTxID != "" {
		return chainFromAsset(firstNonEmpty(tx.InputAsset, tx.Asset)), payout
	}
	if edge.ActionClass != "transfers" {
		return "", "THOR"
	}
	return "", payout
}

func mdEscape(text string) string {
	replacer := strings.NewReplacer("|", "\\|", "\n", " ", "[", "\\[", "]", "\\]")
	return replacer.Replace(text)
}

func mdLink(text, href string) string {
	if href == "" {
		return mdEscape(text)
	}
	return "[" + mdEscape(text) + "](" + href + ")"
}

func shortTx(txID string) string {
	if len(txID) <= 16 {
		return txID
	}
	return txID[:8] + "…" + txID[len(txID)-6:]
}

func formatTraceUnits(amount float64) string {
	switch {
	case amount >= 1000:
		return strconv.FormatFloat(amount, 'f', 2, 64)
	case amount >= 1:
		return strconv.FormatFloat(amount, 'f', 4, 64)
	default:
		return strconv.FormatFloat(amount, 'f', 8, 64)
	}
}

func formatUSDText(value float64) string {
	if value == 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	digits := strconv.FormatFloat(math.Abs(math.Round(value)), 'f', 0, 64)
	var out []byte
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	if value < 0 {
		return "-$" + string(out)
	}
	return "$" + string(out)
}

func traceAmountsText(amounts []TraceAmount) string {
	parts := make([]string, 0, len(amounts))
	for _, amount := range amounts {
		parts = append(parts, formatTraceUnits(amount.Amount)+" "+amount.Asset)
	}
	return strings.Join(parts, ", ")
}

var traceReasonText = map[string]string{
	"held":               "still held",
	"held_before_window": "held before the window",
	"pool":               "liquidity pool",
	"bond":               "validator bond",
	"contract":           "contract",
	"protocol":           "protocol address",
	"max_depth":          "hop limit",
	"max_branches":       "branch limit",
	"below_min_usd":      "below minimum",
	"expansion_limit":    "expansion limit",
	"unexpanded":         "not expanded",
}

func traceReason(reason string) string {
	if text, ok := traceReasonText[reason]; ok {
		return text
	}
	return strings.ReplaceAll(reason, "_", " ")
}

// caseTraces loads the trace runs pinned to a case, in pin order.
func (a *App) caseTraces(ctx context.Context, c Case) ([]TraceRun, error) {
	var runs []TraceRun
	for _, item := range c.Items {
		if item.Kind != CaseItemTraceRun {
			continue
		}
		id, _ := strconv.ParseInt(item.Ref, 10, 64)
		run, err := getTraceRun(ctx, a.db, id)
		if err != nil {
			return nil, fmt.Errorf("load trace run %d: %w", id, err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ExportCaseMarkdown renders a case as a Markdown report.
func (a *App) ExportCaseMarkdown(ctx context.Context, id int64) (string, error) {
	c, err := a.GetCase(ctx, id)
	if err != nil {
		return "", err
	}
	runs, err := a.caseTraces(ctx, c)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Title)
	fmt.Fprintf(&b, "_Case %d, exported %s UTC from chain-analysis-app. Values are in USD at transaction time unless noted._\n\n", c.ID, time.Now().UTC().Format("2006-01-02 15:04"))
	if notes := strings.TrimSpace(c.NotesMD); notes != "" {
		b.WriteString("## Notes\n\n" + notes + "\n\n")
	}

	b.WriteString("## Summary\n\n")
	if len(runs) == 0 {
		b.WriteString("No traces are pinned to this case.\n\n")
	}
	for _, run := range runs {
		if run.Response == nil {
			continue
		}
		totals := run.Response.Totals
		fmt.Fprintf(&b, "- **%s**: %s traced (%s); %s reached %d sinks; %s stopped at limits.\n",
			mdEscape(run.Title), formatUSDText(totals.SeedUSD), traceAmountsText(totals.SeedAssets),
			formatUSDText(totals.SinkUSD), len(run.Response.Sinks), formatUSDText(totals.FrontierUSD))
	}
	b.WriteString("\n")

	if len(c.Items) > 0 {
		b.WriteString("## Items\n\n| Kind | Item | Note |\n|---|---|---|\n")
		for _, item := range c.Items {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", item.Kind, a.caseItemMarkdown(ctx, item), mdEscape(item.Note))
		}
		b.WriteString("\n")
	}

	for _, run := range runs {
		if run.Response != nil {
			writeTraceMarkdown(&b, run)
		}
	}

	sources, err := listLabelSources(ctx, a.db)
	if err == nil && len(sources) > 0 {
		b.WriteString("## Label sources\n\n| Source | Labels | Version | License | Imported |\n|---|---:|---|---|---|\n")
		for _, source := range sources {
			fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", mdEscape(source.Source), source.Count, mdEscape(source.Version), mdEscape(source.License), mdEscape(source.ImportedAt))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func (a *App) caseItemMarkdown(ctx context.Context, item CaseItem) string {
	switch item.Kind {
	case CaseItemAddress:
		address, chain := caseItemAddress(item.Ref), caseItemChain(item.Ref)
		text := address
		if item.Title != "" {
			text = item.Title + " (" + shortAddress(address) + ")"
		}
		return mdLink(text, explorerAddressURL(chain, address)) + " " + chain
	case CaseItemTx:
		return mdLink(shortTx(item.Ref), explorerTxURL("THOR", item.Ref))
	case CaseItemTraceRun:
		return mdEscape(fmt.Sprintf("Trace %s: %s", item.Ref, item.Title))
	case CaseItemGraphState:
		return mdEscape(fmt.Sprintf("Graph state %s: %s", item.Ref, item.Title))
	case CaseItemActor:
		text := fmt.Sprintf("Actor %s: %s", item.Ref, item.Title)
		if id, err := strconv.ParseInt(item.Ref, 10, 64); err == nil {
			if snapshots, err := listActorSnapshots(ctx, a.db, id); err == nil && len(snapshots) > 0 {
				latest := snapshots[len(snapshots)-1]
				text += fmt.Sprintf(" — holdings %s at %s UTC", formatUSDText(latest.TotalUSD), latest.TakenAt.UTC().Format("2006-01-02 15:04"))
			}
		}
		return mdEscape(text)
	}
	return mdEscape(item.Ref)
}

func traceNodeLabels(resp *TraceResponse) map[string]FlowNode {
	nodes := map[string]FlowNode{}
	for _, node := range resp.Nodes {
		nodes[node.ID] = node
	}
	return nodes
}

func traceNodeLink(nodes map[string]FlowNode, id string) string {
	node, ok := nodes[id]
	if !ok {
		return mdEscape(id)
	}
	label := firstNonEmpty(node.Label, id)
	return mdLink(label, explorerAddressURL(node.Chain, getString(node.Metrics, "address")))
}

func writeTraceMarkdown(b *strings.Builder, run TraceRun) {
	resp := run.Response
	q := resp.Query
	fmt.Fprintf(b, "## Trace %d: %s\n\n", run.ID, mdEscape(run.Title))
	var seeds []string
	for _, seed := range q.Seeds {
		switch {
		case seed.TxID != "" && seed.Address != "":
			seeds = append(seeds, fmt.Sprintf("%s (sender of %s)", mdLink(seed.Address, explorerAddressURL(seed.Chain, seed.Address)), mdLink(shortTx(seed.TxID), explorerTxURL("THOR", seed.TxID))))
		case seed.TxID != "":
			seeds = append(seeds, mdLink(shortTx(seed.TxID), explorerTxURL("THOR", seed.TxID)))
		default:
			seeds = append(seeds, mdLink(seed.Address, explorerAddressURL(firstNonEmpty(seed.Chain, normalizeChain("", seed.Address)), seed.Address))+" "+seed.Chain)
		}
	}
	fmt.Fprintf(b, "- **Seeds:** %s\n", strings.Join(seeds, ", "))
	fmt.Fprintf(b, "- **Window:** %s to %s UTC\n", q.StartTime.UTC().Format("2006-01-02 15:04"), q.EndTime.UTC().Format("2006-01-02 15:04"))
	fmt.Fprintf(b, "- **Direction:** %s · **Policy:** %s · **Limits:** %d hops, %d branches per address", q.Direction, strings.ReplaceAll(q.Policy, "_", " "), q.MaxDepth, q.MaxBranches)
	if q.MinUSDAtTime > 0 {
		fmt.Fprintf(b, ", minimum %s", formatUSDText(q.MinUSDAtTime))
	}
	b.WriteString("\n")
	if q.Amount > 0 {
		fmt.Fprintf(b, "- **Amount:** %s %s\n", formatTraceUnits(q.Amount), q.Asset)
	}
	fmt.Fprintf(b, "- **Run:** %s UTC\n\n", run.CreatedAt.UTC().Format("2006-01-02 15:04"))

	b.WriteString("### Method\n\n")
	for _, line := range resp.Method {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n")

	nodes := traceNodeLabels(resp)
	writeEndpoints := func(title string, endpoints []TraceEndpoint) {
		fmt.Fprintf(b, "### %s\n\n", title)
		if len(endpoints) == 0 {
			b.WriteString("None.\n\n")
			return
		}
		b.WriteString("| Endpoint | Chain | Why it stops | Traced | USD at time | Confidence | Holds now |\n|---|---|---|---|---:|---:|---:|\n")
		for _, endpoint := range endpoints {
			label := endpoint.Label
			if endpoint.Category != "" {
				label += " [" + endpoint.Category + "]"
			}
			holds := "—"
			if endpoint.HoldingsUSD != nil {
				holds = formatUSDText(*endpoint.HoldingsUSD)
			}
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %d%% | %s |\n",
				mdLink(label, explorerAddressURL(endpoint.Chain, endpoint.Address)), endpoint.Chain, traceReason(endpoint.Reason),
				traceAmountsText(endpoint.Assets), formatUSDText(endpoint.TracedUSDAtTime), int(math.Round(endpoint.Confidence*100)), holds)
		}
		b.WriteString("\n")
	}
	sinksTitle := "Sinks"
	if q.Direction == TraceBackward {
		sinksTitle = "Sources"
	}
	writeEndpoints(sinksTitle, resp.Sinks)
	writeEndpoints("Stopped by limits", resp.Frontier)

	b.WriteString("### Flows\n\n")
	flows := traceFlows(resp)
	if len(flows) == 0 {
		b.WriteString("None.\n\n")
	} else {
		b.WriteString("| Time (UTC) | From → To | Type | Traced | USD at time | Transactions | Confidence |\n|---|---|---|---|---:|---|---:|\n")
		for _, flow := range flows {
			edge, tx := flow.edge, flow.tx
			amount := formatTraceUnits(tx.TracedAmount) + " " + tx.Asset
			if tx.InputAsset != "" {
				amount = formatTraceUnits(tx.TracedInputAmount) + " " + tx.InputAsset + " → " + amount
			}
			inChain, outChain := traceTxChains(edge, tx)
			var links []string
			if tx.InboundTxID != "" {
				links = append(links, "in "+mdLink(shortTx(tx.InboundTxID), explorerTxURL(inChain, tx.InboundTxID)))
				links = append(links, "out "+mdLink(shortTx(tx.TxID), explorerTxURL(outChain, tx.TxID)))
			} else {
				links = append(links, mdLink(shortTx(tx.TxID), explorerTxURL(outChain, tx.TxID)))
			}
			confidence := fmt.Sprintf("%d%%", int(math.Round(tx.Confidence*100)))
			if tx.ConfidenceReason != "" {
				confidence += " (" + mdEscape(tx.ConfidenceReason) + ")"
			}
			fmt.Fprintf(b, "| %s | %s → %s | %s | %s | %s | %s | %s |\n",
				tx.Time.UTC().Format("2006-01-02 15:04:05"), traceNodeLink(nodes, edge.From), traceNodeLink(nodes, edge.To),
				mdEscape(firstNonEmpty(edge.ActionLabel, edge.ActionClass)), amount, formatUSDText(tx.TracedUSDAtTime), strings.Join(links, " "), confidence)
		}
		b.WriteString("\n")
	}

	if len(resp.CoverageGaps) > 0 || len(resp.Warnings) > 0 {
		b.WriteString("### Coverage gaps and warnings\n\n")
		for _, gap := range resp.CoverageGaps {
			b.WriteString("- Gap: " + mdEscape(gap) + "\n")
		}
		for _, warning := range resp.Warnings {
			b.WriteString("- Warning: " + mdEscape(warning) + "\n")
		}
		b.WriteString("\n")
	}
}

type traceFlow struct {
	edge TraceEdge
	tx   TraceTransaction
}

func traceFlows(resp *TraceResponse) []traceFlow {
	var flows []traceFlow
	for _, edge := range resp.Edges {
		for _, tx := range edge.TracedTransactions {
			flows = append(flows, traceFlow{edge: edge, tx: tx})
		}
	}
	sort.SliceStable(flows, func(i, j int) bool {
		if !flows[i].tx.Time.Equal(flows[j].tx.Time) {
			return flows[i].tx.Time.Before(flows[j].tx.Time)
		}
		return flows[i].tx.TxID < flows[j].tx.TxID
	})
	return flows
}

// ExportCaseCSV writes every traced flow in the case's traces as CSV.
func (a *App) ExportCaseCSV(ctx context.Context, id int64) ([]byte, error) {
	c, err := a.GetCase(ctx, id)
	if err != nil {
		return nil, err
	}
	runs, err := a.caseTraces(ctx, c)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"trace_run_id", "time", "from_chain", "from_address", "from_label", "to_chain", "to_address", "to_label",
		"action", "input_asset", "traced_input_amount", "asset", "traced_amount", "traced_usd_at_time",
		"inbound_tx_id", "tx_id", "confidence", "confidence_reason",
	})
	for _, run := range runs {
		if run.Response == nil {
			continue
		}
		nodes := traceNodeLabels(run.Response)
		for _, flow := range traceFlows(run.Response) {
			from, to := nodes[flow.edge.From], nodes[flow.edge.To]
			tx := flow.tx
			inputAmount := ""
			if tx.InputAsset != "" {
				inputAmount = strconv.FormatFloat(tx.TracedInputAmount, 'f', -1, 64)
			}
			_ = w.Write([]string{
				strconv.FormatInt(run.ID, 10), tx.Time.UTC().Format(time.RFC3339),
				from.Chain, getString(from.Metrics, "address"), from.Label,
				to.Chain, getString(to.Metrics, "address"), to.Label,
				firstNonEmpty(flow.edge.ActionLabel, flow.edge.ActionClass), tx.InputAsset, inputAmount,
				tx.Asset, strconv.FormatFloat(tx.TracedAmount, 'f', -1, 64), strconv.FormatFloat(tx.TracedUSDAtTime, 'f', 2, 64),
				tx.InboundTxID, tx.TxID, strconv.FormatFloat(tx.Confidence, 'f', 2, 64), tx.ConfidenceReason,
			})
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
