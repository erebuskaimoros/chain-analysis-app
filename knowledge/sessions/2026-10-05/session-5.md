# Session 5 - Phase 8: Cases, Export, CLI, MCP

> Date: 2026-10-05
> Focus: Refactor plan Phase 8

## Summary

- **Cases** (migration 11: `cases`, `case_items`)
  - Five item kinds: address (stored as `CHAIN|address`), tx (THORChain hash form), trace_run, graph_state and actor.
  - Notes are Markdown.
  - Pinning the same item again updates its note.
  - Trace, graph-state and actor refs must exist.
- **Export:** `GET /api/v1/cases/{id}/export?format=md|csv`.
  - The Markdown report covers:
    - summary
    - items, linked to the explorers
    - for each pinned trace: seeds, window, policy, method, sinks or sources, frontier and flows
    - coverage gaps
    - label sources
  - Swap flows link the deposit on its own chain and the payout on its chain.
  - The CSV has one row per traced transaction.
- **API for the CLI:**
  - `POST /api/v1/jobs/address-profile`: labels, current holdings, and one-hop counterparties and recent transactions.
  - `POST /api/v1/jobs/labels-import`: imports from a path on the server's machine.
  - `GET /api/v1/ledger/coverage`: covered ranges, gaps and deferrals per source.
- **`internal/client`:** a typed `/api/v1` client that runs jobs to completion. `cactl` and `cactl-mcp` both use it.
- **`cmd/cactl`:** JSON output.
  - Commands: `trace`, `actor refresh|summary`, `labels import|lookup`, `case list|show|create|add|export`, `ledger gaps`, `profile`, `tx`, `health`.
  - Build with `make build-cli`.
- **`cmd/cactl-mcp`:** official Go SDK v1.2.0 over stdio.
  - Tools: `trace_funds`, `address_profile`, `actor_summary`, `tx_lookup`, `case_list`, `case_export`. Each returns a compact digest.
  - Registered in `.mcp.json` as `chain-analysis`.
- **UI:**
  - A Cases page: create, notes, pin, remove, export.
  - A "Pin trace to case" control on trace results.

## Verification

- **Tests:**
  - Go unit tests for cases and both exports.
  - API tests for case CRUD and export content types.
  - `cactl` against a real API server over a temp DB.
  - An end-to-end MCP test over in-memory transports: tool list and schemas, case_list and case_export, and tool errors.
  - Frontend tests for the Cases page and pinning a trace.
- **Live on :8091** with a copy of the real database:
  - `cactl trace` for the Bitget exploiter (3 hops) took 31 s and reproduced the case B sinks: j68f holding 31.38 BTC, six more BTC holders, and three hop-limit endpoints.
  - A case with the trace, the exploiter address and the first deposit exported to a 131-line report and a 60-row CSV.
  - A sampled payout link resolves on mempool.space. The linked ETH deposit exists on chain, sent from the exploiter's wallet (ending 96c3).
- **`cactl-mcp` over stdio** answered `tx_lookup` (the 0x-prefixed deposit resolves to the 1 ETH → 0.0316689 BTC swap), `trace_funds` and `case_export`.

## Notes

- MCP SDK v1.6.0 and later require Go 1.25, so v1.2.0 keeps the module at Go 1.23.
- `go mod tidy` picked `golang.org/x/sync` v0.23.0, which raised the go directive to 1.26. It is pinned to v0.16.0, and tidy was run with `-go=1.23.10`.
- The browser check was skipped again: the Claude in Chrome extension is not connected.
