package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// backfillLedgerFromQueryCaches moves rows from the retired query-window
// caches (midgard_action_cache, external_transfer_cache) into the ledger and
// drops those tables. It runs once per database: after the drop there is
// nothing left to move.
func backfillLedgerFromQueryCaches(ctx context.Context, db *sql.DB) error {
	if exists, err := sqliteTableExists(ctx, db, "midgard_action_cache"); err != nil {
		return err
	} else if exists {
		if err := backfillLedgerActions(ctx, db); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `DROP TABLE midgard_action_cache`); err != nil {
			return err
		}
	}
	if exists, err := sqliteTableExists(ctx, db, "external_transfer_cache"); err != nil {
		return err
	} else if exists {
		if err := backfillLedgerTransfers(ctx, db); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `DROP TABLE external_transfer_cache`); err != nil {
			return err
		}
	}
	return nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// legacyActionCacheSource maps a retired cache key to its ledger source and
// address. Keys without a prefix predate protocol prefixes and hold THOR
// Midgard history.
func legacyActionCacheSource(cacheKey string) (string, string) {
	switch {
	case strings.HasPrefix(cacheKey, mergedTHORActionCachePref):
		return ledgerMergedTHORSource(), strings.TrimPrefix(cacheKey, mergedTHORActionCachePref)
	case strings.HasPrefix(cacheKey, mayaActionCachePrefix):
		return ledgerActionSource(sourceProtocolMAYA), strings.TrimPrefix(cacheKey, mayaActionCachePrefix)
	case strings.HasPrefix(cacheKey, thorActionCachePrefix):
		return ledgerActionSource(sourceProtocolTHOR), strings.TrimPrefix(cacheKey, thorActionCachePrefix)
	default:
		return ledgerActionSource(sourceProtocolTHOR), cacheKey
	}
}

type legacyCacheRow struct {
	key       string
	provider  string
	chain     string
	startTS   int64
	endTS     int64
	truncated bool
	payload   string
	cachedAt  string
}

func backfillLedgerActions(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT address, start_ts, end_ts, truncated, actions_json, cached_at
		FROM midgard_action_cache ORDER BY cached_at ASC
	`)
	if err != nil {
		return err
	}
	var pending []legacyCacheRow
	for rows.Next() {
		var row legacyCacheRow
		if err := rows.Scan(&row.key, &row.startTS, &row.endTS, &row.truncated, &row.payload, &row.cachedAt); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range pending {
		var actions []midgardAction
		if err := json.Unmarshal([]byte(row.payload), &actions); err != nil {
			continue
		}
		source, address := legacyActionCacheSource(row.key)
		address = normalizeAddress(address)
		if address == "" {
			continue
		}
		if err := upsertLedgerActions(ctx, db, source, address, actions); err != nil {
			return err
		}
		from := row.startTS
		if row.truncated {
			from = oldestMidgardActionUnix(actions)
			if from == 0 {
				continue
			}
		}
		cachedAt := parseLegacyCacheTime(row.cachedAt)
		if err := markLedgerCovered(ctx, db, source, address, from, ledgerCoverageEnd(row.endTS, cachedAt), cachedAt); err != nil {
			return err
		}
	}
	return nil
}

func backfillLedgerTransfers(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT provider, chain, address, start_ts, end_ts, truncated, transfers_json, cached_at
		FROM external_transfer_cache ORDER BY cached_at ASC
	`)
	if err != nil {
		return err
	}
	var pending []legacyCacheRow
	for rows.Next() {
		var row legacyCacheRow
		if err := rows.Scan(&row.provider, &row.chain, &row.key, &row.startTS, &row.endTS, &row.truncated, &row.payload, &row.cachedAt); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range pending {
		var transfers []externalTransfer
		if err := json.Unmarshal([]byte(row.payload), &transfers); err != nil {
			continue
		}
		address := normalizeAddress(row.key)
		if address == "" {
			continue
		}
		source := ledgerTransferSource(strings.ToLower(strings.TrimSpace(row.provider)), strings.ToUpper(strings.TrimSpace(row.chain)))
		if err := upsertLedgerTransfers(ctx, db, source, address, dedupeExternalTransfers(transfers)); err != nil {
			return err
		}
		if row.truncated {
			continue
		}
		cachedAt := parseLegacyCacheTime(row.cachedAt)
		if err := markLedgerCovered(ctx, db, source, address, row.startTS, ledgerCoverageEnd(row.endTS, cachedAt), cachedAt); err != nil {
			return err
		}
	}
	return nil
}

func parseLegacyCacheTime(raw string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}
