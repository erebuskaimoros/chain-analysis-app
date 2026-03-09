# Session 8 - Single-Hop Swap Projection and Cross-Source Dedupe

> Date: 2026-03-07
> Focus: Ensure swaps are represented once as direct sender-to-recipient liquidity movement

## Summary

Refined Actor Tracker swap modeling so swaps render as a single hop (`sender -> recipient`) and no longer use intermediary Asgard/pool nodes in swap projection. Added cross-source suppression to avoid representing the same swap twice when both THORNode events and Midgard actions are present for the same tx. This aligns graph behavior with the goal of clean, non-duplicative liquidity flow tracing.

## Work Done

- Updated Midgard swap projection to skip Asgard module legs and emit direct sender-to-recipient swap segments.
- Added cross-source swap dedupe: event-path swap segments are suppressed when Midgard already contains the same successful swap tx id.
- Kept liquidity provisioning behavior intact with explicit pool nodes for add/withdraw flows.
- Added/updated regression tests for direct swap projection and swap tx-id suppression behavior.
- Restarted server and verified app availability on `:8090`.

## Discoveries

- The graph pipeline currently merges two swap sources (events + Midgard), so explicit tx-level suppression is required to avoid double representation.
- Asgard blacklisting alone only prevents expansion; it does not guarantee swap middleman removal unless swap projection logic explicitly bypasses Asgard legs.

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Added swap single-hop projection and Midgard-vs-event swap dedupe by tx id; helper utilities for swap tx collection/suppression |
| `internal/app/actor_tracker_test.go` | Updated swap behavior tests to assert direct swap hops and cross-source swap suppression |

## In Progress

- Validate the new single-hop swap behavior in larger live windows to confirm no residual duplicate swap edges

## Next Steps

- [ ] Run a multi-actor, multi-day query and confirm each swap tx appears once in the graph
- [ ] Add API-level integration coverage for mixed event/Midgard swap overlap scenarios
- [ ] Decide whether to expose source precedence (`midgard` vs `events`) in graph debug metadata
- [ ] Continue expanding protocol address labeling only where it improves liquidity-flow readability
