# chain-analysis-app: refactor and improvement plan

## Context

The app reads THORChain correctly: it merges each swap's inbound and outbound legs, models LP custody, and follows bond and rebond chains. The prototype around that core has five structural gaps:

1. **The cache stores query results, not chain data.** `midgard_action_cache` and `external_transfer_cache` hold JSON blobs keyed by (address, start, end). Any new or shifted time window refetches everything, and nothing can be queried across graphs.
2. **Every USD value uses today's price.** `priceBook.usdFor` (`internal/app/pricebook.go`) applies current pool prices to historical transfers. Unpriced tokens skip the min-USD filter (`projection.go`, `rujira_trace.go`, `trackers_transfers.go`).
3. **Labels are a hardcoded map** (`knownAddressLabels` in `actor_tracker.go`) plus 44 manual annotations. There are no exchange, sanctions or scam labels, which is where most traces end.
4. **Long jobs run as single synchronous HTTP requests.** The browser runs the live-holdings retry loop against an endpoint that keeps no state. The default THORNode endpoint returns 403 from this host.
5. **There is no answer-shaped output.** Expansion is by hop count, with no "follow this amount" trace, no historical monitoring of actors, and no case or export. Recent investigations were done beside the app, not in it.

Scope comes from your answers:
- Optimise for **actor/treasury monitoring** and **hack/laundering tracing**.
- Use public endpoints only; no self-hosted Midgard.
- Land the map-improvements branch and drop main's live-holdings WIP.
- Delete the legacy UI and legacy API.

**Acceptance cases used throughout:**
- **(A) TC Treasury monitoring.** Repeat refreshes fetch only new data, and holdings form a time series.
- **(B) Bitget hack, 2026-09-28.** From the exploiter's ETH address (ending 96c3), the app finds all 31 THORChain swaps between 03:55 and 06:23 UTC (2,790.32 ETH in, 87.82 BTC out to one BTC address ending j68f). Every edge carries its transaction IDs. Press reports said 27 swaps, 2,390 ETH and 75.2 BTC; Midgard shows the larger figures.

## Execution conventions

- One branch per phase off `main`, named `refactor/pN-<slug>`. Merge after that phase's exit criteria pass. Each phase is sized for one `/overseer` run if wanted.
- Bug fixes start with a failing test (your global rule).
- Before any commit or push, run `../scripts/audit-workspace.sh --project chain-analysis-app`. The manifest says GitHub, `direct` push to `origin`, reviewed with `gh`.
- After each phase:
  - Run `make restart-server` and check that `/api/v1/health` shows the expected `build.commit` and `build.build_time`.
  - Add a session note under `knowledge/sessions/`.
  - Update `../knowledge/projects/chain-analysis-app.md` and the README, both of which are currently stale.
- SQLite migrations go in `internal/infra/sqlite/migrations.go` (`migrations` slice, `{id, name, up}`). The map branch takes id 4 (`graph_states`). New ids run sequentially from 5.
- New code stays in package `app` (separate files) so it can reach unexported types. Splitting into separate packages is deferred until the seams are stable.

---

## Phase 0: Clean baseline

1. **Drop main's WIP safely.**
   - Save `git diff` of the live-holdings files to `../_local/chain-analysis-app/live-holdings-retry-wip.patch`.
   - Restore these files with `git restore`:
     - `useActorGraphController.ts`
     - `ActorGraphPage.test.tsx`
     - `actor_tracker.go`
     - `external_trackers_test.go`
     - `service_api.go`
     - `internal/web/ui/dist/*`
   - Keep the unrelated `AGENTS.md`/`CLAUDE.md` guidance edits and `knowledge/sessions/2026-08-23/`, and commit them separately as docs.
2. **Land `worktree-map-improvements`** (worktree at `../.worktrees/chain-analysis-app/map-improvements`; commit `b44273a` plus uncommitted edits).
   - Run `go test ./...` and `npm --prefix frontend test` in the worktree, then commit its pending edits. These include the `.gitignore` fix `server` → `/server`.
   - Commit `cmd/server/` and `internal/server/`, which the old rule hid from git.
   - Merge into `main`. Rebuild `internal/web/ui/dist` rather than hand-resolving hashed asset conflicts.
3. **Delete legacy.**
   - Remove `internal/web/static/`.
   - Remove `registerLegacyUI`, `RegisterLegacyStaticRoutes` and `RegisterLegacyAPIRoutes`, plus their wiring in `internal/server/bootstrap/server.go`.
   - Remove the legacy handlers in `internal/app/http.go` and their tests in `http_test.go`. Move the request-logging middleware that `internal/api/v1.go` uses (`WithRequestLoggingFunc`) into `observability.go`.
   - Remove the `CHAIN_ANALYSIS_STATIC_DIR` config and the unused `schemaSQL` const in `store.go`.
   - Point `scripts/restart-server.sh` `HEALTH_URL` and the restart note in `AGENTS.md` at `/api/v1/health`. Rewrite the README API section.
4. **Fix the endpoint defaults** in `internal/app/config.go`.
   - THORNode: Liquify first (`https://gateway.liquify.com/chain/thorchain_api`, then `https://thornode.thorchain.liquify.com`).
   - Midgard: the Liquify gateway (`https://gateway.liquify.com/chain/thorchain_midgard` plus whatever `/v2` suffix a curl check shows is needed).
   - Remove the `*.thorchain.network` hosts, which return 403 from this machine.
5. **Move out-of-scope files** out of the repo root into `../_local/chain-analysis-app/`, confirming each file: operator CSVs and reports, security-design docs, `system_income_(usd).csv`. Add `Saved Graphs/` to `.gitignore`.

**Exit:** a clean `git status` on `main`, all tests green, the UI works from `/`, `/legacy` returns 404, and health is reported at `/api/v1/health`.

**Status (2026-10-04): done and pushed.** All four previous THOR endpoint defaults turned out to be unreachable (expired certificates or DNS failures). The legacy-only wallet liquidity, wallet bonds and rebond continuity endpoints were removed along with the legacy API.

## Phase 1: Safety net and file split

1. **Record/replay HTTP transport** (`internal/app/httpcassette.go`).
   - An `http.RoundTripper` with a record mode, which proxies and writes `testdata/cassettes/<case>/<hash>.json`, and a replay mode, which serves by method+URL and fails on a miss.
   - Redact `apikey`, `api_key` and `key` query parameters and auth headers before hashing and writing.
   - Inject it through a new `Config.HTTPTransport`, used by `App.httpClient` (`app.go`) and `NewThorClient` (`thor_client.go`).
2. **Golden tests** (`internal/app/golden_test.go`). Record live with `CHAIN_ANALYSIS_RECORD=1`; replay by default; regenerate with `-update`.
   - (A) TC Treasury actor over 2026-09-24 → 10-01, max_hops 2. The window is a week rather than 30 days, to keep the cassette near 1 MB. Actor fixtures are created in a temp DB.
   - (B) The exploiter's ETH address as an actor graph over 2026-09-28 03:00–07:00 UTC. The address was identified on-chain from the reported window, batch size and single destination; press reports didn't publish it.
   - (C) A rebond case (old bond address ending s7sa → new bond address ending 8x2l, 2026-10-02).
   - A dedicated Rujira/CALC case was dropped: active Rujira addresses are trading bots with hundreds of actions a day. Unit tests already cover the Rujira/CALC rules, and case A covers CALC payouts to the treasury.
   - Golden output is canonical JSON in `testdata/golden/<case>.json`: nodes and edges sorted, with timestamps, `requested_at` and live metrics removed.
3. **Split the big files without changing behaviour** (same package; golden and unit tests must stay identical):
   - `actor_tracker.go` (5.7k lines) → `midgard_fetch.go`, `live_holdings.go`, `projection.go` (projection, stitching, contract/CALC helpers), `graph_builder.go`, `pricebook.go`, `midgard_parse.go`.
   - `external_trackers.go` (3.9k lines) → `trackers_holdings.go`, `trackers_transfers.go`, `trackers_http.go`.

**Exit:** golden cases A–C pass in replay mode, all existing tests pass, and the split diff only moves code.

**Status (2026-10-04): done.**
- Replay runs offline in about 15 seconds and is stable across runs.
- Two test seams were needed:
  - `Config.LiveHoldingsTimeout`, because live-holdings budgets inside the build otherwise race between record and replay.
  - Sibling-host fallback in replay, because failover endpoints rotate per request.
- Findings to carry forward:
  - **Bug: partial-fill swaps are dropped.** A swap that pays the destination and also returns unfilled input to the sender shares its inbound tx ID with a refund action. The refund-suppression rule then removes the whole swap. Case B shows 29 of 31 swaps (2,704.54 ETH / 85.14 BTC); the two partial fills (85.78 ETH) are missing. Fix this test-first, then update golden B deliberately.
  - **Cost: the legacy action source (Vanaheim) is queried for every THOR address, whatever the window.** It returned about 20 MB for one busy address-day. Phase 2 should query it only for windows Midgard can't serve.

## Phase 2: Ledger instead of query-shaped caches

**Status (2026-10-04): done.** Old caches are backfilled and dropped at startup (instead of a separate migration 6). Truncated ranges are deferred for 24h instead of refetched every build. Vanaheim is opt-in. Real-data validation is in session 2026-10-04 #3.

Migration 5 `ledger`:
- `ledger_thor_actions(action_key PK, protocol, type, status, height, block_time, raw_json)`, where `action_key` comes from `midgardActionKey` (`projection.go`).
- `ledger_thor_action_addresses(address, block_time, action_key, PK(address, action_key))`.
- `ledger_transfers(chain, transfer_key, tx_id, from_addr, to_addr, asset, amount_raw, token_address, token_decimals, height, block_time, provider, raw_json, PK(chain, transfer_key))`, where `transfer_key` comes from `externalTransferKey` (`trackers_transfers.go`).
- `ledger_transfer_addresses(chain, address, block_time, transfer_key)`.
- `ledger_coverage(source, chain, address, from_ts, to_ts, fetched_at)`. `source` is `midgard:THOR`, `midgard:MAYA` or a provider name.

Steps:
1. **`ledger_store.go` (upsert and query).** Queries return `[]midgardAction` and `[]externalTransfer`, so projection is unchanged.
2. **`ledger_coverage.go`.**
   - `gaps(source, chain, addr, from, to)` returns the missing intervals. `markCovered` merges overlapping or adjacent intervals.
   - When a fetch is truncated, the range from the oldest returned item to `to` still counts as covered.
   - Anything after `fetched_at - 10m` is always treated as a gap, so recent activity gets picked up.
3. **Rewire the fetch paths to fetch gaps, then query the ledger:**
   - `fetchMidgardActionsForAddressOnlyFromProtocol` (`midgard_fetch.go`), reusing the existing paged fetchers and their `fromTimestamp`/`timestamp` parameters.
   - `fetchExternalTransfersForAddress` (`trackers_transfers.go`).
   - Retire `lookup*/insert*Cache` (`store.go`).
4. **Backfill** in Go migration code: import the existing cache rows (961 Midgard, 859 external) into the ledger plus coverage. Migration 6 drops the old cache tables once the phase is verified.

**Exit:**
- Golden tests are unchanged.
- Rebuilding the same window makes zero upstream requests (the replay transport's hit counter shows this).
- Shifting the window by a day fetches only the missing day (an `httptest` server checks the requested timestamps).

## Phase 3: Provider gateway, job runner, live holdings on the server

**Status (2026-10-05): done.** The holdings snapshot table is migration 6, since the old caches were dropped in app code. The browser check was skipped because the extension was disconnected. Partial graphs are exposed by the API (`?partial=1`); the UI shows partial counts rather than re-rendering the canvas mid-build, to avoid relayout churn. Details are in session 2026-10-04 #4.

1. **Typed provider errors.** Classify at the HTTP layer (the `getJSONAbsolute*`/`postJSONAbsolute*` helpers in `trackers_http.go`, and `ThorClient.GetJSON`) as `RateLimited(RetryAfter)`, `Banned` (403/challenge), `Transient` (5xx/timeout), `Permanent` (4xx/decode) or `Config` (DNS/missing key). This replaces string matching such as `isHTTPStatusError`.
2. **Circuit breaker** in `tracker_health.go`.
   - `allow(provider, chain) (bool, retryAt)` uses the stored `Retry-After` and failure counts. A ban opens the circuit for 15 minutes; a 429 respects `Retry-After`.
   - `ThorClient.rotatedEndpoints` skips endpoints whose circuit is open.
3. **Cache the bond index.** `fetchProtocolBondIndexes` (`trackers_holdings.go`) goes through the `metadata_cache.go` pattern with a 60-second TTL.
4. **Job runner** (`jobs.go`): generalise the map branch's `build_progress.go` progress tokens into `Job{ID, Kind, Status, Progress, Partial, Warnings, Err, LogPath, cancel}`.
   - Kinds: `actor_graph_build`, `actor_graph_expand`, `explorer_build`, `live_holdings`.
   - API: `GET /api/v1/jobs/{id}` and `DELETE` (cancel). The build and expand endpoints return a `job_id`. The frontend polls and renders the partial graph.
5. **Live holdings on the server.**
   - Migration 7 adds `holdings_snapshots(chain, address, taken_at, status, holdings_json, error_kind, error_detail, retry_at)`.
   - Lookups are queued per provider and respect circuits.
   - Final statuses are `available`, `zero` and `unavailable(reason)`. `pending` carries a `retry_at`.
   - Snapshots are reused for 10 minutes.
   - Delete the browser retry loop in `useActorGraphController.ts` (`refreshGraphLiveHoldings`); a manual refresh becomes a forced job.
6. **Per-job logs.** Write `data/logs/runs/<job-id>.jsonl` instead of overwriting a single file through `saveLastRunLog` (`app.go`). `last-run.log` becomes a copy of the most recent build job's log only.

**Write these failing tests first:**
- A persistent 403 reaches `unavailable(banned)` without retries during the circuit window.
- A 429 with `Retry-After` is retried only after that window.
- The bond list is fetched once across several chunks.
- A holdings job does not overwrite the build log.

**Exit:** those tests pass, the frontend no longer has a retry loop, and live values for golden case A end in a final state.

## Phase 4: Prices at transaction time

**Status (2026-10-05): done.** Price points are migration 7. Unpriced transfers touching an actor stay visible. The UI normalises at-time values at the API boundary. Bitget acceptance: $265,723 per 100 ETH. Details are in session 2026-10-05 #1.

1. **`price_history.go`:** `PriceAt(ctx, asset, t) (usd, source, ok)`. Sources, in order:
   1. THOR/MAYA pool history from Midgard `/v2/history/depths/{pool}?interval=hour` (`assetPriceUSD`). RUNE comes from `/v2/history/rune` (check the exact field at implementation) or from stable-pool depth ratios, as `buildPriceBookFresh` does today.
   2. A $1 peg for stablecoins, using the existing `isStableAsset`.
   3. DefiLlama `coins.llama.fi/prices/historical` for EVM tokens by contract address. Behind a config flag, on by default.
   4. Otherwise the transfer is marked unpriced.
2. **Cache:** migration 8 `price_points(asset, interval, bucket_ts, usd, source, PK(asset, interval, bucket_ts))`, filled in day-sized blocks.
3. **Model:** add `usd_at_time` and `price_source` to `FlowAssetValue`, `FlowEdgeTransaction` and `SupportingAction` (`types.go`). Edge USD becomes the sum of per-transaction `usd_at_time`. `usd_spot` stays for "worth now". Switch `usdFor` to big-decimal maths to avoid int64 overflow on large raw amounts.
4. **Filter:** `min_usd` uses `usd_at_time`. Unpriced segments are excluded unless an endpoint belongs to an actor or the request sets `include_unpriced` (fixes all three copies of the filter, in `projection.go`, `rujira_trace.go` and `trackers_transfers.go`).
5. **Frontend:** show the at-time USD and its source; label the filter "Min USD (at time)"; add an include-unpriced toggle.

**Exit:**
- Golden diffs are limited to the new USD fields. Regenerate them once and review.
- In case B, a 100 ETH batch is valued at about $265k (the figure in public reporting), within 5%.

## Phase 5: Labels

**Status (2026-10-05): done.** User labels stay in `address_annotations` (kind=label) and are merged at lookup instead of being migrated. Imported 509k GraphSense, 106k eth-labels, 2.5k ScamSniffer and 1k OFAC labels. Details are in session 2026-10-05 #2.

1. **Tables (migration 9):** `labels(chain, address, normalized_address, label, category, actor_name, source, source_ref, confidence, created_at, UNIQUE(normalized_address, source, label))` and `label_sources(source, version, license, imported_at, count)`. Categories follow the GraphSense TagPack / INTERPOL DWVA taxonomy, embedded as a file.
2. **Importers** (`labels_import.go`), idempotent, run through a server subcommand `labels import <source> <path>`:
   - **builtin:** move `knownAddressLabels` into the embedded file `labels/builtin.yaml` (TagPack format). `knownCalcRepresentativePayouts` is a routing rule, so it stays in code.
   - **user:** move the 44 `address_annotations` rows of `kind=label` to `source=user`. The annotations UI writes `user` labels from then on.
   - **graphsense-tagpacks:** a local clone; parse YAML with header→tag inheritance.
   - **OFAC:** the 0xB10C `sanctioned_addresses_<ASSET>.txt` lists, as category `sanctioned`.
   - **eth-labels:** check its export format at implementation.
   - **ScamSniffer:** imported from a file you download; never committed to the repo, because the data is GPL.
3. **Resolution:** precedence is user > builtin > imported, then by confidence. Nodes carry `label_category`, `label_source` and `label_confidence`. The detail panel lists every label. Graphs show category badges and a category filter.

**Exit:**
- After importing OFAC, a known SDN ETH address shows as sanctioned in the explorer.
- User labels still win.
- Running an import twice gives the same counts.

## Phase 6: Actor monitoring

1. **Watch flag.** Migration 10 adds `watch` and `last_viewed_at` to `actors`, plus `actor_snapshots(actor_id, taken_at, total_usd, holdings_json, flows_summary_json)`.
2. **`actor_refresh` job.** For each address: a ledger tail fetch (gaps only, from coverage end to now), plus a holdings snapshot from Phase 3, then write `actor_snapshots`.
3. **Scheduler.** An in-process ticker set by `CHAIN_ANALYSIS_ACTOR_REFRESH_INTERVAL` (default off; for example `1h`) refreshes watched actors through the job runner.
4. **"Since last look" UI** on the Actors page. Charts must follow the `dataviz` skill.
   - Holdings over time (line chart).
   - Change by asset and by address.
   - A table of new flows with at-time USD and counterparty labels.
   - First-seen counterparties.
   - Movements above a threshold, highlighted.

**Exit:**
- For TC Treasury, a second refresh makes only tail requests (checked).
- There are at least two snapshots.
- The diff view lists the flows between them.

**Status (2026-10-05): done.** Snapshots and watch state are migration 9 (`actor_watch` and `actor_snapshots` tables rather than new `actors` columns). A refresh reuses the graph builder over the new window with max_hops 1, so the ledger fetches only the tail, then runs the holdings job. Asset deltas are computed from the per-address asset lists in the snapshots. Real-data check: TC Treasury's second refresh fetched Midgard only from the previous window end minus the 10-minute lag. Details are in session 2026-10-05 #3.

## Phase 7: "Follow this amount" trace

1. **API:** `POST /api/v1/analysis/trace` starts a job. Request fields:
   - seeds: `{chain, address}` or `{tx_id}`
   - `start_time`, optional `amount`+`asset`
   - `direction` (forward|backward), `policy` (fifo|haircut|largest_out)
   - `max_depth`, `max_branches`, `min_usd_at_time`
   - `stop_categories` (default: exchange, sanctioned, mixer)
   - LP into a THOR pool ends as a "pool" sink.
2. **Engine** (`trace.go`).
   - It consumes the custody segments that existing projection produces from ledger data: `projectMidgardActionWithExternal`, and `stitchMidgardAction` in `projection.go`. Swaps therefore already arrive as wallet→recipient with an asset conversion.
   - For each address, it orders inflows and outflows by time and allocates the traced amount by policy.
   - Swaps convert the traced amount using the action's in/out ratio.
   - Gaps in coverage lower confidence and are reported.
3. **Real confidence values.**
   - Stitching sets `projectedSegment.Confidence` and `confidence_reason`: an exact txid or memo match scores 1.0; an amount-and-time-window match scores about 0.7 with its reason.
   - Matches on affiliate-fee legs alone stay excluded.
4. **Output:** reuses `FlowNode`/`FlowEdge`, with extra fields:
   - Edges get `traced_amount`, `traced_usd_at_time`, `confidence` and `confidence_reason`.
   - The result adds `sinks` (labelled endpoints, and unlabelled terminal holders with their current holdings), `frontier` (where limits stopped the trace) and `coverage_gaps`.
5. **UI:** a Trace page with the form, a graph (reusing `GraphCanvas`), a sinks table, a flows table with transaction IDs and explorer links, and a summary of the method used.

**Exit (acceptance B):**
- A forward FIFO trace from the exploiter's address finds all 31 swaps, including the two partial fills.
- The sink is one BTC address with about 87.82 BTC traced (within 1%).
- Every edge has transaction IDs.
- It finishes in under 2 seconds on replay and under 60 seconds live when uncached.

**Status (2026-10-05): done.**
- **Storage and API:** trace runs are migration 10 (`trace_runs`). The job is `POST /api/v1/jobs/trace`, with `/api/v1/analysis/trace` as an alias.
- **Tx-ID matching:** fixing the EVM `0x` matching revealed a second bug. Deposits and payouts through rotated vaults were drawn twice, inside the swap and as separate transfers. Both were fixed test-first.
- **Confidence:** projection now scores 1 for legs THORChain records, with lower, explained scores for inferred legs.
- **Acceptance, against the corrected figures (31 swaps, 87.823 BTC):**
  - Replay takes 0.07 s.
  - A live, uncached depth-3 trace takes 39 s.
- **Not one sink:** the exploiter also sent 119.34 ETH to another wallet, so the BTC destination is not the only endpoint. The test checks that the swaps reach it and that its BTC is conserved.

Details are in session 2026-10-05 #4.

## Phase 8: Cases, export, CLI, MCP

1. **Cases.** Migration 11 adds `cases(id, title, notes_md, created_at, updated_at)` and `case_items(case_id, kind, ref, note, pinned_at)`. Item kinds are address, tx, trace_run, graph_state and actor. Graph states (from the map branch) and trace runs attach to cases.
2. **Export.** `GET /api/v1/cases/{id}/export?format=md|csv`.
   - The Markdown report covers: summary, seeds, method and policy, sinks, a flows table (time, from→to with labels, amount, at-time USD, linked txid), coverage gaps and warnings, and label sources.
   - The CSV is the flows table.
3. **CLI** `cmd/cactl`: a thin `/api/v1` client with JSON output.
   - Commands: `trace`, `actor refresh|summary`, `labels import`, `case export`, `ledger gaps`.
4. **MCP server** `cmd/cactl-mcp` (stdio, official Go SDK `github.com/modelcontextprotocol/go-sdk`; pin the version at implementation).
   - Tools: `trace_funds`, `actor_summary`, `address_profile` (labels, holdings, recent flows), `tx_lookup`, `case_export`.
   - It uses the same client as the CLI and is registered in the repo's `.mcp.json`.

**Exit:**
- `cactl trace` reproduces the case B sinks.
- The MCP tool is callable from Claude Code.
- The exported Markdown renders with working explorer links.

---

## Out of scope

Validator operator attribution and clustering, protocol-level incident forensics, a self-hosted Midgard or full node, external alerts (Telegram and the like), multiple users, and moving code into separate Go packages beyond `app`.

## Ordering and dependencies

0 → 1 → 2 → 3 → (4 and 5 are independent and can run in parallel) → 6 and 7 (independent; 7 needs 4 and 5) → 8.

## Verification (every phase)

- `go test ./...` and `npm --prefix frontend test` pass. Golden tests replay in about a second: `go test ./internal/app -run Golden`.
- `make restart-server`, then `curl -s localhost:8090/api/v1/health`; check `build.commit` and `build.build_time`.
- Live smoke test:
  - Build the TC Treasury graph twice; the second build's run log shows no upstream requests for covered ranges.
  - No `thorchain.network` hosts appear in `data/logs/runs/*`.
  - From Phase 7 on, run the case B trace from both the UI and `cactl`.
- For UI phases, check in the browser with claude-in-chrome: the graph renders, live values reach a final state, and the trace page shows the sinks table.
- Run the workspace audit before every commit or push.
