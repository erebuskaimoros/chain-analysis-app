package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const schemaSQL = `
DROP TABLE IF EXISTS blocks;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS action_details;
DROP TABLE IF EXISTS ingest_state;

CREATE TABLE IF NOT EXISTS rebond_links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	height INTEGER NOT NULL,
	tx_id TEXT,
	node_address TEXT,
	old_bond_address TEXT NOT NULL,
	new_bond_address TEXT NOT NULL,
	data_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(height, tx_id, node_address, old_bond_address, new_bond_address)
);

CREATE INDEX IF NOT EXISTS idx_rebond_old ON rebond_links(old_bond_address);
CREATE INDEX IF NOT EXISTS idx_rebond_new ON rebond_links(new_bond_address);

CREATE TABLE IF NOT EXISTS actors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	color TEXT NOT NULL,
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS actor_addresses (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	actor_id INTEGER NOT NULL,
	address TEXT NOT NULL,
	normalized_address TEXT NOT NULL,
	chain_hint TEXT NOT NULL DEFAULT '',
	label TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE(actor_id, normalized_address),
	FOREIGN KEY(actor_id) REFERENCES actors(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_actor_addresses_normalized ON actor_addresses(normalized_address);
CREATE INDEX IF NOT EXISTS idx_actor_addresses_actor ON actor_addresses(actor_id);

CREATE TABLE IF NOT EXISTS midgard_action_cache (
	address TEXT NOT NULL,
	start_ts INTEGER NOT NULL,
	end_ts INTEGER NOT NULL,
	max_pages INTEGER NOT NULL,
	truncated INTEGER NOT NULL DEFAULT 0,
	actions_json TEXT NOT NULL,
	action_count INTEGER NOT NULL DEFAULT 0,
	cached_at TEXT NOT NULL,
	PRIMARY KEY (address, start_ts, end_ts)
);

CREATE TABLE IF NOT EXISTS external_transfer_cache (
	provider TEXT NOT NULL,
	chain TEXT NOT NULL,
	address TEXT NOT NULL,
	start_ts INTEGER NOT NULL,
	end_ts INTEGER NOT NULL,
	max_pages INTEGER NOT NULL,
	truncated INTEGER NOT NULL DEFAULT 0,
	transfers_json TEXT NOT NULL,
	transfer_count INTEGER NOT NULL DEFAULT 0,
	cached_at TEXT NOT NULL,
	PRIMARY KEY (provider, chain, address, start_ts, end_ts)
);

CREATE TABLE IF NOT EXISTS address_annotations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	address TEXT NOT NULL,
	normalized_address TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT '',
	value TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_address_annotations_addr_kind
	ON address_annotations(normalized_address, kind);

CREATE TABLE IF NOT EXISTS graph_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	request_json TEXT NOT NULL,
	actor_names TEXT NOT NULL DEFAULT '',
	node_count INTEGER NOT NULL DEFAULT 0,
	edge_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS address_blocklist (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	address TEXT NOT NULL,
	normalized_address TEXT NOT NULL UNIQUE,
	reason TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`

func initSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	return nil
}

func insertRebondLink(ctx context.Context, execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, link RebondLink) error {
	raw, err := json.Marshal(link.Data)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO rebond_links(
			height, tx_id, node_address, old_bond_address, new_bond_address, data_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(height, tx_id, node_address, old_bond_address, new_bond_address) DO NOTHING
	`,
		link.Height,
		link.TxID,
		link.NodeAddress,
		link.OldBondAddress,
		link.NewBondAddress,
		string(raw),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func queryRebondLinksByAddress(ctx context.Context, db *sql.DB, address string, limit int) ([]RebondLink, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT height, tx_id, node_address, old_bond_address, new_bond_address, data_json
		FROM rebond_links
		WHERE old_bond_address = ? OR new_bond_address = ?
		ORDER BY height DESC
		LIMIT ?
	`, address, address, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRebondLinks(rows)
}

func queryRebondLinksForAddressSet(ctx context.Context, db *sql.DB, addresses []string, limit int) ([]RebondLink, error) {
	if len(addresses) == 0 {
		return nil, nil
	}
	where, args := buildInClause(addresses)
	where2, args2 := buildInClause(addresses)
	args = append(args, args2...)
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT height, tx_id, node_address, old_bond_address, new_bond_address, data_json
		FROM rebond_links
		WHERE old_bond_address IN (%s) OR new_bond_address IN (%s)
		ORDER BY height DESC
		LIMIT ?
	`, where, where2)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRebondLinks(rows)
}

func scanRebondLinks(rows *sql.Rows) ([]RebondLink, error) {
	var out []RebondLink
	for rows.Next() {
		var link RebondLink
		var raw string
		if err := rows.Scan(
			&link.Height,
			&link.TxID,
			&link.NodeAddress,
			&link.OldBondAddress,
			&link.NewBondAddress,
			&raw,
		); err != nil {
			return nil, err
		}
		link.Data = map[string]any{}
		_ = json.Unmarshal([]byte(raw), &link.Data)
		out = append(out, link)
	}
	return out, rows.Err()
}

func buildInClause(values []string) (string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, v := range values {
		placeholders = append(placeholders, "?")
		args = append(args, v)
	}
	return strings.Join(placeholders, ","), args
}

func lookupMidgardActionCache(ctx context.Context, db *sql.DB, address string, startTS, endTS int64, maxPages int) ([]midgardAction, bool, bool, error) {
	var actionsJSON string
	var truncated bool
	var cachedMaxPages int
	var cachedStartTS, cachedEndTS int64

	// Exact match.
	err := db.QueryRowContext(ctx, `
		SELECT actions_json, truncated, max_pages, start_ts, end_ts
		FROM midgard_action_cache
		WHERE address = ? AND start_ts = ? AND end_ts = ?
	`, address, startTS, endTS).Scan(&actionsJSON, &truncated, &cachedMaxPages, &cachedStartTS, &cachedEndTS)

	if err == sql.ErrNoRows || (err == nil && truncated && cachedMaxPages < maxPages) {
		// Superset match: a non-truncated entry whose range fully covers the request.
		supErr := db.QueryRowContext(ctx, `
			SELECT actions_json, truncated, max_pages, start_ts, end_ts
			FROM midgard_action_cache
			WHERE address = ? AND start_ts <= ? AND end_ts >= ? AND truncated = 0
			ORDER BY (end_ts - start_ts) ASC
			LIMIT 1
		`, address, startTS, endTS).Scan(&actionsJSON, &truncated, &cachedMaxPages, &cachedStartTS, &cachedEndTS)
		if supErr == sql.ErrNoRows {
			return nil, false, false, nil
		}
		if supErr != nil {
			return nil, false, false, supErr
		}
	} else if err != nil {
		return nil, false, false, err
	}

	var actions []midgardAction
	if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil {
		return nil, false, false, nil
	}

	// If serving from a superset range, filter actions to the requested window.
	if cachedStartTS < startTS || cachedEndTS > endTS {
		actions = filterMidgardActionsByTimeRange(actions, startTS, endTS)
	}

	return actions, truncated, true, nil
}

func insertMidgardActionCache(ctx context.Context, db *sql.DB, address string, startTS, endTS int64, maxPages int, truncated bool, actions []midgardAction) error {
	raw, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO midgard_action_cache(address, start_ts, end_ts, max_pages, truncated, actions_json, action_count, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(address, start_ts, end_ts) DO UPDATE SET
			max_pages = excluded.max_pages,
			truncated = excluded.truncated,
			actions_json = excluded.actions_json,
			action_count = excluded.action_count,
			cached_at = excluded.cached_at
		WHERE excluded.truncated = 0 OR excluded.max_pages > midgard_action_cache.max_pages
	`, address, startTS, endTS, maxPages, truncated, string(raw), len(actions), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func lookupExternalTransferCache(ctx context.Context, db *sql.DB, provider, chain, address string, startTS, endTS int64, maxPages int, allowSuperset bool) ([]externalTransfer, bool, bool, error) {
	var transfersJSON string
	var truncated bool
	var cachedMaxPages int
	var cachedStartTS, cachedEndTS int64

	err := db.QueryRowContext(ctx, `
		SELECT transfers_json, truncated, max_pages, start_ts, end_ts
		FROM external_transfer_cache
		WHERE provider = ? AND chain = ? AND address = ? AND start_ts = ? AND end_ts = ?
	`, provider, chain, address, startTS, endTS).Scan(&transfersJSON, &truncated, &cachedMaxPages, &cachedStartTS, &cachedEndTS)

	if (err == sql.ErrNoRows || (err == nil && truncated && cachedMaxPages < maxPages)) && allowSuperset {
		supErr := db.QueryRowContext(ctx, `
			SELECT transfers_json, truncated, max_pages, start_ts, end_ts
			FROM external_transfer_cache
			WHERE provider = ? AND chain = ? AND address = ? AND start_ts <= ? AND end_ts >= ? AND truncated = 0
			ORDER BY (end_ts - start_ts) ASC
			LIMIT 1
		`, provider, chain, address, startTS, endTS).Scan(&transfersJSON, &truncated, &cachedMaxPages, &cachedStartTS, &cachedEndTS)
		if supErr == sql.ErrNoRows {
			return nil, false, false, nil
		}
		if supErr != nil {
			return nil, false, false, supErr
		}
	} else if err != nil {
		return nil, false, false, err
	}

	var transfers []externalTransfer
	if err := json.Unmarshal([]byte(transfersJSON), &transfers); err != nil {
		return nil, false, false, nil
	}
	if cachedStartTS < startTS || cachedEndTS > endTS {
		transfers = filterExternalTransfersByTimeRange(transfers, startTS, endTS)
	}
	return transfers, truncated, true, nil
}

func insertExternalTransferCache(ctx context.Context, db *sql.DB, provider, chain, address string, startTS, endTS int64, maxPages int, truncated bool, transfers []externalTransfer) error {
	raw, err := json.Marshal(transfers)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO external_transfer_cache(provider, chain, address, start_ts, end_ts, max_pages, truncated, transfers_json, transfer_count, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, chain, address, start_ts, end_ts) DO UPDATE SET
			max_pages = excluded.max_pages,
			truncated = excluded.truncated,
			transfers_json = excluded.transfers_json,
			transfer_count = excluded.transfer_count,
			cached_at = excluded.cached_at
		WHERE excluded.truncated = 0 OR excluded.max_pages > external_transfer_cache.max_pages
	`, provider, chain, address, startTS, endTS, maxPages, truncated, string(raw), len(transfers), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func insertGraphRun(ctx context.Context, db *sql.DB, req ActorTrackerRequest, actorNames string, nodeCount, edgeCount int) (int64, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	result, err := db.ExecContext(ctx, `
		INSERT INTO graph_runs(request_json, actor_names, node_count, edge_count, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, string(raw), actorNames, nodeCount, edgeCount, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func listGraphRuns(ctx context.Context, db *sql.DB) ([]GraphRun, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, request_json, actor_names, node_count, edge_count, created_at
		FROM graph_runs
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GraphRun
	for rows.Next() {
		var run GraphRun
		var rawReq string
		var createdAt string
		if err := rows.Scan(&run.ID, &rawReq, &run.ActorNames, &run.NodeCount, &run.EdgeCount, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rawReq), &run.Request)
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, run)
	}
	return out, rows.Err()
}

func deleteGraphRun(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM graph_runs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
