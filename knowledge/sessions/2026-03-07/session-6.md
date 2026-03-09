# Session 6 - Hop Logic Fixes, Node Dedup, and Address Labels

> Date: 2026-03-07
> Focus: Fix exponential frontier blowup in hop expansion, eliminate disconnected graph components, add known address labels

## Summary

Fixed three critical issues in the actor tracker graph builder: (1) unbounded frontier expansion causing 347+ nodes for 2 hops, (2) disconnected graph components caused by depth-dependent node IDs creating duplicate nodes for the same address, and (3) the Reserve address polluting graphs. Also added a known address labeling system for protocol modules and notable addresses.

## Work Done

- Added `maxFrontierPerHop = 20` constant to cap frontier expansion per hop level
- Refactored BFS loop to collect next-hop candidates with cumulative USD flow, sort by USD descending, and cap — prioritizing highest-value connections
- Added `frontierCandidate` type to support USD-prioritized frontier selection
- Added `graphExcludedAddresses` map to completely exclude addresses from graphs (Reserve: `thor1dheycdevq39qlkxs2a6wuuzyn4aqxhve4qxtxt`)
- Fixed `ensureNode` to deduplicate by `Key` (normalized address) instead of `ID` (depth-dependent) — eliminates duplicate nodes and disconnected components
- Added `knownAddressLabels` map for labeling well-known addresses without blocking expansion
- Labels added: Synth Module, Arb Bots (x2), Asgard Module, Rujira/TCY contract addresses

## Discoveries

- Node IDs included depth (e.g., `external_address:ADDR:external:1` vs `:4`), causing the same address at different hop depths to create separate unconnected nodes — root cause of disconnected graph components
- The "THOR External Cluster" is a client-side visual grouping of low-value, single-connection external addresses (controlled by `lowSignalExternal` in app.js)
- `frontierBlacklist` prevents hop expansion but still shows the node; `graphExcludedAddresses` drops the address entirely; `knownAddressLabels` only provides display labels
- Without frontier caps, a single seed over 5 months could produce 347 nodes / 354 edges through exponential counterparty discovery

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Added `maxFrontierPerHop`, `frontierCandidate` type, `graphExcludedAddresses`, `knownAddressLabels`; refactored BFS to USD-prioritized capped frontier; fixed `ensureNode` Key-based dedup; added Reserve exclusion check in `makeAddressRef` |

## In Progress

None - session complete

## Next Steps

- [ ] Re-run graph with 2 hops to verify frontier cap and node dedup fix visually
- [ ] Consider adding more module addresses to `graphExcludedAddresses` (e.g., Pool Module, Fee Module) if they create noise
- [ ] Tune `maxFrontierPerHop` value (currently 20) based on real-world graph results
- [ ] Consider making frontier cap configurable via the UI query parameters
- [ ] Add logging for frontier cap events (how many candidates were trimmed per hop)
