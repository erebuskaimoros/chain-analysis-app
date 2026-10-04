# Session 2 - Phase 1: Record/Replay Safety Net and File Split

> Date: 2026-10-04
> Focus: Build the regression safety net for the refactor (record/replay transport, golden actor-graph tests) and split the two largest files by concern

## Summary

Phase 1 of `docs/refactor-plan.md` is done on `refactor/p1-safety-net`.
- Every outbound request can now go through a record/replay cassette.
- Three golden actor-graph cases replay offline in about 15 seconds.
- `actor_tracker.go` (5.7k lines) and `external_trackers.go` (3.9k lines) were split into eleven files by concern. The golden graphs are unchanged and the code-line multiset is identical (9,105 lines).
- Recording the Bitget case exposed a projection bug that drops partial-fill swaps.

## Work Done

- **`httpcassette.go`** (`7aa078e`): record/replay `http.RoundTripper`.
  - Requests are identified by method, redacted URL with sorted query, and a request-body hash.
  - Credentials are redacted from query parameters, URL paths (NodeReal), request bodies, and response bodies.
  - The file is gzip JSON.
  - Transport errors are recorded and replayed; cancellations are not.
  - Replay serves a sibling host's recording when failover rotation starts at a different equivalent endpoint.
  - Unit tests cover redaction, POST bodies, misses, recorded errors, and sibling hosts.
- **Config seams.**
  - `Config.HTTPTransport` routes all THORNode, Midgard, and tracker clients.
  - `Config.LiveHoldingsTimeout` optionally replaces the live-holdings batch and per-lookup budgets. Zero keeps the defaults.
- **Golden tests** (`golden_test.go`, `testdata/golden/*`, `59ea285`):

  | Case | Seed | Window | Cassette | Snapshot |
  |---|---|---|---|---|
  | treasury | TC Treasury, 16 addresses | 2026-09-24 → 10-01, hops 2 | 202 requests, ~1 MB | ~1 MB |
  | bitget_exploiter | ETH wallet ending 96c3 | 2026-09-28 03:00–07:00, hops 2 | — | — |
  | rebond | old bond address ending s7sa | 2026-10-02 | — | — |

  - Record with `CHAIN_ANALYSIS_RECORD=1` (API keys come from `.env`). Regenerate snapshots with `-update`.
  - A scan of the cassettes found no real keys.
- **File split** (`3d278af`): new files `midgard_fetch.go`, `pricebook.go`, `live_holdings.go`, `projection.go`, `graph_builder.go`, `midgard_parse.go`, `trackers_holdings.go`, `trackers_transfers.go`, and `trackers_http.go`.
- **Plan updates.**
  - Case B now uses the on-chain figures.
  - The Rujira/CALC golden case was dropped.
  - The treasury window was narrowed to a week.
  - Stale file:line references now point at the new files.

## Discoveries

- **Bitget exploiter identified on-chain.**
  - ETH wallet `0xf7bc92103f23ef312658cd9b81dc2713f7b396c3` made 31 swaps from 03:55 to 06:23 UTC on 2026-09-28: 2,790.32 ETH → 87.82 BTC, all to `bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f`.
  - Press reports said 27 swaps, 2,390 ETH, and 75.2 BTC, and did not publish the address.
  - The match rests on the window, the ~100 ETH batches, and the single destination.
- **Bug: partial-fill swaps are dropped from graphs.**
  - Two of the 31 swaps (05:03:28 and 05:04:05 UTC, 85.78 ETH together) returned unfilled ETH to the sender as well as paying BTC.
  - Their inbound tx IDs match refund actions, so refund suppression removes the whole swap.
  - The graph shows 29 swaps (2,704.54 ETH / 85.14 BTC), so about 2.68 BTC of the exploiter's outflow is missing.
  - Golden B records today's behaviour; the fix should start with a failing test and update golden B deliberately.
- **Vanaheim cost.** The legacy action source (`vanaheimex.com`) is queried for every THOR address whatever the window, at about 1 MB per 50-action page (~20 MB for one busy address-day).
- **Ownership edges carry wall-clock time.** Actor→address edges are stamped with `time.Now()`; golden canonicalisation drops that field.
- **Three `min_usd` filters.** The filter exists in three copies (`projection.go`, `rujira_trace.go`, `trackers_transfers.go`); Phase 4 must change all three.
- **Busy Rujira addresses are bots.** Every Rujira/FIN address active on 2026-09-26 had hundreds of contract actions per day.
- **Midgard paging.** Combining `nextPageToken` with `fromTimestamp`/`timestamp` returns 500. With `fromTimestamp` set, results come back ascending and `prevPageToken` moves forward.

## Verification

- `go vet ./...` and `go test ./...` pass.
- Golden replay passes repeatedly with no misses.
- The split commit builds and passes with byte-identical golden graphs.

## Next Steps

- [ ] Merge `refactor/p1-safety-net` into `main` and push.
- [ ] Fix the partial-fill swap bug test-first (golden B should then show 31 swaps / 87.82 BTC).
- [ ] Phase 2: ledger and coverage intervals. Restrict Vanaheim to windows Midgard cannot serve.
