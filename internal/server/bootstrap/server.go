package bootstrap

import (
	"context"
	"net/http"
	"time"

	"chain-analysis-app/internal/api"
	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/domain/services"
	"chain-analysis-app/internal/web/ui"
)

type Runtime struct {
	App      *app.App
	Services *services.Container
	Server   *http.Server
}

func New(cfg app.Config) (*Runtime, error) {
	legacy, err := app.New(cfg)
	if err != nil {
		return nil, err
	}

	legacy.StartBackground()
	svcs := services.New(legacy)
	v1 := api.NewV1(svcs)
	mux := http.NewServeMux()
	v1.Register(mux)
	mux.Handle("/", ui.NewHandler(cfg.UIBuildDir, cfg.UIBuildDirOverride))

	server := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Runtime{
		App:      legacy,
		Services: svcs,
		Server:   server,
	}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.App == nil {
		return nil
	}
	return r.App.Close()
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || r.Server == nil {
		return nil
	}
	return r.Server.Shutdown(ctx)
}
