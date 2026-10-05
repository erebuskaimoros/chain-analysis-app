# Session 2 - Phase 5: Address Labels

> Date: 2026-10-05
> Focus: Refactor plan Phase 5

## Summary

- **Storage:** labels attribute addresses to entities, each with a category, a source and a TagPack confidence (`labels` / `label_sources`, migration 8).
- **Precedence:** user annotations, then built-in labels, then imported labels by confidence.
- **Built-in labels:** moved from a hardcoded map to the embedded TagPack `internal/app/labels/builtin.yaml`, seeded at startup.
- **Imports:** run with `chain-analysis-server labels import <source> <path>`; each re-import replaces that source's labels.
- **Graph nodes** carry `label_category`, `label_source`, `label_confidence` and the top labels.
- **UI:** category badges on hover cards, a label list in the inspector, and a "Labeled Entities Shown" category filter.
- **API:** `GET /api/v1/labels?address=` and `GET /api/v1/labels/sources`.

## Real-data acceptance

Imported into a copy of the real database:

| Source | Labels | Import time |
|---|---|---|
| GraphSense TagPacks (79 packs) | 509,098 | 13 s |
| eth-labels | 105,765 | 3 s |
| ScamSniffer | 2,530 | <1 s |
| OFAC | 1,047 | <1 s |

- Re-importing OFAC gave the same count, so imports are idempotent.
- An OFAC-listed ETH address (ending e54A) shows as "OFAC SDN listed" / sanctioned in the Address Explorer. GraphSense independently names the sanctioned entity, and two counterparties resolve as exchanges.
- Goldens: only one treasury node gained label metadata (a built-in label); edges and actions are unchanged.

## Notes

- Downloaded source data lives under `data/labels/` (gitignored). ScamSniffer data is GPL-3.0, so it is imported locally and never committed.
- Category matching for "miner" was tightened (the substring `min` also matched "criminal"/"admin"). GraphSense genuinely tags about 38k mining addresses.
