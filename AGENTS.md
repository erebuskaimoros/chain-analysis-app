# chain-analysis-app Agent Notes

## Scope

- This repo owns local on-demand THORChain ingestion, SQLite caching, action/actor analysis, and graph exploration.
- BooneTools owns public/operator dashboards and product tooling; this app remains the local investigative surface.
- Rujira docs here are analysis/taxonomy material, while contract truth lives in `../Rujira`.

## Server Restart Policy

- Restart from this directory with `make restart-server` or `./scripts/restart-server.sh restart`.
- Do not use `pkill -f "go run ./cmd/server"` as the primary restart method.
- After restart, run `curl -s http://localhost:8090/api/v1/health` and verify `build.commit` and `build.build_time` reflect the expected code.

## Shared Context

In a linked worktree, resolve shared parent/sibling paths from the primary
checkout shown by `git worktree list --porcelain`; use the current worktree for
code and its local documentation. Do not assume branch-directory depth matches
the canonical checkout. Read only context relevant to the requested change.

- For substantial domain changes or cross-project work, read `../AGENTS.md`, `../knowledge/projects/chain-analysis-app.md`, and `../knowledge/workstreams/analytics-and-tooling.md`.
- Keep detailed session notes in `knowledge/`; update the shared wiki when durable project understanding changes.
