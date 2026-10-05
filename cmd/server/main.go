package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/server/bootstrap"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	cfg := app.LoadConfigFromEnv()
	if len(os.Args) > 1 && os.Args[1] == "labels" {
		os.Exit(runLabelsCommand(cfg, os.Args[2:]))
	}
	if strings.TrimSpace(version) != "" {
		cfg.BuildVersion = strings.TrimSpace(version)
	}
	if strings.TrimSpace(commit) != "" {
		cfg.BuildCommit = strings.TrimSpace(commit)
	}
	if strings.TrimSpace(buildTime) != "" {
		cfg.BuildTime = strings.TrimSpace(buildTime)
	}

	runtime, err := bootstrap.New(cfg)
	if err != nil {
		log.Fatalf("failed to start app: %v", err)
	}
	defer runtime.Close()

	go func() {
		log.Printf("chain-analysis-app listening on %s (version=%s commit=%s build_time=%s)", cfg.BindAddr, cfg.BuildVersion, cfg.BuildCommit, cfg.BuildTime)
		if err := runtime.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*cfg.RequestTimeout)
	defer cancel()
	_ = runtime.Shutdown(shutdownCtx)
}

// runLabelsCommand handles `labels import <source> <path>` and
// `labels sources` against the configured database.
func runLabelsCommand(cfg app.Config, args []string) int {
	usage := "usage: chain-analysis-server labels import <graphsense|ofac|eth-labels|scamsniffer> <path>\n       chain-analysis-server labels sources"
	a, err := app.New(cfg)
	if err != nil {
		log.Printf("open app: %v", err)
		return 1
	}
	defer a.Close()
	ctx := context.Background()
	switch {
	case len(args) == 3 && args[0] == "import":
		result, err := a.ImportLabels(ctx, args[1], args[2])
		if err != nil {
			log.Printf("import %s labels: %v", args[1], err)
			return 1
		}
		fmt.Printf("imported %d %s labels\n", result.Count, result.Source)
		return 0
	case len(args) == 1 && args[0] == "sources":
		sources, err := a.LabelSources(ctx)
		if err != nil {
			log.Printf("list label sources: %v", err)
			return 1
		}
		for _, s := range sources {
			fmt.Printf("%-12s %8d labels  imported %s  (%s, %s)\n", s.Source, s.Count, s.ImportedAt, s.Version, s.License)
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
}
