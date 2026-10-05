# Session Log Index

## Recent Sessions

| Date | Focus | Summary | File |
|------|-------|---------|------|
| 2026-10-05 #6 | Refactor Plan Wrap-Up | All nine refactor phases merged and pushed (main f5d92da); live cactl/MCP checks; papercut review (3 resolved, typecheck script); shared wiki rewritten (uncommitted in workspace repo) | `sessions/2026-10-05/session-6.md` |
| 2026-10-05 #5 | Phase 8: Cases, Export, CLI, MCP | Cases with Markdown/CSV export; address profile, labels import and ledger coverage APIs; shared client; cactl CLI; cactl-mcp server (Go SDK v1.2.0) in .mcp.json; Cases page and pin-to-case | `sessions/2026-10-05/session-5.md` |
| 2026-10-05 #4 | Phase 7: Follow-the-Funds Trace | Fixed rotated-vault/0x stitching double counts; explained confidence; forward/backward trace engine with fifo/haircut/largest_out; trace runs, API and Trace page; Bitget acceptance (31 swaps, 87.823 BTC) | `sessions/2026-10-05/session-4.md` |
| 2026-10-05 #3 | Phase 6: Actor Monitoring | Actor snapshots and watch state; refresh job reusing the ledger tail plus the holdings job; since-last-look changes by asset, address and counterparty; scheduler; Monitor panel with holdings chart | `sessions/2026-10-05/session-3.md` |
| 2026-10-05 #2 | Phase 5: Address Labels | Labels table with TagPack confidence; built-in TagPack; GraphSense, OFAC, eth-labels, ScamSniffer importers; node label categories; UI badges and category filter | `sessions/2026-10-05/session-2.md` |

## Current Work In Progress

- Browser QA of the Monitor panel, Trace page and Cases page (only component-tested; Claude in Chrome was not connected)
- Approve and try the `chain-analysis` MCP server (`.mcp.json`, needs the server on :8090)
- Delete or rotate: old `data/logs` files may contain the Etherscan API key from before log redaction
- Set chain hints on TC Treasury SOL (…LK5g) and BCH (…skzw) addresses so live holdings resolve
- Commit the shared wiki edits (`../knowledge/projects/chain-analysis-app.md`, `../knowledge/log.md`) in the workspace repo
- Run `/code-review` over the merged map-improvements changes (landed after tests only, without a separate review pass)
- Investigate Treasury BTC address `bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs` missing actor-colored rim; confirm whether it should be added to `TC Treasury`
- Manual QA: wheel-mode Auto heuristic with real hardware (Logitech MX Master smooth scroll) — or rely on the new explicit Zoom/Pan wheel-mode preference
