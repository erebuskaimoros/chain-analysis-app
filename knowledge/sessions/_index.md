# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-10 #3 | Restore Wheel Zoom with Box Selection | Removed wheel suppression and explicitly re-enabled zoom while preserving left-drag selection and middle-button panning | `sessions/2026-03-10/session-3.md` |
| 2026-03-10 #2 | Graph Interaction Controls and Actor Badge Tuning | Added wheel zoom, empty-space box selection, node dragging preservation, and actor badge visual tuning | `sessions/2026-03-10/session-2.md` |
| 2026-03-10 #1 | Address Explorer Completion | Finished preview-first explorer flow, chain-scoped EVM targets, past-run history, migration fix, and verified restart | `sessions/2026-03-10/session-1.md` |
| 2026-03-09 #4 | Address Explorer Tab (In Progress) | New tab for single-address tx graphing; backend + frontend implemented; scoping bug fixed; needs chain awareness and e2e testing | `sessions/2026-03-09/session-4.md` |
| 2026-03-09 #3 | CALC Graph Collapse and Orphan Swap Node Fix | Removed execute-only CALC fallback edges that created an orphan Rujira THORChain Swap node and restored treasury-facing collapse | `sessions/2026-03-09/session-3.md` |

## Current Work In Progress

- **Graph interaction/browser validation (IN PROGRESS)**: Wheel zoom was restored after removing container wheel suppression, while box selection and middle-pan were preserved. A final browser pass is still needed to verify gesture feel and page-scroll behavior.
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
- Consider re-adding node bond metrics via alternative source
