# Session 7 - LP Pool Nodes and Liquidity Flow Projection

> Date: 2026-03-07
> Focus: Visualize LP pools as first-class graph nodes and preserve address-level liquidity provenance

## Summary

Updated the Actor Tracker graph projection so liquidity flows now traverse explicit pool nodes with canonical labels (for example, `Pool SOL.SOL` and `Pool BTC.BTC`). Fixed Midgard liquidity handling so `addLiquidity` actions with only `in` legs still produce usable graph edges from each provider address into the pool. This makes LP deposits/withdrawals traceable without reintroducing swap-pool noise.

## Work Done

- Added liquidity-specific event projection paths that route deposits and withdrawals through pool nodes.
- Kept swap-path graphing poolless to preserve previous noise reduction for high-volume swap activity.
- Added Midgard liquidity projection support for in-only and out-only legs using `midgardActionPool`.
- Canonicalized pool labels via `poolDisplayLabel` to ensure normalized, readable pool node names.
- Added regression tests for liquidity pool node creation, pool label normalization, and distinct pool identity (`SOL.SOL` vs `BTC.BTC`).
- Verified with `go test ./internal/app`.

## Discoveries

- Midgard `addLiquidity` often appears as two provider `in` legs with empty `out`, so requiring both legs drops valid LP actions.
- Pool identity must be normalized by asset string to avoid accidental node merging or label drift.
- Keeping pools excluded from swap-only projection still provides cleaner actor flow maps while allowing LP accounting visibility.

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Added pool-node liquidity projection for events and Midgard actions; normalized pool labels; introduced event-type helpers and pool inference |
| `internal/app/actor_tracker_test.go` | Added regression tests for LP pool-node projection, distinct pool identity (`SOL.SOL`/`BTC.BTC`), and Midgard add-liquidity provider flows |
| `internal/app/store.go` | Added `midgard_action_cache` schema + lookup/insert helpers used by actor tracker fetch flow |

## In Progress

- Validate live dashboard rendering for large multi-actor windows with the new pool-node liquidity paths

## Next Steps

- [ ] Run a production-scale graph query and confirm LP deposits/withdrawals show pool nodes with expected labels
- [ ] Add a withdrawal-focused Midgard regression test to validate pool-to-address projection
- [ ] Validate edge labels in the UI for LP events when multiple assets are aggregated on a single edge
- [ ] Expand known address labels for recurring protocol/system addresses discovered during LP tracing

