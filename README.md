# THORChain Chain Analysis App

Production-oriented local app for THORChain analysis with **on-demand ingestion**.

It does not backfill the entire chain by default. It ingests blocks only when needed by a query, caches them in SQLite, and reuses the cached data for later requests.

The root UI is now a React/TypeScript shell served by the Go app. The full legacy workspace remains available at `/legacy/` while feature migration continues.

## Features

- Live THORNode data integration with endpoint failover.
- Cached block/event ingestion in SQLite.
- Saved actor registry in SQLite.
- Combined multi-actor flow DAG with Cytoscape.js + ELK layout.
- Action trace lookup by TX hash.
- Wallet liquidity flow analysis.
- Wallet bond/rebond/unbond analysis.
- REBOND continuity tracing (old bond address -> new bond address).
- Node bond-provider inspection.
- Web dashboard with tabbed query surfaces and Actor Tracker.

## API

### Legacy API

- `GET /api/health`
- `GET|POST /api/ingest/recent`
- `GET /api/overview`
- `GET /api/actions/{txid}`
- `GET /api/wallets/{address}/liquidity`
- `GET /api/wallets/{address}/bonds`
- `GET /api/rebond/{address}`
- `GET /api/nodes/{address}/bond-providers`
- `GET /api/actors`
- `POST /api/actors`
- `PUT /api/actors/{id}`
- `DELETE /api/actors/{id}`
- `POST /api/actor-tracker/graph`

### V1 API

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
- `POST /api/v1/analysis/address-explorer`
- `GET /api/v1/runs/actor-graph`
- `DELETE /api/v1/runs/actor-graph/{id}`
- `GET /api/v1/runs/address-explorer`
- `DELETE /api/v1/runs/address-explorer/{id}`

## Run

From `chain-analysis-app/`:

```bash
go mod tidy
npm --prefix frontend install
npm --prefix frontend run build
go run ./cmd/server
```

Open [http://localhost:8090](http://localhost:8090).

Open [http://localhost:8090/legacy/](http://localhost:8090/legacy/) for the legacy workspace.

For deterministic restarts that always pick up latest code:

```bash
make restart-server
```

This script:
- stops using PID file first (`data/run/server.pid`)
- force-kills any leftover listener on the configured port
- builds the React/TypeScript UI bundle in `internal/web/ui/dist`
- rebuilds `data/bin/chain-analysis-server` with embedded build metadata
- starts fresh and verifies `/api/health`

Use `make stop-server` to stop and `make build-server` to build without restarting.

## Environment

- `CHAIN_ANALYSIS_ADDR` (default `:8090`)
- `CHAIN_ANALYSIS_DB` (default `data/chain-analysis.db`)
- `CHAIN_ANALYSIS_STATIC_DIR` (default `internal/web/static`)
- `CHAIN_ANALYSIS_UI_BUILD_DIR` (default `internal/web/ui/dist`)
- `CHAIN_ANALYSIS_TIMEOUT_SECONDS` (default `20`)
- `CHAIN_ANALYSIS_DEFAULT_SCAN_BLOCKS` (default `120`)
- `CHAIN_ANALYSIS_MAX_SCAN_BLOCKS` (default `3000`)
- `CHAIN_ANALYSIS_MAX_GRAPH_SCAN_BLOCKS` (default `30000`)
- `THORNODE_ENDPOINTS` (default `https://thornode.thorchain.network,https://thornode.thorchain.liquify.com`)
- `MIDGARD_ENDPOINTS` (default `https://midgard.thorchain.network/v2,https://midgard.thorchain.liquify.com/v2`)

Tracker endpoint values can be multi-homed:

- use `|` between equivalent endpoints for a single provider, for example `CHAIN_ANALYSIS_SOLANA_RPC_URL=https://api.mainnet-beta.solana.com|https://solana-rpc.publicnode.com`
- use `CHAIN=URL1|URL2` inside chain maps, for example `CHAIN_ANALYSIS_UTXO_TRACKERS=BTC=https://blockstream.info/api|https://mempool.space/api`

Current built-in non-THOR defaults now include validated failover pairs for:

- `BTC` Esplora: `blockstream.info` and `mempool.space`
- `GAIA` REST: `rest.cosmos.directory`, `ccvalidators`, and `Lavender.Five`
- `SOL` RPC: Solana Labs public RPC and PublicNode
- `XRP` RPC: `s1.ripple.com`, `s2.ripple.com`, and `xrplcluster.com`

## Notes

- The app is designed for incremental ingestion and analysis. Use `/api/ingest/recent` only when you want to pre-warm recent ranges.
- Actor Tracker only ingests blocks needed for the requested time slice. A first query against an uncached recent window can take noticeably longer while those blocks are fetched and cached.
- Midgard can be added later as an optional connector, but this implementation already supports the full flow with THORNode block/event + tx-detail sources.
- `GET /api/health` includes build metadata (`build.version`, `build.commit`, `build.build_time`) so you can confirm the running binary matches source state.
