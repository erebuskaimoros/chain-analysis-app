# Session 3 - CALC Graph Collapse and Orphan Swap Node Fix

> Date: 2026-03-09
> Focus: Diagnose the last treasury graph run, remove orphan `Rujira THORChain Swap` edges, and keep CALC strategy executions collapsed into treasury-facing flows

## Summary

Traced the six treasury contract hashes through raw `/api/actions` responses, saved graph runs, and live graph replays to identify why the graph still showed extra CALC execution structure. The root cause was execute-only `wasm-calc-strategy/process` actions falling through the generic contract fallback, which created `executor -> Rujira THORChain Swap` edges; fixed that by reusing the known representative treasury payout for the real strategy contract and suppressing the bad fallback path.

## Work Done

- Pulled the raw Midgard action payloads for the six treasury hashes and compared them to the saved graph-run outputs.
- Confirmed the requested hashes already rendered as the intended two-edge treasury view, and isolated the real visual bug to separate execute txs in the same replay windows.
- Identified that execute-only `wasm-calc-strategy/process` rows had `funds=""` and `msg.execute[...]`, so they emitted no representative segment and dropped into the generic contract fallback.
- Added `calcPayoutByContract` tracking in the graph builder so representative CALC setup/update actions teach later execute actions which treasury address should receive collapsed TCY payouts.
- Updated contract projection so execute-only CALC rows reuse that payout mapping and do not fabricate fallback edges into `thor1n5...`.
- Added regression tests for both the positive path (execute payout reuses the treasury mapping) and the negative path (scheduler-only execute leg emits nothing without a known representative mapping).
- Ran targeted Go tests, restarted the app with `make restart-server`, verified `/api/health`, and replayed saved graph runs to confirm the orphan swap node disappeared.

## Discoveries

- The six setup/update hashes were already projecting correctly as `treasury -> contract` BTC and `contract -> treasury` TCY; the extra graph noise came from separate execution txs in the same date windows.
- Execute-only CALC process actions often contain no `funds`, no `contract_address`, and no `distribute.destinations`, so the generic contract fallback can invent incorrect address-to-address edges unless they are explicitly suppressed or backfilled from a known representative action.
- The visible `Rujira THORChain Swap` node was being materialized from the known address label on `thor1n5...`, not because it was a valid actor-connected contract hop for the desired treasury view.

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Added CALC representative payout tracking and blocked the execute-only fallback that created orphan swap-node edges |
| `internal/app/actor_tracker_test.go` | Added regressions for execute-only CALC payout reuse and fallback suppression |
| `data/bin/chain-analysis-server` | Rebuilt server binary during the approved restart flow |
| `data/logs/server-runtime.log` | Captured graph replay and restart verification logs |
| `data/logs/actor-tracker-last-run.log` | Updated last-run capture during live replay verification |
| `data/run/server.pid` | Refreshed PID after restart |

## In Progress

None - session complete for the CALC graph fallback fix.

## Next Steps

- [ ] Validate the updated treasury graph in the browser to confirm the live Cytoscape view matches the cleaned API replay
- [ ] Add coverage for `wasm-calc-manager/strategy.execute` if Midgard starts surfacing it as a visible graph candidate
- [ ] Review whether execute-only CALC supporting actions should also be collapsed or hidden in the detail pane
- [ ] Continue the broader graph cleanup backlog: Past Runs UX validation, tracker truncation monitoring, and scam/spam heuristics for EVM expansion
