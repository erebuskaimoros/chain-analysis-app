# Session 3 - Phase 6: Actor Monitoring

> Date: 2026-10-05
> Focus: Refactor plan Phase 6

## Summary

- **Storage:** migration 9 adds `actor_watch` (watch flag, `last_viewed_at`) and `actor_snapshots`. A snapshot holds the window, total USD, per-address holdings with asset lists, and a flow summary by counterparty.
- **Refresh (`RefreshActor`, job kind `actor_refresh`):**
  - Builds the actor's flows over the window since the previous snapshot; the first refresh covers the last 30 days and is the baseline.
  - The build uses max_hops 1 and skips its own live-holdings pass (`withoutBuildLiveHoldings`).
  - It then runs the server-side holdings job with `force=true`.
  - A per-actor lock stops refreshes from overlapping.
- **Changes since last look:**
  - Merged across all snapshots after `last_viewed_at` (or after the baseline if the actor was never viewed).
  - Holding deltas by address include addresses that emptied; asset deltas sum amounts and spot USD per asset.
  - Flows are merged by counterparty; first-seen counterparties are flagged against all earlier snapshots.
- **Scheduler:** `CHAIN_ANALYSIS_ACTOR_REFRESH_INTERVAL` (default off) refreshes watched actors in-process, started by `StartBackground` and stopped by `Close`.
- **API:**
  - `GET /api/v1/actors/{id}/monitor`
  - `PUT /api/v1/actors/{id}/watch`
  - `POST /api/v1/actors/{id}/viewed`
  - `POST /api/v1/jobs/actor-refresh`
- **UI:**
  - A Monitor button on each actor card opens a full-width panel with: watch toggle, Refresh now (with job progress), Mark as seen, and the headline total with its change.
  - A holdings-over-time line chart: one series, crosshair, tooltip, arrow-key navigation, and a table view.
  - Below the chart: asset and address change tables, new counterparties with label category badges, and flows by counterparty. A USD threshold highlights large moves.

## Real-data acceptance

Run against a copy of the real database on :8091, for TC Treasury (16 addresses):

| Refresh | Window | Result |
|---|---|---|
| 1 (baseline) | last 30 days | $32.43M holdings; 308 transactions, $1.19M in / $0.52M out across 12 counterparties |
| 2 | 3 minutes since #1 | Midgard requests only with `fromTimestamp` = previous window end minus 10 minutes; no new flows |
| 3 | since #2 | 3 snapshots in the series; since-last-look lists asset deltas (BTC +$80.7k, ETH −$4.0k, ...) and address deltas |

- Each refresh took about 2.5 minutes; most of that was the holdings job retrying the DOGE explorer (502s) until its retries ran out.
- Two treasury addresses (SOL ending LK5g, BCH ending skzw) have no chain hint and can't be inferred from the address, so their holdings stay unavailable. Setting a chain hint on those actor addresses would fix it.

## Notes

- Moves between snapshots taken minutes apart are price changes, since the asset deltas use spot USD.
- The browser check was skipped because the Claude in Chrome extension was not connected. Instead, the served `ActorsPage` chunk was checked for the panel's strings.
