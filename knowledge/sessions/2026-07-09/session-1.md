# Session 1 - Graph Canvas ("Map") Overhaul

> Date: 2026-07-09
> Focus: Full implementation of the 17-item map review — layout, interaction, investigation features, perf, and backend persistence — in worktree `worktree-map-improvements`, browser-verified against the real treasury graph

## Summary

Implemented every item from the graph-canvas review in an isolated worktree: incremental anchored layout (expansions no longer scramble the map), zoom clamping, instant selection, node search, neighborhood highlighting, hover cards, a minimap, a time scrubber, space+drag pan with a wheel-mode preference, a persistent label layer, ELK in a web worker, server-side saved graph states, and live build-progress polling. Everything was verified end-to-end in Chrome against the 34MB treasury graph (1,643 nodes), which surfaced two real bugs unit tests missed — both fixed with regression tests. Final state: 85/85 frontend tests, full Go suite green, production build + live API smoke tests pass.

## Work Done

- **Canvas bug fixes**: diff-based element sync with anchored placement for new nodes (replaces coordinate-frame-mismatched global ELK relayout on expansion); cytoscape `minZoom`/`maxZoom` set (0.05–10), fixing the falsy-clamp bug; single-click selection now immediate except on primary-action nodes (`nodeHasPrimaryAction` prop); drags tracked at window level (capture phase) so they survive leaving the canvas; fullscreen enter/exit preserves the viewport; edge labels render once (was 3×) with `min-zoomed-font-size` culling
- **Investigation features**: node search (`/` shortcut, match highlighting, Enter/Shift+Enter cycling, animated centering); neighborhood highlight (dims everything outside the selection's closed neighborhood, HTML labels dim too); hover cards (address, kind/chain, live holdings, flow USD); click/drag minimap with viewport rectangle; dual-handle time scrubber driving the existing time filters (actor + explorer pages)
- **Ergonomics**: space+drag panning; wheel-mode toolbar toggle (Auto/Zoom/Pan) persisted in localStorage as an escape hatch from trackpad heuristics
- **Performance**: label layer rewritten as a persistent DOM pool (transform-only updates, viewport + zoom culling); ELK layout moved to a web worker via `elkjs/lib/elk-api` (main thread now loads a 5KB chunk instead of 1.4MB; bundled engine kept as lazy fallback split into its own chunk)
- **Backend**: `graph_states` table (sqlite migration id 4) + `GET|POST /api/v1/graph-states`, `GET|DELETE /api/v1/graph-states/{id}`; toolbar save now prompts for a name and saves server-side, with a "Saved Graph States" sidebar section (Load/Export/Delete); saved payloads no longer embed derived `visible_graph`/`filtered_actions`; actor-graph build progress via `progress_token` on the request, an in-memory registry hooked into the Midgard fetch loops, `GET /api/v1/analysis/actor-graph/progress/{token}`, and frontend polling into the status line
- **Testing**: extracted a much more capable shared cytoscape mock (`test-support/cytoscapeMock.ts`), added `GraphCanvas.mapImprovements.test.tsx` (11 tests), Go tests for graph-state CRUD/validation and the progress registry; 85 frontend + full Go suite green
- **Browser verification** (Claude in Chrome, real treasury graph): confirmed search, hover, dimming (2,960 dimmed / 161 highlighted on TreasuryLP-ETH), minimap pan, wheel-mode persistence, scrubber live-filtering 1,643→417→1,643 nodes, zoom clamp on the live instance, zero console errors
- **Bugs found only in browser, then fixed + regression-tested**: (1) surface mousedown/wheel handlers hijacked overlay widgets — the search box could never take focus; fixed with an overlay-target guard; (2) node labels rendered as vertical one-character columns — the label pool's zero-width anchor collapsed shrink-to-fit width; fixed with `width: max-content` on `.graph-node-text`; also moved the search box below the fullscreen header

## Discoveries

- **`.gitignore` bare `server` rule hides the server entrypoint** — `cmd/server/main.go` and `internal/server/bootstrap/` have never been committed; fresh clones/worktrees cannot build the server until those are copied from a working checkout. Fix would be changing the rule to `/server` and committing both directories. (Promoted to auto-memory; evergreen.)
- **The UI is `go:embed`ded** (`internal/web/ui/embed.go`) — after `npm run build`, the Go binary must be rebuilt or the server serves stale assets. Cost a debugging loop during verification.
- Absolutely-positioned label text inside a zero-width anchor resolves shrink-to-fit width against the anchor → one character per line; `width: max-content` is the fix
- `preventDefault()` on mousedown anywhere in the canvas surface blocks focus for overlay inputs rendered inside it — gesture handlers need an overlay-target exemption
- Cytoscape instances are reachable in the browser console via `container._cyreg.cy` — invaluable for live verification
- ELK position application must be cancellable (`isCancelled` check before `layout().run()`), or a stale async layout stomps positions applied by a newer incremental sync
- The main checkout carries uncommitted live-holdings retry changes to 4 files also touched here (`useActorGraphController.ts`, `actor_tracker.go`, `service_api.go`, `ActorGraphPage.test.tsx`) — merging this branch needs reconciliation

## Files Changed

| File | Change |
|------|--------|
| frontend/src/features/shared/graph-canvas/useGraphCanvasCore.ts | Zoom limits, immediate selection, diff-based topology sync, stale-layout invalidation |
| frontend/src/features/shared/graph-canvas/layout.ts | ELK via web worker w/ fallback, cancellable layout, anchored placement for new nodes |
| frontend/src/features/shared/graph-canvas/useGraphCanvasInteractions.ts | Window-capture drags, space+drag pan, wheel-mode, fullscreen viewport preservation, overlay-target guard |
| frontend/src/features/shared/graph-canvas/useGraphLabelLayer.ts | Persistent label pool, zoom culling, dim-awareness |
| frontend/src/features/shared/graph-canvas/useGraphSearch.ts (new) | Search state, match classes, animated centering |
| frontend/src/features/shared/graph-canvas/useGraphNeighborhoodHighlight.ts (new) | Selection neighborhood dimming |
| frontend/src/features/shared/graph-canvas/useGraphHoverCard.ts (new) | Hover card state + cursor |
| frontend/src/features/shared/graph-canvas/useGraphMinimap.ts (new) | Minimap draw + click/drag pan |
| frontend/src/features/shared/GraphCanvas.tsx / GraphCanvasOverlays.tsx / types.ts | Wiring, search UI, wheel-mode button, hover card, minimap canvas |
| frontend/src/features/shared/GraphTimeScrubber.tsx (new) | Dual-handle time scrubber bound to graph filters |
| frontend/src/lib/graph/presentation.ts | Single edge label + min-zoomed-font-size, dim/highlight/search styles |
| frontend/src/features/actor-graph/* | nodeHasPrimaryAction, scrubber, saved-states sidebar, server save, progress polling |
| frontend/src/features/explorer/ExplorerPage.tsx | Time scrubber |
| frontend/src/lib/api.ts / types.ts | graph-states + progress endpoints/types |
| frontend/src/styles.css | Search/minimap/hover/scrubber/pan-cursor styles, label width fix |
| frontend/src/test-support/cytoscapeMock.ts (new) | Shared capable cytoscape mock |
| frontend/src/features/shared/graph-canvas/__tests__/* | Rewritten + new map-improvements tests (85 total) |
| frontend/vite.config.ts | elk-bundled fallback split from elk-api chunk |
| internal/infra/sqlite/migrations.go | Migration 4: graph_states table |
| internal/app/graph_states.go (new) / build_progress.go (new) + tests | Graph-state store, progress registry |
| internal/app/{app,types,service_api,actor_tracker}.go | Registry wiring, ProgressToken, progress hooks in fetch loops |
| internal/domain/services/{analysis,container}.go | GraphStateService, BuildProgress accessor |
| internal/api/v1.go / dto/types.go + v1_graph_states_test.go (new) | graph-states + progress routes/handlers/DTOs/tests |
| README.md | New endpoint docs |

## In Progress

- Branch `worktree-map-improvements` (this commit) is **not merged to main**; main checkout has uncommitted overlapping live-holdings changes that must be reconciled when landing
- `.gitignore` `server` rule fix (→ `/server` + commit `cmd/server`, `internal/server`) proposed but not applied — user decision pending
- User asked about a `/orchestrator` skill (doesn't exist); clarifying question about running a broader bug-hunt review over the diff was interrupted — no further bug-fix pass was run

## Next Steps

- [ ] Merge `worktree-map-improvements` into main, reconciling the 4 overlapping uncommitted files in the main checkout
- [ ] Fix `.gitignore` (`server` → `/server`) and commit `cmd/server/` + `internal/server/` so fresh clones build
- [ ] Run `/code-review` over the branch diff for an independent bug-hunt before landing
- [ ] Manual hardware QA: wheel-mode Auto heuristic with Logitech MX Master smooth scroll (or just rely on the new Zoom/Pan preference)
- [ ] Consider replacing `window.prompt` in save-graph-state with an inline name field (prompt blocks browser automation and is easy to mis-dismiss)
