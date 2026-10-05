# Session 3 - Phase 2: Ledger, Coverage, and Two Projection Fixes

> Date: 2026-10-04
> Focus: Replace query-window caches with a row-level ledger (refactor plan Phase 2) and fix two projection bugs found along the way

## Summary

Midgard actions and tracker transfers are now stored row by row per queried
address (migration 5).
- Fetches request only uncovered time ranges and read the window back from the
  ledger in upstream order. The last 10 minutes are never marked covered.
- Truncated ranges are deferred for 24 hours instead of being re-downloaded on
  every build.
- Existing cache rows were backfilled on startup and the old tables dropped.
  Rehearsed on a copy of the real 241 MB database: about 30 s, 26,880 unique
  actions, 232,858 transfers, 472 coverage intervals.
- The legacy Vanaheim action source is now opt-in.

## Fixes landed on main first (each test-first, fixed by a subagent)

- `435fb6b` Partial-fill swaps:
  - Midgard attaches a partial fill's outbounds to both the swap and a refund action. Refund suppression dropped the whole swap.
  - The Bitget exploiter graph now shows all 31 swaps (2,790.32 ETH → 87.82 BTC).
- `67529ed` Shared actions between traced addresses:
  - The first frontier marked an action as seen but kept only its own segments, so the other address's leg (for example the asset side of a symmetric add) was lost depending on processing order.
  - A per-build action ledger now re-projects for each frontier and counts every segment once. The treasury golden regains two asset legs.

## Real-data validation (saved run #79: TC Treasury 2018 → 2026-04-17, 1 hop)

| | Nodes | Edges | Warm build |
|---|---|---|---|
| `main` (old caches) | 366 | 495 | 16 s |
| Ledger build | 405 | 556 | 16 s (first build 47 s) |

- `main` stops at its 20-page Midgard cap; the ledger serves deeper history.
- No edge is missing from the ledger build.
- The one transaction `main` shows on an extra TRON transfer edge was a double count (swap output plus vault transfer), which the ledger build stitches correctly.

## Discoveries

- Liquify Midgard has THOR history back to at least 2021. Vanaheim returned nothing for 2021 windows and only duplicated recent ones.
- EVM tracker tx IDs keep their `0x` prefix, so they never match Midgard tx IDs in refund suppression and stitching. To be examined in Phase 7.

## Next

Phase 3: typed provider errors, circuit breakers, job runner, server-side live holdings.
