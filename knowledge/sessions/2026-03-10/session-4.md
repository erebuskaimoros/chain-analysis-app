# Session 4 - THOR Rebond Graph Accuracy

> Date: 2026-03-10
> Focus: Fix rebond graph accuracy, Midgard source selection, and explorer label refresh behavior for THOR bond wallets

## Summary

Corrected the THOR rebond graph path so rebonds project from old bond wallet to new bond wallet with validator metadata preserved, while all-time explorer lookups now prefer Ninerealms to avoid Liquify's incomplete rebond history. Also fixed explorer UI refresh issues so right-click labeling updates immediately and rebond edges no longer collapse distinct validators into a single displayed edge.

## Work Done

- Traced `thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu` through explorer, wallet bonds, rebond continuity, and actor-tracker expansion paths
- Verified a live Midgard divergence: Ninerealms returned 17 all-time actions for `pumu` including 9 rebonds, while Liquify returned only 8 with no rebonds
- Changed bond projection so native `rebond` actions map old bond wallet to new bond wallet and attach validator metadata from documented Midgard rebond fields
- Added regression coverage for rebond wallet-to-wallet projection
- Updated `ThorClient` endpoint ordering so no-time-range `/actions` lookups stay primary-first while bounded queries keep the existing rotation
- Added tests covering all-time `/actions`, bounded `/actions`, and txid lookups for endpoint-order behavior
- Fixed explorer right-click label / Asgard / remove actions to refresh annotations before rerendering
- Fixed both visible-graph reducers so rebond edges remain distinct per `validator_address` instead of collapsing validator context together
- Ran `go test ./...`, restarted with `make restart-server`, and verified `/api/health` plus live explorer replays

## Discoveries

- `https://midgard.thorchain.liquify.com/v2/actions` currently omits rebond history for all-time THOR lookups (`fromTimestamp=0`) that `https://midgard.ninerealms.com/v2/actions` does return.
- The backend rebond payloads were correct after the Midgard-source fix; a remaining frontend edge-collapse key was still able to smear validator labeling across distinct rebond edges.
- Explorer annotation writes need a post-write refresh of `/api/address-annotations` and `/api/address-blocklist`; rerendering from stale client state makes label changes appear to fail.

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Projected rebond actions from old bond wallet to new bond wallet and resolved validator metadata from Midgard rebond fields |
| `internal/app/actor_tracker_test.go` | Added rebond projection regression coverage |
| `internal/app/thor_client.go` | Preferred the primary Midgard endpoint for no-time-range `/actions` lookups while preserving rotation for bounded queries |
| `internal/app/thor_client_test.go` | Added endpoint-order tests for all-time, bounded, and txid-based `/actions` lookups |
| `internal/web/static/app.js` | Refreshed explorer annotations after right-click edits and kept rebond edges distinct per validator in both graph renderers |

## In Progress

None - session complete

## Next Steps

- [ ] Run one manual browser pass on Address Explorer and Actor Tracker to confirm the refreshed frontend bundle reflects the rebond-edge separation and immediate label updates
- [ ] Monitor Midgard backend divergence for all-time THOR action lookups and pin additional lookup classes to Ninerealms first if Liquify continues returning incomplete histories
- [ ] Consider adding a small integration test fixture for explorer graph rendering with multiple rebond validators so frontend edge-collapsing regressions are caught earlier
