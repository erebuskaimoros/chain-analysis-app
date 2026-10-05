# Session 4 - Phase 7: Follow-the-Funds Trace

> Date: 2026-10-05
> Focus: Refactor plan Phase 7

## Summary

- **Bug: rotated-vault legs drawn twice.** It was fixed test-first: the failing test came first, then a subagent fixed it.
  - `cleanTxID` kept the `0x` prefix of EVM hashes, so Ethereum transfers never matched Midgard legs.
  - Stitching only consumed transfers through vaults still listed in `inbound_addresses`.
  - Swap deposits to rotated vaults, and payouts from them, were drawn twice: inside the swap and as plain transfers.
  - The Bitget graph went from 118 to 33 edges (31 deposits and 17 BTC payouts removed). The treasury graph lost a rotated LTC vault's 30 deposits and 84 payouts.
- **Confidence with reasons.**
  - A leg that THORChain records scores 1. Lower scores are explained: inferred amounts 0.7, missing amounts 0.6, contract-call senders 0.85, CALC-routed payouts 0.8, UTXO senders 0.82 or lower.
  - Every edge carries `confidence_reason` and takes its weakest segment's confidence.
  - Swap transactions keep the deposit hash (`inbound_tx_id`) beside the payout hash.
- **Trace engine** (`trace.go`, `trace_result.go`):
  - Each hop is expanded with the one-hop actor expansion, so all of the ledger, stitching and pricing is reused.
  - A forward trace is a chronological lot simulation with three policies: fifo, haircut and largest_out.
  - A backward trace decomposes each payment into the receipts it spent.
  - Swaps convert the traced share. A swap payout that is also seen as a vault transfer is counted once.
  - Mixing traced with untraced funds lowers confidence by 15%.
- **Transaction seeds** follow that deposit's amount. Forward windows start just before it; backward windows end just before it.
- **Runs** are saved in `trace_runs` (migration 10).
- **API:**
  - `POST /api/v1/jobs/trace`, with alias `/api/v1/analysis/trace`
  - `GET /api/v1/traces`
  - `GET|DELETE /api/v1/traces/{id}`
- **UI:** a Trace page with the form, saved runs, summary tiles, method, graph, sinks and frontier tables (including current holdings), a flows table with explorer links for both swap hashes, and coverage gaps.
  - The dead "Open Legacy App" link was removed.

## Acceptance (case B, Bitget exploiter)

- **Recorded trace golden** (`testdata/trace_golden/bitget_exploiter`), which replays in 0.07 s:
  - Depth 1: the 31 swaps sit on one edge to BTC ending j68f with 87.823014 BTC, and every swap has both hashes.
  - Depth 3: j68f's traced BTC is either passed on (56.44 BTC to six addresses) or still held (31.38 BTC).
- **Live, uncached depth-3 trace:** 39 s. It gives the same endpoints as the replay. Current holdings show five of the BTC endpoints still holding about $3.6M.
- The exploiter also sent 119.34 ETH to a wallet ending 164c, which has no THORChain history. That wallet split it to two wallets, which swapped about 3.76 BTC more. So the BTC destination is not the only endpoint.
- **Transaction seed** (the first 1 ETH deposit):
  - Forward: the 0.031669 BTC goes via j68f to a BTC address ending mq6j.
  - Backward: the 1 ETH came from an address ending 63ee in the prior 30 days.

## Notes

- The plan quoted 27 swaps and 75.2 BTC from early reporting. On-chain, there are 31 swaps carrying 87.823 BTC.
- The ExplorerPage fullscreen test failed whenever run alone, on main as well. It clicked before the canvas rendered; it now awaits the button.
- The browser check was skipped again: the Claude in Chrome extension is not connected.
