# Session Index

## Recent Sessions

| Session | Focus | Summary | File |
|---------|-------|---------|------|
| 2026-03-12 #1 | Graph Canvas Persistence and Actor Affiliate Fee Suppression | Added saved node-position restore, tightened graph interactions and labels, and prevented actor-graph swaps from leaking in through affiliate-fee-only matches | `sessions/2026-03-12/session-1.md` |
| 2026-03-11 #7 | Saved Graph State Loading | Added JSON graph-state import for Actor Graph and Explorer, restored saved UI/filter state, and verified the flow with page tests and a rebuilt bundle | `sessions/2026-03-11/session-7.md` |
| 2026-03-11 #6 | Annotation Editing from the Metadata Tab | Added inline editing for saved address labels from the Annotations tab and verified the flow with a dedicated page test and rebuilt bundle | `sessions/2026-03-11/session-6.md` |
| 2026-03-11 #5 | Unified THOR + MAYA Liquidity Flow Parsing Wrap-Up | Logged the completed unified liquidity-engine delivery, confirmed full verification, and left the repo aligned with origin without committing unrelated local artifacts | `sessions/2026-03-11/session-5.md` |
| 2026-03-11 #4 | Address Live-Value Prioritization and Protocol Provenance Surfacing | Fixed address-node live-value starvation under the bounded lookup budget and captured the broader multi-protocol provenance work still active in the tree | `sessions/2026-03-11/session-4.md` |

## Current Work In Progress

- **Merged THOR + MAYA browser validation (IN PROGRESS)**: Run a large live graph in the browser and confirm merged provenance remains clear under real public tracker latency and partial failures.
- Validate actor-graph affiliate fee suppression against a live treasury run with known affiliate-heavy swaps.
- Decide whether `Saved Graphs/` should be gitignored or otherwise treated as expected local-only output.
- Decide whether to keep shipping refreshed `internal/web/ui/dist` assets in-session or move that bundle workflow elsewhere as chunk size keeps growing.
