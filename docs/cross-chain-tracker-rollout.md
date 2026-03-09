# Cross-Chain Tracker Rollout

## Goal

Track liquidity custody across every THOR-supported external chain while preserving the graph rule set:

- show only address-to-address or address-to-pool custody changes
- collapse THOR ingress, router, and Asgard transit out of swap paths
- keep pool custody edges for LP adds and withdraws
- avoid orchestration-only, approval-only, and zero-value edges

## Current State

- `BTC`, `LTC`, `BCH`, `DOGE`, `GAIA`, `SOL`, `TRON`, and `XRP` already have native-chain tracker paths.
- `ETH`, `BSC`, `AVAX`, and `BASE` still share one `etherscan`-style adapter.
- Tracker selection is now configurable per chain through `CHAIN_ANALYSIS_CHAIN_TRACKERS`.

Default provider map:

```text
ETH=etherscan
BSC=etherscan
AVAX=etherscan
BASE=etherscan
BTC=utxo
LTC=utxo
BCH=utxo
DOGE=utxo
GAIA=cosmos
SOL=solana
TRON=trongrid
XRP=xrpl
```

## Phase 1

- Add a per-chain provider registry in config and tracker dispatch.
- Expose configured provider selection in `/api/health`.
- Keep current behavior unchanged while making later provider swaps isolated.

Exit criteria:

- external tracker dispatch no longer hardcodes provider choice by chain family
- provider selection can be overridden for one chain without replacing the whole default map
- tests cover default and overridden provider resolution

## Phase 2

- Implement `BASE -> blockscout`.
- Keep the same normalized `externalTransfer` shape.
- Reuse existing graph projection logic so no UI change is required.

Exit criteria:

- `CHAIN_ANALYSIS_CHAIN_TRACKERS=BASE=blockscout` works
- BASE native and token balance-moving transfers appear as liquidity edges

## Phase 3

- Implement `AVAX -> avacloud`.
- Support both native AVAX transfers and token transfers.
- Add rate-limit and truncation warnings specific to the provider.

## Phase 4

- Implement `BSC -> nodereal` or another durable address-history provider.
- Add request throttling and cache reuse before enabling wider hop expansion.

## Phase 5

- Add a THOR stitch layer that correlates external inbound legs with THOR outbound settlement.
- Render swaps as `wallet -> recipient`.
- Render LP adds and withdraws as `wallet -> pool` and `pool -> wallet`.

Exit criteria:

- inbound vault, router, and Asgard addresses are treated as transit-only in cross-chain flows
- swaps no longer stop at THOR ingress addresses

## Phase 6

- Add provider health scoring, backoff, and cache tables.
- Prefer the healthiest configured provider per chain when multiple providers are available.
- Surface per-provider warnings and truncation in the API response.
