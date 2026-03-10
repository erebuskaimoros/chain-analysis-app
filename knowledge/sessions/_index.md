# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-09 #4 | Address Explorer Tab (In Progress) | New tab for single-address tx graphing; backend + frontend implemented; scoping bug fixed; needs chain awareness and e2e testing | `sessions/2026-03-09/session-4.md` |
| 2026-03-09 #3 | CALC Graph Collapse and Orphan Swap Node Fix | Removed execute-only CALC fallback edges that created an orphan Rujira THORChain Swap node and restored treasury-facing collapse | `sessions/2026-03-09/session-3.md` |
| 2026-03-09 #2 | Graph Run UX and EVM Address Hardening | Converted Past Runs to dropdown replay UX; canonicalized Midgard lookups; fixed strict EVM address inference for graph expansion | `sessions/2026-03-09/session-2.md` |
| 2026-03-09 #1 | Past Graph Runs Persistence | Store graph run params in SQLite; Past Runs UI card for replaying builds; fixed scroll-zoom regression | `sessions/2026-03-09/session-1.md` |
| 2026-03-08 #2 | External Tracker Resilience and RPC Batch | Fixed graph frontier loss on tracker failure; batched Solana/NodeReal RPC calls; Midgard timeout 5→10s | `sessions/2026-03-08/session-2.md` |

## Current Work In Progress

- **Address Explorer tab (IN PROGRESS)**: Backend + frontend implemented, `decorateVisibleNode` scoping bug fixed. Needs: chain awareness for EVM addresses, end-to-end graph rendering test, direction chooser/pagination test, context menu test. Plan file: `.claude/plans/woolly-stargazing-ocean.md`
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
- Consider re-adding node bond metrics via alternative source
