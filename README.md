# THORChain Chain Analysis App

Local investigative app for THORChain fund flows with **on-demand ingestion**.

It never backfills the chain. When a query needs an address's history, the app
fetches that address's THORChain/MAYA actions from Midgard and its external-chain
transfers from per-chain trackers, caches the results in SQLite, and projects
them into an actor/address flow graph. The UI is a React/TypeScript app embedded
in the Go server.

## Features

- Actor registry (named address sets) and multi-actor flow graphs with
  Cytoscape.js + ELK layout, incremental expansion, search, minimap, and a time
  scrubber.
- THORChain-native projection: swaps collapse inbound/outbound legs into
  sender → recipient, LP adds/withdraws become pool custody edges, bond/rebond
  continuity, and Rujira/CALC contract-call taxonomy.
- Address Explorer for single-address graphs across THOR and external chains.
- Cross-chain trackers for BTC, LTC, BCH, DOGE, ETH, BSC, AVAX, BASE, GAIA, SOL,
  TRON, XRP, and Radix, with per-chain provider selection and failover.
- Live holdings for graph nodes (bank, LP, and bond positions on THOR/MAYA;
  native and token balances on external chains).
- Action lookup by transaction ID, address annotations, blocklist, saved graph
  runs, and server-side saved graph states.
- Actor monitoring: each refresh records an actor's holdings and the flows since
  the previous refresh, with a "since you last looked" view on the Actors page.
- Follow-the-funds tracing: from an address or a THORChain transaction, follow
  value hop by hop (through swaps across chains) to exchanges, sanctioned
  addresses, pools, bonds, or wallets that still hold it.
- Investigation cases with Markdown and CSV export, a `cactl` command-line
  client, and an MCP server (`cactl-mcp`) for Claude Code and other MCP clients.

## API

All endpoints live under `/api/v1`:

- `GET /api/v1/health`
- `GET /api/v1/actions/{txid}`
- `GET|POST /api/v1/actors`
- `PUT|DELETE /api/v1/actors/{id}`
- `GET|PUT|DELETE /api/v1/annotations`
- `GET|POST /api/v1/blocklist`
- `DELETE /api/v1/blocklist/{address}`
- `POST /api/v1/analysis/actor-graph`
- `POST /api/v1/analysis/actor-graph/expand`
- `POST /api/v1/analysis/actor-graph/live-holdings`
- `GET /api/v1/analysis/actor-graph/progress/{token}`
- `POST /api/v1/analysis/address-explorer`
- `GET /api/v1/runs/actor-graph`
- `DELETE /api/v1/runs/actor-graph/{id}`
- `GET /api/v1/runs/address-explorer`
- `DELETE /api/v1/runs/address-explorer/{id}`
- `GET|POST /api/v1/graph-states`
- `GET|DELETE /api/v1/graph-states/{id}`
- `POST /api/v1/jobs/actor-graph`, `/jobs/actor-graph/expand`,
  `/jobs/address-explorer`, `/jobs/live-holdings`, `/jobs/actor-refresh`
- `GET|DELETE /api/v1/jobs/{id}` (poll or cancel a job)
- `GET /api/v1/labels?address=`, `GET /api/v1/labels/sources`
- `GET /api/v1/actors/{id}/monitor`, `PUT /api/v1/actors/{id}/watch`,
  `POST /api/v1/actors/{id}/viewed`
- `POST /api/v1/jobs/trace` (alias `POST /api/v1/analysis/trace`),
  `GET /api/v1/traces`, `GET|DELETE /api/v1/traces/{id}`
- `GET|POST /api/v1/cases`, `GET|PUT|DELETE /api/v1/cases/{id}`,
  `POST /api/v1/cases/{id}/items`, `DELETE /api/v1/cases/{id}/items/{item_id}`,
  `GET /api/v1/cases/{id}/export?format=md|csv`
- `POST /api/v1/jobs/address-profile`, `POST /api/v1/jobs/labels-import`,
  `GET /api/v1/ledger/coverage?address=&start=&end=`

## Run

From `chain-analysis-app/`:

```bash
make restart-server
```

Open [http://localhost:8090](http://localhost:8090).

The restart script:
- stops the server using the PID file (`data/run/server.pid`)
- force-kills any leftover listener on the configured port
- builds the React/TypeScript UI bundle into `internal/web/ui/dist`
- rebuilds `data/bin/chain-analysis-server` with embedded build metadata
- starts fresh and verifies `/api/v1/health`

Use `make stop-server` to stop and `make build-server` to build without
restarting. The UI is embedded with `go:embed`, so rebuild the server after
`npm --prefix frontend run build`.

## Environment

- `CHAIN_ANALYSIS_ADDR` (default `:8090`)
- `CHAIN_ANALYSIS_DB` (default `data/chain-analysis.db`)
- `CHAIN_ANALYSIS_UI_BUILD_DIR` (default `internal/web/ui/dist`)
- `CHAIN_ANALYSIS_LAST_RUN_LOG` (default `data/logs/actor-tracker-last-run.log`)
- `CHAIN_ANALYSIS_TIMEOUT_SECONDS` (default `20`)
- `CHAIN_ANALYSIS_MIDGARD_TIMEOUT_SECONDS` (default `10`)
- `THORNODE_ENDPOINTS` (default `https://gateway.liquify.com/chain/thorchain_api`)
- `MIDGARD_ENDPOINTS` (default `https://gateway.liquify.com/chain/thorchain_midgard/v2`)
- `MAYANODE_ENDPOINTS` (default `https://mayanode.mayachain.info`)
- `MAYA_MIDGARD_ENDPOINTS` (default `https://midgard.mayachain.info/v2`)
- `CHAIN_ANALYSIS_LEGACY_ACTION_ENDPOINTS` (default empty; set, for example, `https://vanaheimex.com` to merge a second THOR action-history source)
- `CHAIN_ANALYSIS_ACTOR_REFRESH_INTERVAL` (default off; for example `1h`
  refreshes watched actors on that interval)
- `CHAIN_ANALYSIS_DEFILLAMA_URL` (default `https://coins.llama.fi`; prices EVM tokens without a THORChain pool at transaction time; set empty to disable)
- `CHAIN_ANALYSIS_CHAIN_TRACKERS` per-chain provider overrides, for example
  `BASE=blockscout`
- Tracker URLs and keys: `CHAIN_ANALYSIS_ETHERSCAN_API_URL`,
  `CHAIN_ANALYSIS_ETHERSCAN_API_KEY`, `CHAIN_ANALYSIS_BLOCKSCOUT_API_URLS`,
  `CHAIN_ANALYSIS_BLOCKSCOUT_API_KEYS`, `CHAIN_ANALYSIS_AVACLOUD_API_KEY`,
  `CHAIN_ANALYSIS_NODEREAL_API_KEY`, `CHAIN_ANALYSIS_TRONGRID_API_KEY`,
  `CHAIN_ANALYSIS_UTXO_TRACKERS`, `CHAIN_ANALYSIS_COSMOS_TRACKERS`,
  `CHAIN_ANALYSIS_SOLANA_RPC_URL`, `CHAIN_ANALYSIS_XRP_RPC_URL`,
  `CHAIN_ANALYSIS_RADIX_GATEWAY_URL`

Tracker endpoint values can be multi-homed:

- use `|` between equivalent endpoints for a single provider, for example `CHAIN_ANALYSIS_SOLANA_RPC_URL=https://api.mainnet-beta.solana.com|https://solana-rpc.publicnode.com`
- use `CHAIN=URL1|URL2` inside chain maps, for example `CHAIN_ANALYSIS_UTXO_TRACKERS=BTC=https://blockstream.info/api|https://mempool.space/api`

Do not configure Nine Realms THORChain endpoints.

## Address labels

Labels attribute addresses to exchanges, sanctioned parties, scams, protocols, and other entities. Built-in labels ship with the app; import third-party sources from local copies (keep them under the gitignored `data/labels/`):

```bash
git clone --depth 1 https://github.com/graphsense/graphsense-tagpacks data/labels/tagpacks
git clone --depth 1 --branch lists https://github.com/0xB10C/ofac-sanctioned-digital-currency-addresses data/labels/ofac
curl -o data/labels/eth-labels-accounts.json https://raw.githubusercontent.com/dawsbot/eth-labels/v1/data/json/accounts.json
curl -o data/labels/scamsniffer-address.json https://raw.githubusercontent.com/scamsniffer/scam-database/main/blacklist/address.json

data/bin/chain-analysis-server labels import graphsense data/labels/tagpacks
data/bin/chain-analysis-server labels import ofac data/labels/ofac
data/bin/chain-analysis-server labels import eth-labels data/labels/eth-labels-accounts.json
data/bin/chain-analysis-server labels import scamsniffer data/labels/scamsniffer-address.json
data/bin/chain-analysis-server labels sources
```

User labels (annotations) always take precedence.

## Actor monitoring

Open an actor's **Monitor** panel on the Actors page and choose **Refresh now**.
The first refresh records a baseline: holdings plus the last 30 days of flows.
Each later refresh fetches only the flows since the previous one (the ledger
fetches just the tail) and records a new holdings snapshot. The panel shows:

- holdings over time
- changes by asset and by address
- flows by counterparty, valued at transaction time
- counterparties seen for the first time

Changes above the chosen USD threshold are highlighted. **Mark as seen** resets
the comparison point. Tick **Refresh on schedule** and set
`CHAIN_ANALYSIS_ACTOR_REFRESH_INTERVAL` to refresh watched actors in the
background.

## Tracing funds

The Trace page (or `POST /api/v1/jobs/trace`) follows value from seeds:

- **Seeds:** addresses (`CHAIN|address` when the chain is ambiguous), or a
  THORChain transaction hash. A transaction seed follows that deposit's amount.
- **Direction:** `forward` (where did it go) or `backward` (where did it come
  from).
- **Allocation policy** at each address:
  - `fifo`: payments spend the oldest funds first.
  - `haircut`: each payment carries the address's traced share.
  - `largest_out`: traced funds go to the largest payments first.
- **Limits:** hops, branches per address, and a minimum USD value at
  transaction time.
- **Stops:** labels in the stop categories (default exchange, sanctioned,
  mixer), liquidity pools, validator bonds, and contracts.

Swaps carry the traced share of their input into the output asset. Each traced
edge lists its transactions (swaps show both the deposit and the payout hash),
the value at transaction time, and a confidence with its reason. Results list:

- **sinks:** where the value rests, including addresses still holding it.
- **frontier:** where a limit stopped the trace.
- **coverage gaps:** history the providers truncated or could not return.

Every finished trace is saved and can be reopened.

## Cases and export

A case collects what an investigation found: addresses, transactions, saved
traces, saved graph states and actors, each with a note, plus Markdown notes
for the case. Pin a trace from its result on the Trace page, or pin anything
on the Cases page.

**Export Markdown** writes a report with these sections:

- summary
- pinned items
- each trace's seeds, method, sinks, frontier and flows, linked to the chain
  explorers
- coverage gaps
- the label sources behind the labels

**Export flows CSV** writes one row per traced transaction.

## Command line (`cactl`)

`cactl` drives a running server over `/api/v1` and prints JSON (case exports
print the report itself). Build it with `make build-cli` (into `data/bin/`) or
run it with `go run ./cmd/cactl`. Set `--url` or `CHAIN_ANALYSIS_URL` for a
server other than `http://localhost:8090`.

```bash
cactl trace --seed 'ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3' \
  --start 2026-09-28T03:00:00Z --end 2026-09-28T07:00:00Z --max-depth 3 --holdings
cactl trace --tx <deposit hash> --direction backward
cactl actor refresh "TC Treasury"; cactl actor summary "TC Treasury"
cactl labels import ofac data/labels/ofac; cactl labels lookup <address>
cactl case create "Bitget hack"; cactl case add 1 trace_run 1 --note "FIFO, 3 hops"
cactl case export 1 --out case.md; cactl case export 1 --format csv
cactl ledger gaps <address> --start 2026-09-01T00:00:00Z
cactl profile <address> --days 30; cactl tx <hash>
```

## MCP server (`cactl-mcp`)

`cactl-mcp` serves these tools over MCP on stdio:

- `trace_funds`
- `address_profile`
- `actor_summary`
- `tx_lookup`
- `case_list`
- `case_export`

Each tool returns a compact digest. The repository's `.mcp.json` registers it
for Claude Code as `chain-analysis` (it runs `go run ./cmd/cactl-mcp` against
`http://localhost:8090`), so start the server first.

## Notes

- A first query against an uncached address or window takes longer while
  Midgard pages and tracker history are fetched and cached.
- `GET /api/v1/health` includes build metadata (`build.version`, `build.commit`,
  `build.build_time`) so you can confirm the running binary matches source.
