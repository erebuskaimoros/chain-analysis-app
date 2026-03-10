# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-10 #8 | React Graph Refactor and Legacy RUJI Valuation | Continued the React graph refactor, refreshed the built UI bundle, and fixed legacy `x/ruji` THOR holdings so RUJI picks up Midgard USD pricing | `sessions/2026-03-10/session-8.md` |
| 2026-03-10 #7 | Directional Edge Collapse and Txn Value Filters | Added per-transaction USD min/max graph filters and collapsed visible flow rendering to one edge per direction between nodes. | `sessions/2026-03-10/session-7.md` |
| 2026-03-10 #6 | Granular Graph Filters and Edge Transactions | Added shared graph filters on both tabs and extended edges with per-transaction data so time windows trim transactions inside aggregated edges. | `sessions/2026-03-10/session-6.md` |
| 2026-03-10 #5 | Explorer Annotation Refresh Scope Fix | Fixed the explorer right-click label regression by moving annotation refresh into shared frontend scope for both graph tabs | `sessions/2026-03-10/session-5.md` |
| 2026-03-10 #4 | THOR Rebond Graph Accuracy | Fixed THOR rebond wallet projection, made all-time Midgard action lookups prefer Ninerealms, and corrected explorer rebond edge/render refresh behavior | `sessions/2026-03-10/session-4.md` |

## Current Work In Progress

- **React graph/browser validation (IN PROGRESS)**: A manual browser pass is still needed on Actor Tracker and Address Explorer to validate the shared React GraphCanvas and GraphFilterPopover behavior, ELK layout, saved-run reloads, expansion flows, and parity with the refreshed built bundle.
- Monitor Midgard backend divergence on all-time THOR `/actions` lookups; explorer now prefers Ninerealms first for no-time-range requests because Liquify was returning incomplete rebond history
- Validate whether any additional THOR module denoms besides `x/ruji` need to normalize into `THOR.*` assets for accurate legacy valuation
- Add app-side heuristics for likely spam/scam EVM transfers since Etherscan API responses do not expose a documented spam filter
- Monitor ETH/Solana external tracker truncation and rate-limit behavior on larger expansions
