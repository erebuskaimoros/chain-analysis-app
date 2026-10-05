# Session 4 - Phase 3: Provider Errors, Circuit Breakers, Jobs, Server-Side Holdings

> Date: 2026-10-04 to 2026-10-05
> Focus: Refactor plan Phase 3

## Summary

- **Typed provider errors** (rate limited, banned, transient, permanent, config, circuit open) are produced at the HTTP layer; error text is unchanged.
- **Circuit breakers in tracker health:** 15 minutes for bans, `Retry-After` for 429s, 1 minute after 3 transient failures. Helpers fail fast while a circuit is open, and THORNode/Midgard clients try healthy endpoints first.
- **Bond-index cache:** `/thorchain/nodes` is cached for 1 minute.
- **Job runner** replaces the progress-token registry:
  - build, expand, explorer, and live-holdings jobs
  - progress reporting and cancellation
  - partial results, serialized at publish time: the graph per hop, holdings per pass
  - per-job logs under `data/logs/runs/`
- **Live holdings run on the server.**
  - Snapshots are stored in `holdings_snapshots` (migration 6) and reused for 10 minutes.
  - Only retryable failures are retried, waiting out provider backoff.
  - Every node ends in a final state with `live_holdings_error_kind`.
- **The UI uses jobs** for builds, expansions, explorer builds, and live holdings. The client retry loop is gone.

## Real-data smoke test

Run on a copy of the real database: TC Treasury, run #79, then holdings for its 394 nodes.

| | Available | Retries exhausted | Time |
|---|---|---|---|
| First version | about 240 | 84–105 | — |
| After fixes | 308 | 28 | 381 s (2nd run with reused snapshots: 142 s) |

The fixes:
- Our own deadlines no longer open provider circuits. Etherscan throttle-queue timeouts had tripped the Etherscan circuit and cascaded into 565 skipped lookups.
- Jobs get a 90 s pass budget instead of the 10 s request budget.

Remaining failures all have reasons:
- 47 validator nodes without bond: `no_bond`
- 8 addresses on chains without a tracker (BNB, TERRA): `config`
- 3 BCH addresses where bchexplorer.cash returns a proof-of-work challenge: `banned`

## Security

Configured API keys appeared in log lines, because error messages embed request URLs and Etherscan keys travel in the query string. All structured log lines now redact registered secrets. Log files written before this change, under `data/logs/`, may still contain the Etherscan key.

## Not done

The browser check could not run: the Claude-in-Chrome extension was not connected. UI behavior is covered by the updated frontend tests, and the API flows the UI uses were verified live.
