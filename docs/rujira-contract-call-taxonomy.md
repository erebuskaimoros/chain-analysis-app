# Rujira Contract Call Taxonomy

This file is the human-readable companion to the runtime taxonomy in
`internal/app/rujira_taxonomy.go`.

## Graph Rule

The actor graph should only render rows where `funds_move = yes`.

- `caller -> contract` when liquidity is parked in a contract.
- `contract -> recipient` when a contract pays out or forwards liquidity.
- `hide` for internal orchestration, callbacks, replies, queries, admin, migrate, and sudo.
- `hide` for zero-value contract calls even if Midgard emits a synthetic `contract` action row.

## Categories

- `public-user`: wallet-facing user action.
- `role-gated`: callable only by a market, executor, router, or controller.
- `internal-pipeline`: callback, reply, scheduler, settlement, or self-call.
- `query-read`: state inspection only.
- `admin-control`: owner, sudo, migrate, instantiate, or config update.

## Matrix

The flat matrix lives in
[`docs/rujira-contract-call-taxonomy.csv`](/Users/boonewheeler/Desktop/Projects/THORChain/chain-analysis-app/docs/rujira-contract-call-taxonomy.csv).

Use these columns as the graph policy input:

- `call_class`: existing graph bucket (`liquidity`, `swaps`, `bonds`, `transfers`, `read`, `control`)
- `funds_move`: whether the call should ever render as a liquidity edge
- `graph_behavior`: `address_to_contract`, `contract_to_address`, `address_to_address`, `mixed`, or `hide`
