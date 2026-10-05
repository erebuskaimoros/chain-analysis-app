// Command cactl is a command-line client for a running chain-analysis server.
// Every command prints JSON on stdout (case export prints the report itself);
// progress goes to stderr.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/client"
)

const usage = `usage: cactl [--url URL] <command> [arguments]

Commands:
  trace       --seed CHAIN|ADDRESS (repeatable) | --tx HASH, --start, --end,
              --direction forward|backward, --policy fifo|haircut|largest_out,
              --max-depth N, --max-branches N, --amount X --asset ASSET,
              --min-usd X, --stop exchange,sanctioned,mixer, --holdings
  actor       refresh <id|name> | summary <id|name>
  labels      import <source> <path> | lookup <address>
  case        list | show <id> | create <title> [--notes TEXT] |
              add <id> <address|tx|trace_run|graph_state|actor> <ref> [--note TEXT] |
              export <id> [--format md|csv] [--out FILE]
  ledger      gaps <address> [--start RFC3339] [--end RFC3339]
  profile     <address> [--chain CHAIN] [--days N]
  tx          <hash>
  health

The server URL defaults to $CHAIN_ANALYSIS_URL, then http://localhost:8090.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "cactl:", err)
		os.Exit(1)
	}
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	global := flag.NewFlagSet("cactl", flag.ContinueOnError)
	global.SetOutput(stderr)
	baseURL := global.String("url", "", "chain-analysis server URL")
	global.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		global.Usage()
		return errors.New("a command is required")
	}
	c := client.New(*baseURL)
	progress := func(job client.Job) {
		fmt.Fprintf(stderr, "\r%s: %s %s", job.Kind, job.Stage, job.Message)
	}
	emit := func(value any, err error) error {
		fmt.Fprint(stderr, "\r")
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}

	command, params := rest[0], rest[1:]
	switch command {
	case "health":
		return emit(c.Health(ctx))
	case "trace":
		req, err := parseTraceFlags(params, stderr)
		if err != nil {
			return err
		}
		return emit(c.Trace(ctx, req, progress))
	case "actor":
		if len(params) != 2 || (params[0] != "refresh" && params[0] != "summary") {
			return errors.New("usage: cactl actor refresh|summary <id|name>")
		}
		actor, err := c.ResolveActor(ctx, params[1])
		if err != nil {
			return err
		}
		if params[0] == "refresh" {
			return emit(c.ActorRefresh(ctx, actor.ID, progress))
		}
		return emit(c.ActorMonitor(ctx, actor.ID))
	case "labels":
		switch {
		case len(params) == 3 && params[0] == "import":
			return emit(c.LabelsImport(ctx, params[1], params[2], progress))
		case len(params) == 2 && params[0] == "lookup":
			return emit(c.Labels(ctx, params[1]))
		}
		return errors.New("usage: cactl labels import <source> <path> | lookup <address>")
	case "case":
		return runCase(ctx, c, params, stdout, stderr, emit)
	case "ledger":
		if len(params) < 2 || params[0] != "gaps" {
			return errors.New("usage: cactl ledger gaps <address> [--start RFC3339] [--end RFC3339]")
		}
		fs := flag.NewFlagSet("ledger gaps", flag.ContinueOnError)
		fs.SetOutput(stderr)
		start := fs.String("start", "", "window start (RFC 3339)")
		end := fs.String("end", "", "window end (RFC 3339)")
		if err := fs.Parse(params[2:]); err != nil {
			return err
		}
		from, err := parseOptionalTime(*start)
		if err != nil {
			return err
		}
		to, err := parseOptionalTime(*end)
		if err != nil {
			return err
		}
		return emit(c.LedgerCoverage(ctx, params[1], from, to))
	case "profile":
		if len(params) < 1 {
			return errors.New("usage: cactl profile <address> [--chain CHAIN] [--days N]")
		}
		fs := flag.NewFlagSet("profile", flag.ContinueOnError)
		fs.SetOutput(stderr)
		chain := fs.String("chain", "", "chain of the address when ambiguous")
		days := fs.Int("days", 30, "days of flows to summarise")
		if err := fs.Parse(params[1:]); err != nil {
			return err
		}
		return emit(c.AddressProfile(ctx, app.AddressProfileRequest{Chain: *chain, Address: params[0], Days: *days}, progress))
	case "tx":
		if len(params) != 1 {
			return errors.New("usage: cactl tx <hash>")
		}
		return emit(c.LookupTx(ctx, params[0]))
	}
	global.Usage()
	return fmt.Errorf("unknown command %q", command)
}

func parseOptionalTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: want RFC 3339 such as 2026-09-28T03:00:00Z", raw)
	}
	return t, nil
}

func parseTraceFlags(params []string, stderr io.Writer) (app.TraceRequest, error) {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var seeds, txs stringList
	fs.Var(&seeds, "seed", "seed address, CHAIN|ADDRESS (repeatable)")
	fs.Var(&txs, "tx", "seed transaction hash (repeatable)")
	start := fs.String("start", "", "window start (RFC 3339)")
	end := fs.String("end", "", "window end (RFC 3339)")
	direction := fs.String("direction", "forward", "forward or backward")
	policy := fs.String("policy", "fifo", "fifo, haircut or largest_out")
	maxDepth := fs.Int("max-depth", 3, "hops to follow")
	maxBranches := fs.Int("max-branches", 8, "branches followed per address")
	amount := fs.Float64("amount", 0, "amount to trace (default: everything)")
	asset := fs.String("asset", "", "asset of --amount, such as ETH.ETH")
	minUSD := fs.Float64("min-usd", 0, "minimum USD at transaction time to follow a branch")
	stops := fs.String("stop", "exchange,sanctioned,mixer", "label categories that end the trace")
	holdings := fs.Bool("holdings", false, "look up what each endpoint holds now")
	if err := fs.Parse(params); err != nil {
		return app.TraceRequest{}, err
	}
	req := app.TraceRequest{
		StartTime: *start, EndTime: *end, Direction: *direction, Policy: *policy,
		MaxDepth: *maxDepth, MaxBranches: *maxBranches, Amount: *amount, Asset: *asset,
		MinUSDAtTime: *minUSD, IncludeHoldings: *holdings, StopCategories: []string{},
	}
	for _, category := range strings.Split(*stops, ",") {
		if category = strings.TrimSpace(category); category != "" {
			req.StopCategories = append(req.StopCategories, category)
		}
	}
	for _, seed := range seeds {
		chain, address, ok := strings.Cut(seed, "|")
		if !ok {
			chain, address = "", seed
		}
		req.Seeds = append(req.Seeds, app.TraceSeed{Chain: strings.ToUpper(chain), Address: address})
	}
	for _, tx := range txs {
		req.Seeds = append(req.Seeds, app.TraceSeed{TxID: tx})
	}
	if len(req.Seeds) == 0 {
		return app.TraceRequest{}, errors.New("trace needs at least one --seed or --tx")
	}
	return req, nil
}

func runCase(ctx context.Context, c *client.Client, params []string, stdout, stderr io.Writer, emit func(any, error) error) error {
	if len(params) == 0 {
		return errors.New("usage: cactl case list|show|create|add|export")
	}
	parseID := func(raw string) (int64, error) {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("invalid case id %q", raw)
		}
		return id, nil
	}
	switch params[0] {
	case "list":
		return emit(c.Cases(ctx))
	case "show":
		if len(params) != 2 {
			return errors.New("usage: cactl case show <id>")
		}
		id, err := parseID(params[1])
		if err != nil {
			return err
		}
		return emit(c.Case(ctx, id))
	case "create":
		if len(params) < 2 {
			return errors.New("usage: cactl case create <title> [--notes TEXT]")
		}
		fs := flag.NewFlagSet("case create", flag.ContinueOnError)
		fs.SetOutput(stderr)
		notes := fs.String("notes", "", "Markdown notes")
		if err := fs.Parse(params[2:]); err != nil {
			return err
		}
		return emit(c.CreateCase(ctx, app.CaseUpsertRequest{Title: params[1], NotesMD: *notes}))
	case "add":
		if len(params) < 4 {
			return errors.New("usage: cactl case add <id> <kind> <ref> [--note TEXT]")
		}
		id, err := parseID(params[1])
		if err != nil {
			return err
		}
		fs := flag.NewFlagSet("case add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		note := fs.String("note", "", "note on the item")
		if err := fs.Parse(params[4:]); err != nil {
			return err
		}
		return emit(c.AddCaseItem(ctx, id, app.CaseItemRequest{Kind: params[2], Ref: params[3], Note: *note}))
	case "export":
		if len(params) < 2 {
			return errors.New("usage: cactl case export <id> [--format md|csv] [--out FILE]")
		}
		id, err := parseID(params[1])
		if err != nil {
			return err
		}
		fs := flag.NewFlagSet("case export", flag.ContinueOnError)
		fs.SetOutput(stderr)
		format := fs.String("format", "md", "md or csv")
		out := fs.String("out", "", "write to this file instead of stdout")
		if err := fs.Parse(params[2:]); err != nil {
			return err
		}
		data, err := c.CaseExport(ctx, id, *format)
		if err != nil {
			return err
		}
		if *out != "" {
			return os.WriteFile(*out, data, 0o644)
		}
		_, err = stdout.Write(data)
		return err
	}
	return fmt.Errorf("unknown case command %q", params[0])
}
