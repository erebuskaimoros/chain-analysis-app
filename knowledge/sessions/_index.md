# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-10 #2 | Graph Interaction Controls and Actor Badge Tuning | Added wheel zoom, empty-space box selection, node dragging preservation, and actor badge visual tuning | `sessions/2026-03-10/session-2.md` |
| 2026-03-10 #1 | Address Explorer Completion | Finished preview-first explorer flow, chain-scoped EVM targets, past-run history, migration fix, and verified restart | `sessions/2026-03-10/session-1.md` |
| 2026-03-09 #4 | Address Explorer Tab (In Progress) | New tab for single-address tx graphing; backend + frontend implemented; scoping bug fixed; needs chain awareness and e2e testing | `sessions/2026-03-09/session-4.md` |
| 2026-03-09 #3 | CALC Graph Collapse and Orphan Swap Node Fix | Removed execute-only CALC fallback edges that created an orphan Rujira THORChain Swap node and restored treasury-facing collapse | `sessions/2026-03-09/session-3.md` |
| 2026-03-09 #2 | Graph Run UX and EVM Address Hardening | Converted Past Runs to dropdown replay UX; canonicalized Midgard lookups; fixed strict EVM address inference for graph expansion | `sessions/2026-03-09/session-2.md` |

## Current Work In Progress

- **Graph interaction/browser validation (IN PROGRESS)**: Wheel zoom, empty-space box selection, node dragging, and actor badge sizing are implemented, but the final feel and visual balance still need a manual browser pass. If actor badges still look inverted, replace the full-color logo treatment with a simpler badge.
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
- Consider re-adding node bond metrics via alternative source
