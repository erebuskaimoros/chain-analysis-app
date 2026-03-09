# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-09 #3 | CALC Graph Collapse and Orphan Swap Node Fix | Removed execute-only CALC fallback edges that created an orphan Rujira THORChain Swap node and restored treasury-facing collapse | `sessions/2026-03-09/session-3.md` |
| 2026-03-09 #2 | Graph Run UX and EVM Address Hardening | Converted Past Runs to dropdown replay UX; canonicalized Midgard lookups; fixed strict EVM address inference for graph expansion | `sessions/2026-03-09/session-2.md` |
| 2026-03-09 #1 | Past Graph Runs Persistence | Store graph run params in SQLite; Past Runs UI card for replaying builds; fixed scroll-zoom regression | `sessions/2026-03-09/session-1.md` |
| 2026-03-08 #2 | External Tracker Resilience and RPC Batch | Fixed graph frontier loss on tracker failure; batched Solana/NodeReal RPC calls; Midgard timeout 5→10s | `sessions/2026-03-08/session-2.md` |
| 2026-03-08 #1 | Remove THORNode, Midgard-Only | Major refactor removing all THORNode dependencies; Midgard is now sole data source (-2300 lines) | `sessions/2026-03-08/session-1.md` |

## Current Work In Progress

- Validate the updated treasury graph in the browser to confirm the live Cytoscape rendering matches the cleaned API replay
- Validate the Past Runs dropdown load/delete flow end-to-end in the browser after the latest UI changes
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
- Consider re-adding node bond metrics via alternative source
