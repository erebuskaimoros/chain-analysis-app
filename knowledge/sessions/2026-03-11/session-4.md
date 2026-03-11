# Session 4 - Address Live-Value Prioritization and Protocol Provenance Surfacing

> Date: 2026-03-11
> Focus: Fix actor/address live-value starvation under the bounded lookup budget, while continuing the broader multi-protocol provenance and liquidity-engine work already in the tree

## Summary

This session fixed the main actor-graph live-value regression affecting address nodes by proving the bug with backend tests and then changing lookup scheduling so actor-owned and explorer seed nodes are enriched before the long external tail burns the batch budget. In parallel, the worktree continued to accumulate protocol-provenance and liquidity-engine changes across the backend and React graph surfaces, so this log captures both the shipped prioritization fix and the larger in-progress protocol expansion.

## Work Done

- Reproduced the address-node live-value issue with backend tests instead of patching blind, including an actor-graph build repro and a deterministic task-planning repro.
- Confirmed the narrow actor-graph unit path still populated THOR and non-THOR address node live values, which ruled out a simple frontend display bug.
- Traced the real failure mode to the bounded live-holdings batch scheduler: important actor-owned addresses could be skipped behind a large unordered external-address tail.
- Added phased live-holdings scheduling so actor-owned and explorer-target addresses are attempted first, with unsupported and lower-priority external lookups deferred until budget remains.
- Kept the earlier bounded live-holdings timeout behavior intact so slow public trackers still degrade to warnings instead of hanging the graph response.
- Refined backend regressions around THOR address live-value refresh and normalized those tests so they assert the intended behavior instead of a brittle pricing-fixture assumption.
- Ran the focused backend regressions, then the full Go suite, and restarted the local server successfully on `:8090`.
- Continued the in-progress protocol/provenance expansion already present in the worktree, including health-panel, selection-inspector, graph merge/derive, and action lookup changes that surface protocol/source metadata more broadly through the React app.

## Discoveries

- A small actor-graph repro can pass while production-like graphs still fail if the real issue is lookup-order starvation under a batch budget rather than enrichment correctness.
- Under a fixed live-holdings time budget, unordered bucket execution is enough to starve the most important nodes even when the underlying tracker integrations work.
- The right default priority is actor-owned and explorer seed addresses first, then bond/protocol-adjacent addresses, and only after that the anonymous external tail.
- Full-suite regressions are useful for catching fixture assumptions unrelated to the target bug; the THOR refresh repro needed to validate positive enrichment rather than overfitting to one specific stable-pool normalization path.
- The current worktree still mixes shipped graph-behavior fixes with broader MAYA/ARB/XRD/provenance groundwork, so end-of-session notes need to separate completed behavior from active architectural expansion.

## Files Changed

| File | Change |
|------|--------|
| internal/app/actor_tracker.go | Added phased address live-holdings scheduling and priority ordering so actor-owned/explorer nodes are enriched before the external tail |
| internal/app/actor_tracker_test.go | Added an actor-graph repro proving address nodes should receive live values during graph build |
| internal/app/external_trackers_test.go | Added scheduling-priority coverage and refreshed THOR live-value regressions |
| internal/app/external_trackers.go | Continued protocol-aware live-holdings and tracker plumbing already in progress this session |
| internal/app/liquidity_protocols.go | Expanded shared protocol/liquidity-engine helpers used by the evolving multi-protocol backend |
| internal/app/config.go | Extended provider and engine configuration for the broader multi-protocol worktree |
| internal/app/address_explorer.go | Kept explorer behavior aligned with the expanding protocol-aware backend model |
| internal/app/service_api.go | Updated service-level health/action metadata to reflect the protocol-aware backend state |
| internal/app/thor_action_sources.go | Continued generalized THOR legacy/action-source handling under the multi-source model |
| internal/api/v1_test.go | Extended API-level coverage around the evolving graph/provenance behavior |
| frontend/src/features/health/HealthPanel.tsx | Surfaced richer liquidity-engine and tracker/protocol health data |
| frontend/src/features/shared/SelectionInspector.tsx | Surfaced protocol provenance/details in node and edge inspection |
| frontend/src/features/shared/ActionLookupPanel.tsx | Continued action lookup UI work around protocol-aware responses |
| frontend/src/features/shared/SupportingActionsTable.tsx | Adjusted supporting-action presentation for the expanded provenance model |
| frontend/src/lib/graph/deriveShared.ts | Threaded live/provenance metadata through visible graph derivation |
| frontend/src/lib/graph/internals.ts | Updated graph helpers for merge/provenance behavior |
| frontend/src/lib/graph/merge.ts | Continued canonical graph merge handling with provenance-aware metadata |
| frontend/src/lib/graph/types.ts | Extended visible graph types for additional live/protocol metadata |
| frontend/src/lib/types.ts | Expanded frontend API/graph response types |
| internal/web/ui/dist/index.html | Refreshed the built frontend entrypoint |
| internal/web/ui/dist/assets/index-D4W-AfZI.js | Refreshed the built frontend bundle after the current UI/backend changes |

## In Progress

- MAYA/ARB/XRD liquidity-engine and provenance work remains active across backend fetch/pricing logic and frontend graph presentation.
- A browser pass on a large actor graph is still needed to confirm the new address-live-value prioritization behaves correctly with real public tracker latency and partial failures.
- The current session still ships built frontend assets directly from `internal/web/ui/dist`; whether to keep doing that in-session remains open.

## Next Steps

- [ ] Run a browser validation on a large actor graph and confirm actor-owned address nodes now receive live values before the external tail exhausts the batch budget.
- [ ] Finish threading the multi-protocol liquidity-engine work through pricing, graph fetch, and UI provenance so MAYA/related engines behave as first-class sources.
- [ ] Decide whether external address live-value refresh should remain batch-budgeted inline work or move further toward post-load/background enrichment.
- [ ] Clean up the remaining protocol/provenance UI surface so health, selection, lookup, and supporting-action views present a consistent source model.
- [ ] Revisit whether built `internal/web/ui/dist` assets should continue to be committed as part of iterative feature sessions.
