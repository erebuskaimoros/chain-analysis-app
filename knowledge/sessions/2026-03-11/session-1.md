# Session 1 - React Graph Parity and Explorer Expansion Cleanup

> Date: 2026-03-11
> Focus: Restore React graph behavior to legacy parity, fix Explorer UI regressions, and stop duplicate Explorer expansion actions from surviving merges

## Summary

The React graph surface was brought much closer to legacy behavior by restoring fullscreen behavior, right-click placement, multi-select context actions, and graph-side layout/interaction parity. On the Explorer side, expansion merges now canonicalize supporting actions onto merged node IDs and the client only requests newly discovered seeds, which fixes repeated txn counts and removes the wasteful full-seed replay on every expansion.

## Work Done

- Ported the legacy graph pipeline into the shared React graph utilities and rewired Actor Graph and Explorer pages onto the shared typed merge/derive/filter stack.
- Restored graph surface behavior including ELK layout, toolbar actions, fullscreen handling, right-click menus, multi-select actions, and context-menu positioning.
- Reorganized the Explorer page so Preview and Saved Runs sit above the graph, with Selection Detail above the action panels.
- Investigated the most recent Explorer expansion logs and traced repeated action counts to supporting-action merges that keyed on raw node IDs instead of canonical merged node IDs.
- Fixed Explorer expansion dedupe by canonicalizing `supporting_actions.from_node` and `to_node` through the node alias map before merge-time dedupe.
- Changed Explorer expansion requests to send only newly discovered seeds instead of replaying the full expanded seed set on every click.
- Added regression coverage for multi-select context-menu behavior and Explorer expansion supporting-action dedupe.
- Rebuilt the frontend bundle and repeatedly restarted the app server to verify the updated UI and runtime path.

## Discoveries

- Explorer one-hop expansion intentionally rebuilds a graph for the requested seed set, so frontend merge correctness depends on canonicalizing every action and edge onto merged node identities.
- Deduping `supporting_actions` by raw `from_node` and `to_node` is insufficient when the backend can re-emit the same logical action with different transient node IDs across expansion responses.
- Once merge-time action canonicalization is correct, Explorer expansions do not need to resend the cumulative seed set; delta-only seed requests are enough and reduce redundant Midgard fetches.
- Right-click behavior for multi-select is cleaner when node-hit detection returns the actual selected node instead of a boolean, because the interaction layer can choose between single-node and grouped actions without guessing.

## Files Changed

| File | Change |
|------|--------|
| frontend/src/features/explorer/hooks/useExplorerGraphController.ts | Sent only new Explorer expansion seeds and updated expansion status messaging |
| frontend/src/features/shared/GraphCanvas.tsx | Reduced the top-level canvas wrapper to orchestration after moving more interaction logic into split hooks |
| frontend/src/features/shared/graph-canvas/useGraphCanvasCore.ts | Simplified core Cytoscape setup and selection behavior after moving native context-menu handling out of the core hook |
| frontend/src/features/shared/graph-canvas/useGraphCanvasInteractions.ts | Fixed right-click placement/selection behavior and added grouped multi-select context-menu handling |
| frontend/src/features/shared/graph-canvas/utils.ts | Returned concrete hit-test nodes, added selected-node extraction helpers, and shared context-menu coordinate helpers |
| frontend/src/features/shared/graph-canvas/__tests__/GraphCanvas.multiSelectContextMenu.test.tsx | Added regression coverage for grouped right-click context-menu behavior |
| frontend/src/lib/graph/merge.ts | Canonicalized supporting-action node IDs before merge-time dedupe so repeated Explorer txns do not survive as duplicates |
| frontend/src/lib/graph/__tests__/merge.test.ts | Added a regression test for duplicate Explorer actions arriving with different raw node IDs |
| frontend/src/lib/graph/presentation.ts | Restored edge endpoint label styling for the refreshed graph rendering |
| internal/web/ui/dist/index.html | Refreshed the built frontend entrypoint |
| internal/web/ui/dist/assets/index-BU4UeOQz.js | Refreshed the built frontend bundle after the graph and Explorer fixes |

## In Progress

- Manual browser validation is still needed for the full React graph parity surface, especially repeated Explorer expansions, grouped node actions, and any remaining fullscreen/context-menu edge cases.

## Next Steps

- [ ] Replay the same Explorer expansion flow in the browser and confirm repeated txn counts now stay stable across successive expansions.
- [ ] Do a focused manual parity pass on Actor Graph and Explorer multi-select, node actions, and fullscreen behavior.
- [ ] Decide whether to add broader merge tests for actor graph supporting-action canonicalization as a follow-up hardening pass.
- [ ] Trim or split the frontend bundle if the large-chunk warning keeps growing.
