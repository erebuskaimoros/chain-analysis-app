# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-10 #7 | Directional Edge Collapse and Txn Value Filters | Added per-transaction USD min/max graph filters and collapsed visible flow rendering to one edge per direction between nodes. | `sessions/2026-03-10/session-7.md` |
| 2026-03-10 #6 | Granular Graph Filters and Edge Transactions | Added shared graph filters on both tabs and extended edges with per-transaction data so time windows trim transactions inside aggregated edges. | `sessions/2026-03-10/session-6.md` |
| 2026-03-10 #5 | Explorer Annotation Refresh Scope Fix | Fixed the explorer right-click label regression by moving annotation refresh into shared frontend scope for both graph tabs | `sessions/2026-03-10/session-5.md` |
| 2026-03-10 #4 | THOR Rebond Graph Accuracy | Fixed THOR rebond wallet projection, made all-time Midgard action lookups prefer Ninerealms, and corrected explorer rebond edge/render refresh behavior | `sessions/2026-03-10/session-4.md` |
| 2026-03-10 #3 | Restore Wheel Zoom with Box Selection | Removed wheel suppression and explicitly re-enabled zoom while preserving left-drag selection and middle-button panning | `sessions/2026-03-10/session-3.md` |

## Current Work In Progress

- **Graph filter/browser validation (IN PROGRESS)**: A manual browser pass is still needed on Actor Tracker and Address Explorer to validate per-transaction time and USD filtering, one-edge-per-direction rendering, and graph/action/stat synchronization under expansion and load-more flows.
- Monitor Midgard backend divergence on all-time THOR `/actions` lookups; explorer now prefers Ninerealms first for no-time-range requests because Liquify was returning incomplete rebond history
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
- Consider re-adding node bond metrics via alternative source
