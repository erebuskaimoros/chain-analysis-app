# Session 3 - Live Value Inline Optimization and Liquidity Engine Groundwork

> Date: 2026-03-11
> Focus: Stop actor graphs from stalling on live-value lookups, trim redundant refresh work for inline-computed nodes, and capture the parallel liquidity-engine plumbing now in the worktree

## Summary

This session tightened the actor-graph live-value path so slow external balance lookups cannot hold the initial graph response open indefinitely, and it removed redundant follow-up refresh calls for pool and validator nodes whose values are already computed inline. The worktree also now contains in-progress multi-protocol liquidity-engine scaffolding for MAYA and Radix support, so the session log captures both the completed graph-behavior change and that broader backend state.

## Work Done

- Investigated the latest missing-graph report and traced the stall to inline live-holdings enrichment continuing long after graph construction had otherwise completed.
- Added a failing backend repro that showed large slow EVM live-value batches were taking near-serial wall-clock time instead of respecting a bounded response budget.
- Changed actor-graph live-holdings enrichment to run under a batch-level timeout so slow external providers degrade to warnings instead of blocking the graph response for minutes.
- Kept pool and validator-node live values inline, and changed the backend refresh path to skip pool snapshot fetches entirely when no pool nodes are present.
- Added frontend graph helpers so later manual or bulk live-value refreshes exclude `pool` and `node` kinds and only hit address nodes that truly need external balance calls.
- Added frontend coverage proving inline-computed nodes are filtered out of refresh payloads and that manual refresh on a pool node short-circuits with an explanatory status message.
- Rebuilt the frontend bundle, ran the Go and frontend test suites, and restarted the server on port `8090`.
- Left the in-progress MAYA/Radix liquidity-engine plumbing in the worktree, including new config surface, protocol-selection helpers, broader address normalization, and protocol-aware action lookup/history helpers used by the evolving backend model.

## Discoveries

- Pool nodes and THOR validator nodes are the two safe inline live-value cases here: pools can be priced from pool snapshots and validator nodes from THOR bond state already fetched during graph construction.
- External wallet/address nodes cannot reuse graph-history calls for current balances; they still require separate point-in-time balance lookups against tracker APIs.
- Historical graph data is not a safe substitute for live-value computation because the graph is time-windowed, hop-limited, and intentionally suppresses some classes of flows.
- Bounding the whole live-value enrichment pass is more important than only bounding individual upstream calls; otherwise many slow per-address timeouts still accumulate into a hung-feeling graph build.
- The worktree now spans both completed graph fixes and larger multi-protocol backend groundwork, so end-of-session logging needs to separate shipped behavior from active architectural expansion.

## Files Changed

| File | Change |
|------|--------|
| frontend/src/features/actor-graph/hooks/useActorGraphController.ts | Filtered graph-wide live-value refreshes down to only nodes that still require external balance lookups |
| frontend/src/features/shared/graph-hooks/useSharedGraphNodeActions.ts | Short-circuited manual/unavailable-node live-value refreshes for inline-computed pool and validator nodes |
| frontend/src/features/shared/graph-hooks/__tests__/useSharedGraphNodeActions.test.tsx | Added coverage for skipping manual refresh on inline-computed pool nodes |
| frontend/src/lib/graph.ts | Re-exported the new live-value filtering helpers |
| frontend/src/lib/graph/actions.ts | Added helpers for detecting inline-computed nodes and building refreshable live-value payloads |
| frontend/src/lib/graph/__tests__/actions.test.ts | Added coverage for filtering pool/validator nodes out of live-value refresh candidates |
| internal/app/actor_tracker.go | Added a batch-level live-holdings timeout and skipped pool snapshot work when no pool nodes are present |
| internal/app/external_trackers_test.go | Added regressions for bounded live-holdings batch time and skipping pool snapshot fetches without pool nodes |
| internal/app/liquidity_protocols.go | Added shared THOR/MAYA protocol helpers and liquidity-engine selection groundwork |
| internal/app/actor_store.go | Extended address normalization for MAYA and Radix-style addresses |
| internal/app/thor_action_sources.go | Generalized action lookup/history paths to iterate protocol engines and annotate source protocol metadata |
| internal/app/app.go | Added MAYA client wiring and protocol-specific pool fetch support |
| internal/app/config.go | Added MAYA endpoint and Radix gateway configuration plus provider-candidate updates |
| internal/app/service_api.go | Expanded health snapshot and action lookup types for multi-protocol engine reporting |
| internal/app/types.go | Extended graph/action types with source-protocol metadata and updated pool fields |
| internal/web/ui/dist/index.html | Refreshed the built frontend entrypoint to point at the newest bundle |
| internal/web/ui/dist/assets/index-I6D6AcoE.js | Refreshed the built frontend bundle after the live-value refresh changes |

## In Progress

- MAYA/Radix liquidity-engine support is scaffolded in config, app wiring, and shared protocol helpers, but it is not yet fully integrated through graph/action fetch, pricing, and UI-level protocol presentation.
- A browser validation pass is still needed to confirm the updated live-value refresh UX behaves correctly on Actor Graph and Explorer with real graph payloads.
- External address live-value refresh is still available on demand; only inline-computed pool and validator nodes have been removed from the later refresh path.

## Next Steps

- [ ] Run a focused browser pass to confirm Actor Graph now renders before long external live-value lookups finish and that pool/validator refresh actions no longer issue redundant requests.
- [ ] Replay the previously stalled actor-graph flow with a large ETH-heavy node set and confirm the new batch timeout yields a graph plus warnings instead of an apparent hang.
- [ ] Finish threading MAYA/Radix liquidity-engine support through action fetch, pool pricing, and graph metadata before exposing it as a user-facing capability.
- [ ] Decide whether external address live-value refresh should remain user-triggered or move to a background/non-blocking post-load flow.
- [ ] Revisit the built-asset commit workflow if frontend bundle churn keeps dominating otherwise small graph/UI sessions.
