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
	run_type TEXT NOT NULL DEFAULT 'actor_tracker',
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
	if err := ensureGraphRunsSchema(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureGraphRunsSchema(ctx context.Context, db *sql.DB) error {
	columns, err := tableColumnSet(ctx, db, "graph_runs")
	if err != nil {
		return err
	}
	if _, ok := columns["run_type"]; !ok {
		if _, err := db.ExecContext(ctx, `ALTER TABLE graph_runs ADD COLUMN run_type TEXT NOT NULL DEFAULT 'actor_tracker'`); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE graph_runs SET run_type = ? WHERE TRIM(run_type) = ''`, GraphRunTypeActorTracker); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_graph_runs_type_created
		ON graph_runs(run_type, created_at DESC)
	`); err != nil {
		return err
	}
	return nil
}

func tableColumnSet(ctx context.Context, db *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string]struct{}{}
	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}
		columns[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return columns, rows.Err()
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

type storedGraphRun struct {
	ID        int64
	RunType   string
	Request   string
	Summary   string
	NodeCount int
	EdgeCount int
	CreatedAt time.Time
}

func insertTypedGraphRun(ctx context.Context, db *sql.DB, runType string, req any, summary string, nodeCount, edgeCount int) (int64, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	result, err := db.ExecContext(ctx, `
		INSERT INTO graph_runs(run_type, request_json, actor_names, node_count, edge_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, runType, string(raw), summary, nodeCount, edgeCount, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func insertGraphRun(ctx context.Context, db *sql.DB, req ActorTrackerRequest, actorNames string, nodeCount, edgeCount int) (int64, error) {
	return insertTypedGraphRun(ctx, db, GraphRunTypeActorTracker, req, actorNames, nodeCount, edgeCount)
}

func insertAddressExplorerRun(ctx context.Context, db *sql.DB, req AddressExplorerRequest, summary string, nodeCount, edgeCount int) (int64, error) {
	return insertTypedGraphRun(ctx, db, GraphRunTypeAddressExplorer, req, summary, nodeCount, edgeCount)
}

func listTypedGraphRuns(ctx context.Context, db *sql.DB, runType string) ([]storedGraphRun, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, run_type, request_json, actor_names, node_count, edge_count, created_at
		FROM graph_runs
		WHERE run_type = ?
		ORDER BY created_at DESC
	`, runType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []storedGraphRun
	for rows.Next() {
		var run storedGraphRun
		var createdAt string
		if err := rows.Scan(&run.ID, &run.RunType, &run.Request, &run.Summary, &run.NodeCount, &run.EdgeCount, &createdAt); err != nil {
			return nil, err
		}
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, run)
	}
	return out, rows.Err()
}

func listGraphRuns(ctx context.Context, db *sql.DB) ([]GraphRun, error) {
	stored, err := listTypedGraphRuns(ctx, db, GraphRunTypeActorTracker)
	if err != nil {
		return nil, err
	}
	out := make([]GraphRun, 0, len(stored))
	for _, item := range stored {
		run := GraphRun{
			ID:         item.ID,
			RunType:    item.RunType,
			ActorNames: item.Summary,
			NodeCount:  item.NodeCount,
			EdgeCount:  item.EdgeCount,
			CreatedAt:  item.CreatedAt,
		}
		_ = json.Unmarshal([]byte(item.Request), &run.Request)
		out = append(out, run)
	}
	return out, nil
}

func listAddressExplorerRuns(ctx context.Context, db *sql.DB) ([]AddressExplorerRun, error) {
	stored, err := listTypedGraphRuns(ctx, db, GraphRunTypeAddressExplorer)
	if err != nil {
		return nil, err
	}
	out := make([]AddressExplorerRun, 0, len(stored))
	for _, item := range stored {
		run := AddressExplorerRun{
			ID:        item.ID,
			RunType:   item.RunType,
			Summary:   item.Summary,
			NodeCount: item.NodeCount,
			EdgeCount: item.EdgeCount,
			CreatedAt: item.CreatedAt,
		}
		_ = json.Unmarshal([]byte(item.Request), &run.Request)
		out = append(out, run)
	}
	return out, nil
}

func deleteTypedGraphRun(ctx context.Context, db *sql.DB, id int64, runType string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM graph_runs WHERE id = ? AND run_type = ?`, id, runType)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func deleteGraphRun(ctx context.Context, db *sql.DB, id int64) error {
	return deleteTypedGraphRun(ctx, db, id, GraphRunTypeActorTracker)
}

func deleteAddressExplorerRun(ctx context.Context, db *sql.DB, id int64) error {
	return deleteTypedGraphRun(ctx, db, id, GraphRunTypeAddressExplorer)
}
