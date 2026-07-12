# chain-analysis-app Agent Notes

## Scope

- This repo owns local on-demand THORChain ingestion, SQLite caching, action/actor analysis, and graph exploration.
- BooneTools owns public/operator dashboards and product tooling; this app remains the local investigative surface.
- Rujira docs here are analysis/taxonomy material, while contract truth lives in `../Rujira`.

## Server Restart Policy

- Restart from this directory with `make restart-server` or `./scripts/restart-server.sh restart`.
- Do not use `pkill -f "go run ./cmd/server"` as the primary restart method.
- After restart, run `curl -s http://localhost:8090/api/health` and verify `build.commit` and `build.build_time` reflect the expected code.

## Shared Context

- Read `../AGENTS.md`, `../knowledge/projects/chain-analysis-app.md`, and `../knowledge/workstreams/analytics-and-tooling.md`.
- Keep detailed session notes in `knowledge/`; update the shared wiki when durable project understanding changes.
