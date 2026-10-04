# Session 1 - App Review, Refactor Plan, and Phase 0 Baseline

> Date: 2026-10-04
> Focus: Review the app from first principles, write a phased refactor plan, and complete Phase 0 (clean baseline)

## Summary

Reviewed the uncommitted live-holdings retry change and then the whole app.
Wrote a nine-phase refactor plan aimed at actor/treasury monitoring and
hack/laundering tracing, and completed Phase 0 on branch `refactor/p0-baseline`:
dropped the live-holdings WIP, merged the July graph-canvas overhaul, deleted
the legacy UI and API, and fixed the THOR endpoint defaults. Every previous
default was dead.

## Review Findings

- **Live-holdings WIP (dropped):**
  - 403/ban errors were retried for up to 40 passes, and retryable failures produced no warning, so nodes stayed `pending` forever.
  - The no-progress stop check was removed.
  - The refresh overwrote the build's last-run log.
  - Saved at `../_local/chain-analysis-app/live-holdings-retry-wip.patch`.
  - Phase 3 replaces it with server-side jobs.
- **Whole app:**
  - The caches are keyed by (address, start, end) query windows, not normalised chain data.
  - Every USD value uses the current spot price (`priceBook.usdFor`), and unpriced tokens skip `min_usd`.
  - Labels are a hardcoded map plus 44 annotations.
  - Expansion is by hop count with no amount-following trace, and edge confidence is always 1.
  - Long jobs run as single synchronous requests.
  - There is no case/export output or agent interface.
  - Recent investigations (memoless, operator report) were done outside the app.

## Plan

The plan file is `docs/refactor-plan.md`.

| Phase | Work |
|---|---|
| 0 | Clean baseline |
| 1 | Record/replay transport, golden tests (TC Treasury, Bitget 2026-09-28, rebond, Rujira/CALC), and a mechanical file split |
| 2 | Normalised ledger with coverage intervals |
| 3 | Typed provider errors, circuit breakers, job runner, live holdings on the server |
| 4 | Prices at transaction time |
| 5 | Labels (TagPack format; OFAC, eth-labels, ScamSniffer, user) |
| 6 | Actor monitoring |
| 7 | "Follow this amount" trace |
| 8 | Cases, export, CLI, MCP |

Out of scope: operator attribution, protocol forensics, a self-hosted Midgard.

## Phase 0 Work Done

- Committed the AGENTS.md-only guidance change and the 2026-08-23 session note (`3c081e9`).
- Committed the map-improvements follow-ups in the worktree (`ccff897`):
  - inline save-name popover instead of `window.prompt`
  - time-scrubber drag-end fix
  - search animation fix
  - `.gitignore` `server` → `/server`, plus tracking `cmd/server` and `internal/server/bootstrap`
- Merged that branch (`cbe708e`).
- Removed the legacy UI and API (`a5ea4c0`):
  - `internal/web/static`, `internal/app/http.go`, the static/index routes, and `CHAIN_ANALYSIS_STATIC_DIR`
  - the legacy-only wallet liquidity, wallet bonds, and rebond continuity endpoints, plus the `rebond_links` writers and queries
  - Midgard lookup canonicalisation moved to `midgard_lookup.go`
  - lookup and live-holdings tests ported to the v1 service methods
  - health checks moved to `/api/v1/health`
- Default THORNode and Midgard endpoints now point at the Liquify gateway, and the README was rewritten to match the code (`97c43f8`).

## Discoveries

- **All four previous THOR defaults were unreachable from Go:**
  - `thornode|midgard.thorchain.liquify.com` have expired certificates.
  - `thornode|midgard.thorchain.network` do not resolve.
  - With no `.env` override, every THORNode/Midgard request had been failing.
  - `gateway.liquify.com/chain/thorchain_api` and `.../thorchain_midgard/v2` respond.
  - The MAYA, Vanaheim, and BTC defaults still respond.
  - The Liquify gateway returns 403 for `mayachain_*` paths, so MAYA stays on `mayachain.info`.
- The v1 live-holdings DTO rejects unknown fields, so its nodes take only `id`, `kind`, `chain`, and `metrics`.
- `rebond_links` is now unwritten. The table stays in the base migration.

## Verification

- `go test ./...`, `go vet ./...`, and 88/88 frontend tests pass. The legacy-removal commit builds and tests on its own.
- `make restart-server` reports `build.commit` `97c43f8`.
- `/legacy/`, `/api/health`, and `/static/app.js` return 404, and the UI assets load.
- Live smoke test:
  - `/api/v1/actions/{txid}` resolves a recent swap through Liquify Midgard.
  - Live holdings for a THOR address returns `available` with no warnings through the Liquify THORNode gateway.

## Next Steps

- [x] Moved the untracked repo-root operator reports/CSVs, security-design docs, `system_income_(usd).csv`, and untracked `docs/` analyses to `../_local/chain-analysis-app/`.
- [x] Merged `refactor/p0-baseline` into `main` and pushed.
- [ ] Phase 1: record/replay transport and golden tests, starting with the Bitget hacker address from public reporting.
- [ ] Optional: remove the merged `map-improvements` worktree.
