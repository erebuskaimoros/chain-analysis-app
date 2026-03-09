# Session 1 - Past Graph Runs Persistence

> Date: 2026-03-09
> Focus: Store graph run request params and add Past Runs UI card for replaying previous builds

## Summary

Added a `graph_runs` SQLite table that stores the request parameters of each successful graph build. A new "Past Runs" card in the actor-tracker UI lists previous runs with metadata (actor names, time range, node/edge counts) and lets users reload any past run by replaying its request against cached Midgard data. Also fixed a regression where `wheelSensitivity: 0.15` was accidentally removed from the Cytoscape config, breaking scroll-to-zoom.

## Work Done

- Fixed scroll-to-zoom regression by restoring `wheelSensitivity: 0.15` in Cytoscape init
- Added `graph_runs` table to SQLite schema in `store.go`
- Added `GraphRun` type to `types.go`
- Added `insertGraphRun`, `listGraphRuns`, `deleteGraphRun` store functions
- Modified `handleActorTrackerGraph` to save run params after each successful build
- Added `GET /api/actor-tracker/runs` and `DELETE /api/actor-tracker/runs/{id}` endpoints
- Added "Past Runs" card to `index.html` with load/delete buttons
- Added `refreshGraphRuns()`, `renderGraphRuns()`, and click handlers in `app.js`
- Backfilled last graph run (TC Treasury, 56N/73E) from server logs into the new table

## Discoveries

- Midgard action data IS already cached in `midgard_action_cache` SQLite table — `fetchMidgardActionsForAddress` checks disk cache first via `lookupMidgardActionCache()` before hitting the API
- Graph rebuilds from cached data are fast since the expensive Midgard API calls are skipped on cache hit
- The `wheelSensitivity` removal was an accidental side effect of a prior diff — Cytoscape defaults to 1.0 which behaves erratically

## Files Changed

| File | Change |
|------|--------|
| `internal/app/store.go` | Added `graph_runs` table schema, `insertGraphRun`, `listGraphRuns`, `deleteGraphRun` |
| `internal/app/types.go` | Added `GraphRun` struct |
| `internal/app/http.go` | Save run after build, added `handleGraphRuns` + `handleGraphRunDelete`, registered routes |
| `internal/web/static/index.html` | Added Past Runs card HTML |
| `internal/web/static/app.js` | Added `state.graphRuns`, refresh/render/load/delete logic, restored `wheelSensitivity`, added `refreshGraphRuns` on init and after builds |

## In Progress

None - session complete

## Next Steps

- [ ] Restart server and verify Past Runs card renders the backfilled TC Treasury run
- [ ] Click "Load" on the past run and confirm graph rebuilds correctly from cached data
- [ ] Run a new graph build and verify it auto-appears in Past Runs
- [ ] Test delete functionality on past runs
- [ ] Consider adding a "label" field to graph_runs for user-provided run names
