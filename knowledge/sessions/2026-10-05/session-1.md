# Session 1 - Phase 4: Prices at Transaction Time

> Date: 2026-10-05
> Focus: Refactor plan Phase 4

## Summary

Every projected flow is now valued at its transaction time. Price sources, in order:
1. Midgard pool history: hourly if an asset spans 3 days or fewer, daily otherwise.
2. RUNE price history.
3. A $1 peg for stablecoins.
4. DefiLlama, for EVM tokens without a pool.
5. Today's spot price, as a labelled fallback.

Prices are preloaded per frontier and cached permanently in `price_points` (migration 7).

- **New output fields:** edges, transactions, assets and supporting actions carry `usd_at_time` and `price_source`.
- **Swaps count once:** a transaction is valued by its input side.
- **Filter:** `min_usd` uses the value at transaction time, and unpriced flows are excluded unless they touch an actor or `include_unpriced` is set.
- **UI:**
  - Responses are normalised at the API boundary, so labels, weights and filters use at-time values.
  - The inspector shows the value at the time and today.
  - The min-USD field is labelled "at time", with an include-unpriced toggle.

## Acceptance

- **Bitget exploiter:** a 100 ETH batch is valued at $265,723 (target ≈$265k ±5%), priced from Midgard pool history.
  - The swap edge totals $7.41M at the time.
  - The old spot figure, $15.3M, double-counted both sides at today's ETH price.
- **TC Treasury run #79** (2018 → 2026-04, $10 minimum): $916M at transaction time vs $225M at spot, because RUNE traded far higher in 2021–2022.
  - Asset sources: RUNE history 1,733; stable peg 654; pool history 562; spot fallback 29; DefiLlama 4; unpriced 261 (kept where they touch actors).
- **Goldens:** re-recorded with price history; graph structure unchanged.

## Fixes found along the way

- A spam token's absurd raw amount overflowed to ±Inf, and `writeJSON` then returned an empty 200. Values are now kept finite, and the API returns a logged 500 on encoding failure.
- DefiLlama coin lists are sorted so requests are deterministic, for cassette replay.
- The golden canonicaliser sorts transaction assets by identity, since `edgeList` orders them by today's spot value.
- Recording in a worktree needs `.env` (API keys) present; without it, ETH flows disappear.
