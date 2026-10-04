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

## Notes

- A first query against an uncached address or window takes longer while
  Midgard pages and tracker history are fetched and cached.
- `GET /api/v1/health` includes build metadata (`build.version`, `build.commit`,
  `build.build_time`) so you can confirm the running binary matches source.
