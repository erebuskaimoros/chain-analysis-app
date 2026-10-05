# Session Log Index

## Recent Sessions

| Date | Focus | Summary | File |
|------|-------|---------|------|
| 2026-10-05 #4 | Phase 7: Follow-the-Funds Trace | Fixed rotated-vault/0x stitching double counts; explained confidence; forward/backward trace engine with fifo/haircut/largest_out; trace runs, API and Trace page; Bitget acceptance (31 swaps, 87.823 BTC) | `sessions/2026-10-05/session-4.md` |
| 2026-10-05 #3 | Phase 6: Actor Monitoring | Actor snapshots and watch state; refresh job reusing the ledger tail plus the holdings job; since-last-look changes by asset, address and counterparty; scheduler; Monitor panel with holdings chart | `sessions/2026-10-05/session-3.md` |
| 2026-10-05 #2 | Phase 5: Address Labels | Labels table with TagPack confidence; built-in TagPack; GraphSense, OFAC, eth-labels, ScamSniffer importers; node label categories; UI badges and category filter | `sessions/2026-10-05/session-2.md` |
| 2026-10-05 #1 | Phase 4: Prices at Transaction Time | Midgard pool/RUNE history, stable peg, DefiLlama, spot fallback; usd_at_time on all flows; min_usd at time; UI shows at-time values | `sessions/2026-10-05/session-1.md` |
| 2026-10-04 #4 | Phase 3: Jobs and Provider Health | Typed provider errors, circuit breakers, job runner, server-side live holdings with snapshots, UI on jobs, log secret redaction | `sessions/2026-10-04/session-4.md` |
| 2026-10-04 #3 | Phase 2: Ledger and Coverage | Replaced query-window caches with a row-level ledger, coverage, and deferrals; fixed partial-fill swap and shared-action dedupe bugs; validated on real data | `sessions/2026-10-04/session-3.md` |
| 2026-10-04 #2 | Phase 1: Record/Replay Safety Net | Added the record/replay HTTP cassette, golden actor-graph tests (treasury, Bitget exploiter, rebond), and split the two largest files; found the partial-fill swap bug | `sessions/2026-10-04/session-2.md` |
| 2026-10-04 #1 | App Review, Refactor Plan, Phase 0 | Reviewed the app, wrote the 9-phase refactor plan, merged the canvas overhaul, removed the legacy UI/API, and fixed dead THOR endpoint defaults (Liquify gateway) | `sessions/2026-10-04/session-1.md` |
| 2026-08-23 #1 | Memoless Registration Incident | Reconstructed the authz fee bypass, automated registration pattern, slot occupancy, and liveness impact | `sessions/2026-08-23/session-1.md` |
| 2026-07-09 #1 | Graph Canvas ("Map") Overhaul | Implemented all 17 map-review items (anchored incremental layout, search, dimming, hover cards, minimap, scrubber, ELK worker, server-side graph states, build progress); browser-verified on the real treasury graph, fixing 2 bugs found only in-browser | `sessions/2026-07-09/session-1.md` |
| 2026-04-17 #1 | Live Holdings Performance And Endpoint Cleanup | Cached backend metadata, slimmed live-holdings refresh, split frontend bundles, and moved THOR defaults to thorchain.network/liquify | `sessions/2026-04-17/session-1.md` |
| 2026-03-17 #1 | Mouse + Trackpad Coexistence | Added wheelDelta heuristic, extended gesture lock, tuned threshold for graph canvas | `sessions/2026-03-17/session-1.md` |

## Current Work In Progress

- Refactor plan: `docs/refactor-plan.md`; Phases 0–5 merged to `main`; Phase 6 (actor monitoring) is next
- Run `/code-review` over the merged map-improvements changes (landed after tests only, without a separate review pass)
- Investigate Treasury BTC address `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` missing actor-colored rim; confirm whether it should be added to `TC Treasury`
- Manual QA: wheel-mode Auto heuristic with real hardware (Logitech MX Master smooth scroll) — or rely on the new explicit Zoom/Pan wheel-mode preference
