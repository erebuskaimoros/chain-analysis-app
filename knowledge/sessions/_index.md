# Session Log Index

## Recent Sessions

| Date | Focus | Summary | File |
|------|-------|---------|------|
| 2026-10-04 #3 | Phase 2: Ledger and Coverage | Replaced query-window caches with a row-level ledger, coverage, and deferrals; fixed partial-fill swap and shared-action dedupe bugs; validated on real data | `sessions/2026-10-04/session-3.md` |
| 2026-10-04 #2 | Phase 1: Record/Replay Safety Net | Added the record/replay HTTP cassette, golden actor-graph tests (treasury, Bitget exploiter, rebond), and split the two largest files; found the partial-fill swap bug | `sessions/2026-10-04/session-2.md` |
| 2026-10-04 #1 | App Review, Refactor Plan, Phase 0 | Reviewed the app, wrote the 9-phase refactor plan, merged the canvas overhaul, removed the legacy UI/API, and fixed dead THOR endpoint defaults (Liquify gateway) | `sessions/2026-10-04/session-1.md` |
| 2026-08-23 #1 | Memoless Registration Incident | Reconstructed the authz fee bypass, automated registration pattern, slot occupancy, and liveness impact | `sessions/2026-08-23/session-1.md` |
| 2026-07-09 #1 | Graph Canvas ("Map") Overhaul | Implemented all 17 map-review items (anchored incremental layout, search, dimming, hover cards, minimap, scrubber, ELK worker, server-side graph states, build progress); browser-verified on the real treasury graph, fixing 2 bugs found only in-browser | `sessions/2026-07-09/session-1.md` |
| 2026-04-17 #1 | Live Holdings Performance And Endpoint Cleanup | Cached backend metadata, slimmed live-holdings refresh, split frontend bundles, and moved THOR defaults to thorchain.network/liquify | `sessions/2026-04-17/session-1.md` |
| 2026-03-17 #1 | Mouse + Trackpad Coexistence | Added wheelDelta heuristic, extended gesture lock, tuned threshold for graph canvas | `sessions/2026-03-17/session-1.md` |

## Current Work In Progress

- Refactor plan: `docs/refactor-plan.md`; Phases 0–2 merged to `main`; Phase 3 (jobs, provider errors, server-side live holdings) is next
- Run `/code-review` over the merged map-improvements changes (landed after tests only, without a separate review pass)
- Investigate Treasury BTC address `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` missing actor-colored rim; confirm whether it should be added to `TC Treasury`
- Manual QA: wheel-mode Auto heuristic with real hardware (Logitech MX Master smooth scroll) — or rely on the new explicit Zoom/Pan wheel-mode preference
