# Session Log Index

## Recent Sessions

| Date | Focus | Summary | File |
|------|-------|---------|------|
| 2026-07-09 #1 | Graph Canvas ("Map") Overhaul | Implemented all 17 map-review items (anchored incremental layout, search, dimming, hover cards, minimap, scrubber, ELK worker, server-side graph states, build progress); browser-verified on the real treasury graph, fixing 2 bugs found only in-browser | `sessions/2026-07-09/session-1.md` |
| 2026-04-17 #1 | Live Holdings Performance And Endpoint Cleanup | Cached backend metadata, slimmed live-holdings refresh, split frontend bundles, and moved THOR defaults to thorchain.network/liquify | `sessions/2026-04-17/session-1.md` |
| 2026-03-17 #1 | Mouse + Trackpad Coexistence | Added wheelDelta heuristic, extended gesture lock, tuned threshold for graph canvas | `sessions/2026-03-17/session-1.md` |

## Current Work In Progress

- Merge branch `worktree-map-improvements` into main; main checkout has uncommitted live-holdings retry changes overlapping 4 files (`useActorGraphController.ts`, `actor_tracker.go`, `service_api.go`, `ActorGraphPage.test.tsx`) that need reconciling
- Fix `.gitignore` bare `server` rule (→ `/server`) and commit `cmd/server/` + `internal/server/` — currently fresh clones/worktrees cannot build the server
- Run `/code-review` over the map-improvements diff before landing
- Investigate Treasury BTC address `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` missing actor-colored rim; confirm whether it should be added to `TC Treasury`
- Manual QA: wheel-mode Auto heuristic with real hardware (Logitech MX Master smooth scroll) — or rely on the new explicit Zoom/Pan wheel-mode preference
