# Session 1 - Live Holdings Performance And Endpoint Cleanup

> Date: 2026-04-17
> Focus: Speed up actor graph execution, reduce frontend bundle cost, and update preferred THOR endpoints

## Summary

This session focused on reducing the cost of actor graph execution and live-value refreshes across both the backend and frontend. The app now reuses protocol metadata and pool price books across requests, sends a slimmer live-holdings payload, avoids full graph relayouts for metric-only updates, lazy-loads major frontend views, and prefers `thorchain.network` with `liquify` fallback for THORNode and Midgard.

## Work Done

- Added shared TTL metadata caches for protocol directory and price book construction so graph build, expand, explorer, and live-holdings refresh stop refetching the same upstream state on every request.
- Slimmed the live-holdings API contract so the client sends only refresh-relevant node fields and receives metric-only node updates back.
- Updated the Cytoscape canvas path to preserve layout and viewport for live-value-only updates instead of tearing down the graph and rerunning ELK on every node metric change.
- Added backend regression coverage for cache deduplication/cloning and API coverage for the slim live-holdings payload.
- Split the frontend shell into lazy-loaded view chunks, deferred `GraphCanvas` behind graph presence, and added targeted Vite vendor chunking for React, React Query, Cytoscape, and ELK.
- Switched THOR endpoint defaults to prefer `thornode.thorchain.network` and `midgard.thorchain.network/v2`, then removed Nine Realms from the default THOR endpoint lists entirely.
- Rebuilt the UI, reran backend and frontend tests multiple times, restarted the local server, and verified the running health snapshot after each major configuration change.
- Began tracing why some Treasury addresses do not show the actor-colored border; the concrete discovery so far is that `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` is not present in the local `TC Treasury` actor record, so it cannot render as an `actor_address` node.

## Discoveries

- The original live-holdings path was rebuilding protocol inbound metadata and pool price books independently in several handlers; those network calls were a measurable source of latency and variance.
- The graph canvas was treating any node data change as a topology change. Live-holdings refreshes therefore triggered a full Cytoscape teardown and ELK relayout even when the node set and edge set were unchanged.
- Code-splitting was highly effective for the app shell and page entrypoints, but `elk.bundled.js` still produces a large async chunk. That cost is now deferred until the graph canvas loads, but it is still the dominant remaining frontend bundle weight.
- THOR endpoint ordering is visible through `/api/v1/health`, which makes runtime verification straightforward after endpoint preference changes.
- The green actor rim is derived from visible node kind, not just address ownership. Only visible nodes of kind `actor` or `actor_address` get the actor color border. If an address is absent from the actor store, or is rendered as another node kind, it will not receive the rim.

## Files Changed

| File | Change |
|------|--------|
| internal/app/metadata_cache.go | Added shared TTL metadata cache helpers with request deduplication and cloning |
| internal/app/metadata_cache_test.go | Added regression coverage for cache reuse and mutation isolation |
| internal/app/actor_tracker.go | Routed protocol/price metadata through caches and preserved partial price-book results |
| internal/api/dto/types.go | Added slim live-holdings request/response DTOs |
| internal/api/v1.go | Translated live-holdings requests into partial node updates and returned metric-only updates |
| internal/api/v1_test.go | Added API coverage for the slim live-holdings payload |
| frontend/src/lib/api.ts | Sent minimal live-holdings request nodes from the client |
| frontend/src/lib/types.ts | Added frontend types for live-holdings refresh payloads |
| frontend/src/lib/graph/merge.ts | Allowed partial node updates to merge cleanly into the current graph |
| frontend/src/features/actor-graph/hooks/useActorGraphController.ts | Updated background refresh flow for partial node updates |
| frontend/src/features/shared/graph-canvas/useGraphCanvasCore.ts | Avoided full graph rebuilds for metric-only updates |
| frontend/src/App.tsx | Lazy-loaded top-level app views with preload-on-hover/focus |
| frontend/src/features/actor-graph/ActorGraphPage.tsx | Deferred `GraphCanvas` until graph content exists |
| frontend/src/features/explorer/ExplorerPage.tsx | Deferred `GraphCanvas` until graph content exists |
| frontend/vite.config.ts | Added vendor/manual chunking for React, React Query, Cytoscape, and ELK |
| internal/app/config.go | Updated THOR endpoint defaults to `thorchain.network` first, then removed Nine Realms from the THOR defaults |
| internal/app/thor_client_test.go | Updated endpoint-order tests to reflect current THOR Midgard host ordering |
| README.md | Updated documented THOR endpoint defaults |
| internal/web/ui/dist/ | Rebuilt the production UI bundle with split chunks |

## In Progress

Investigating why some Treasury addresses do not show actor-colored borders. Current evidence indicates the specific BTC address `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` is not stored in the local `TC Treasury` actor record, so it is not recognized as an owned `actor_address`.

## Next Steps

- [ ] Confirm in the UI or database whether `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` should be part of `TC Treasury`, and add it if so.
- [ ] Rebuild the relevant actor graph after correcting Treasury ownership data and verify the address renders as `actor_address` with the actor-colored border.
- [ ] Decide whether `elk-vendor` is acceptable as a deferred async chunk or whether the layout engine should be further split/replaced.
- [ ] Run manual QA on large actor graphs to confirm live-value refreshes no longer trigger noticeable full relayouts.
- [ ] Consider ignoring generated/session-local artifacts such as `Saved Graphs/*.json` and test log outputs if they should stay out of future commits.
