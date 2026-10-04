package main

import (
	"context"
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
