# Session 6 - Refactor Plan Wrap-Up

> Date: 2026-10-05
> Focus: Close out the nine-phase refactor (Phases 7–8 landed this session) and review the session's papercuts

## Summary

The refactor plan in `docs/refactor-plan.md` is complete. All nine phases (0–8) are merged to `main` and pushed. The running server reports commit `f5d92da`.

This conversation, from 2026-10-04 to 2026-10-05, took the prototype to a usable investigation tool:
- a ledger of fetched history
- background jobs
- prices at transaction time
- address labels
- actor monitoring
- follow-the-funds traces
- cases with export
- a CLI and an MCP server

The per-phase details are in sessions 2026-10-04 #1–#4 and 2026-10-05 #1–#5.

## Work Done

- Phases 7 (trace) and 8 (cases, export, CLI, MCP) were finished, merged and pushed. See sessions #4 and #5.
- A live, uncached `cactl trace` of the Bitget exploiter took 31 s. The MCP server was checked over stdio, launched exactly as `.mcp.json` does.
- Bounded papercut review of this repo:
  - **Resolved:** the flaky ExplorerPage fullscreen test (fixed in `1080da1`), and `go mod tidy` raising the go directive (`x/sync` is now pinned).
  - **Fixed:** worktree typechecks no longer depend on `npx tsc`. A new script, `npm --prefix frontend run typecheck`, uses the project's own `tsc`.
  - **Left open:** two zsh-quoting papercuts. They concern the global shell guidance, not this repo.
- The shared wiki page (`../knowledge/projects/chain-analysis-app.md`) was rewritten for the finished refactor, and an entry was added to `../knowledge/log.md`. Both are uncommitted in the workspace repo, which holds other people's uncommitted changes.
- Removed the scratch database copies from smoke testing (about 2.9 GB).

## Discoveries

- **Swaps through rotated vaults were drawn twice.** EVM trackers report `0x`-prefixed hashes, and THORChain rotates vaults out of `inbound_addresses`. As a result, swap deposits and payouts through old vaults appeared both inside the swap and as plain transfers. Stitching now matches deposits by sender and payouts by recipient, using transaction and coin. *(Evergreen; already in the shared wiki.)*
- **The Bitget exploiter case differs from early reporting.** On-chain there are 31 swaps carrying 87.823 BTC to the address ending j68f, not 27 swaps and 75.2 BTC. The exploiter also sent 119.34 ETH to a wallet ending 164c, which later swapped onward into BTC. *(Already in the wiki.)*
- **The MCP Go SDK is pinned at v1.2.0.** v1.6.0 and later need Go 1.25, and this module targets Go 1.23.10. `go mod tidy` can raise the go directive by picking new `x/*` versions, so pin them and use `-go=`. *(In the README and plan.)*
- **Expanded addresses must consume lots before taint arrives.** In the trace simulation, an expanded address has to consume its lots for every outflow, including ones before traced funds arrive. Otherwise FIFO state drifts.
- **Endpoint lists must serialise as `[]`.** Empty Go slices serialise as `null`, which would crash UI code that reads `.length`. Trace results now normalise every list to `[]`.

## Files Changed

| File | Change |
|------|--------|
| internal/app/trace*.go, trace_store.go | Trace engine, results and saved runs (Phase 7) |
| internal/app/projection.go, midgard_parse.go, graph_builder.go | Rotated-vault and `0x` stitching fix; confidence with reasons; swap deposit hashes |
| internal/app/cases.go, case_export.go, address_profile.go | Cases, Markdown/CSV export, address profile, label-import job and ledger coverage (Phase 8) |
| internal/client/, cmd/cactl/, cmd/cactl-mcp/, .mcp.json | Shared API client, CLI and MCP server |
| internal/api/v1_trace.go, v1_cases.go, internal/domain/services/*.go | API routes and services |
| internal/infra/sqlite/migrations.go | Migrations 10 (`trace_runs`) and 11 (`cases`, `case_items`) |
| frontend/src/features/trace/, features/cases/, App.tsx | Trace page, Cases page, pin-to-case; removed the dead legacy link |
| frontend/package.json | `typecheck` script |
| README.md, docs/refactor-plan.md, knowledge/sessions/ | Documentation and phase status |

## In Progress

None. The refactor is complete. Follow-ups are listed below.

## Next Steps

- [ ] Browser-QA the new UI (Monitor panel, Trace page, Cases page) once the Claude in Chrome extension is connected. So far it is covered only by component tests.
- [ ] Approve the `chain-analysis` MCP server in Claude Code and try a real investigation through it.
- [ ] Delete old `data/logs` files that may hold the Etherscan API key from before redaction, or rotate the key.
- [ ] Set chain hints on the TC Treasury SOL (ending LK5g) and BCH (ending skzw) addresses so their holdings resolve.
- [ ] Commit the shared wiki edits in the workspace repo once its other uncommitted changes are settled.
