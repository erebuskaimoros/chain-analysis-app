# Session 2 - External Tracker Resilience and RPC Batch Optimization

> Date: 2026-03-08
> Focus: Fix graph frontier loss on external tracker failure, batch JSON-RPC calls, tune Midgard timeout

## Summary

Fixed a critical bug where external tracker failures (e.g. Etherscan timeout) caused the graph builder to skip ALL Midgard action processing for that address, killing frontier discovery entirely. Also consolidated JSON-RPC calls for Solana and NodeReal from N+1 patterns into single batch requests, added a Solana throttle policy, and bumped the Midgard timeout from 5s to 10s.

## Work Done

- Fixed `continue` → `externalTransfers = nil` in graph builder loop (actor_tracker.go:485) so Midgard actions are always processed even when external tracker calls fail
- Batched Solana `getTransaction` calls: 51 HTTP requests/page → 2 (1 `getSignaturesForAddress` + 1 batch `getTransaction`)
- Batched NodeReal `nr_getTransactionDetail` calls: 1+N HTTP requests → 2 (1 page query + 1 batch detail)
- Added Solana throttle policy: 1 concurrent, 200ms spacing
- Bumped Midgard timeout from 5s to 10s default
- Removed old per-signature `fetchSolanaTransactionTransfers`, replaced with `fetchSolanaTransactionTransfersBatch` + `parseSolanaTransactionTransfers`
- Removed old per-hash `fetchNodeRealTokenTransfersByHash`, replaced with `fetchNodeRealTokenTransfersByHashBatch` + `parseNodeRealTokenTransfers`
- Updated NodeReal test mock to handle both single and batch JSON-RPC requests
- Audited ALL external tracker providers for consolidation opportunities (Etherscan, Blockscout, AvaCloud, NodeReal, Esplora, TronGrid, XRPL, Cosmos, Solana)

## Discoveries

- **Graph builder `continue` on external tracker failure was silently dropping Midgard actions** — when Etherscan timed out for the seed ETH address, ALL swap/bond/send actions from Midgard were skipped, preventing counterparty address discovery. This was masked pre-THORNode-removal because events provided a separate discovery path.
- Post-fix graph results jumped from 135→465 actions, 57→215 edges, 41→190 nodes
- Solana public RPC (`api.mainnet-beta.solana.com`) is very aggressive with 429 rate limiting; the old code sent 51 sequential RPC calls per page with no spacing
- JSON-RPC 2.0 batch format (array of request objects) is supported by both Solana and NodeReal, enabling significant call consolidation
- Etherscan has no batch API — `tokenbalance`, `txlist`, `txlistinternal`, `tokentx` are all individual REST endpoints with no multi-query support
- `addresstokenbalance` (Etherscan PRO) already returns all tokens in 1 paginated call — our primary path is optimal
- Providers already efficient: Esplora (1 call/page), AvaCloud (1 call with all data), TronGrid (1 call/page), XRPL (1 call/page), Cosmos (2 calls/page for sender+recipient)

## Files Changed

| File | Change |
|------|--------|
| `internal/app/actor_tracker.go` | Fixed `continue` → `externalTransfers = nil` on external tracker failure |
| `internal/app/config.go` | Bumped Midgard timeout default from 5s to 10s |
| `internal/app/tracker_throttle.go` | Added Solana throttle policy (1 concurrent, 200ms spacing) |
| `internal/app/external_trackers.go` | Batched Solana getTransaction and NodeReal nr_getTransactionDetail calls; removed old per-call functions |
| `internal/app/external_trackers_test.go` | Updated NodeReal mock to handle batch JSON-RPC; added `io` import |

## In Progress

None - session complete.

## Next Steps

- [ ] Run a live graph query and verify ETH addresses are discovered with the batch Solana and increased Midgard timeout
- [ ] Monitor Solana 429 rate with new throttle policy — may need to increase spacing if still hitting limits
- [ ] Consider adding retry with backoff for Solana batch requests on 429
- [ ] Verify the 4 long thor contract addresses that were timing out at 5s now succeed at 10s
