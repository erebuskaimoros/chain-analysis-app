package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type migration struct {
	id   int
	name string
	up   func(context.Context, *sql.Tx) error
}

var migrations = []migration{
	{id: 1, name: "base_schema", up: migrateBaseSchema},
	{id: 2, name: "graph_runs_compat", up: migrateGraphRunsCompat},
	{id: 3, name: "analysis_runs", up: migrateAnalysisRuns},
}

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return err
	}

	for _, item := range migrations {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = ? LIMIT 1`, item.id).Scan(&exists); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return err
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := item.up(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %03d_%s: %w", item.id, item.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(id, name) VALUES (?, ?)`, item.id, item.name); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type execQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func migrateBaseSchema(ctx context.Context, db *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS rebond_links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			height INTEGER NOT NULL,
			tx_id TEXT,
			node_address TEXT,
			old_bond_address TEXT NOT NULL,
			new_bond_address TEXT NOT NULL,
			data_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(height, tx_id, node_address, old_bond_address, new_bond_address)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rebond_old ON rebond_links(old_bond_address)`,
		`CREATE INDEX IF NOT EXISTS idx_rebond_new ON rebond_links(new_bond_address)`,
		`CREATE TABLE IF NOT EXISTS actors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			color TEXT NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS actor_addresses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_id INTEGER NOT NULL,
			address TEXT NOT NULL,
			normalized_address TEXT NOT NULL,
			chain_hint TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(actor_id, normalized_address),
			FOREIGN KEY(actor_id) REFERENCES actors(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_actor_addresses_normalized ON actor_addresses(normalized_address)`,
		`CREATE INDEX IF NOT EXISTS idx_actor_addresses_actor ON actor_addresses(actor_id)`,
		`CREATE TABLE IF NOT EXISTS midgard_action_cache (
			address TEXT NOT NULL,
			start_ts INTEGER NOT NULL,
			end_ts INTEGER NOT NULL,
			max_pages INTEGER NOT NULL,
			truncated INTEGER NOT NULL DEFAULT 0,
			actions_json TEXT NOT NULL,
			action_count INTEGER NOT NULL DEFAULT 0,
			cached_at TEXT NOT NULL,
			PRIMARY KEY (address, start_ts, end_ts)
		)`,
		`CREATE TABLE IF NOT EXISTS external_transfer_cache (
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
		)`,
		`CREATE TABLE IF NOT EXISTS address_annotations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL,
			normalized_address TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			value TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_address_annotations_addr_kind
			ON address_annotations(normalized_address, kind)`,
		`CREATE TABLE IF NOT EXISTS graph_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_type TEXT NOT NULL DEFAULT 'actor_tracker',
			request_json TEXT NOT NULL,
			actor_names TEXT NOT NULL DEFAULT '',
			node_count INTEGER NOT NULL DEFAULT 0,
			edge_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS address_blocklist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL,
			normalized_address TEXT NOT NULL UNIQUE,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateGraphRunsCompat(ctx context.Context, db *sql.Tx) error {
	columns, err := tableColumnSet(ctx, db, "graph_runs")
	if err != nil {
		return err
	}
	if _, ok := columns["run_type"]; !ok {
		if _, err := db.ExecContext(ctx, `ALTER TABLE graph_runs ADD COLUMN run_type TEXT NOT NULL DEFAULT 'actor_tracker'`); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE graph_runs SET run_type = 'actor_tracker' WHERE TRIM(run_type) = ''`); err != nil {
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

func migrateAnalysisRuns(ctx context.Context, db *sql.Tx) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS analysis_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			legacy_graph_run_id INTEGER UNIQUE,
			run_type TEXT NOT NULL,
			request_json TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			node_count INTEGER NOT NULL DEFAULT 0,
			edge_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_analysis_runs_type_created
		ON analysis_runs(run_type, created_at DESC)
	`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_runs(legacy_graph_run_id, run_type, request_json, summary, node_count, edge_count, created_at)
		SELECT
			id,
			COALESCE(NULLIF(TRIM(run_type), ''), 'actor_tracker'),
			request_json,
			actor_names,
			node_count,
			edge_count,
			created_at
		FROM graph_runs
		WHERE NOT EXISTS (
			SELECT 1
			FROM analysis_runs ar
			WHERE ar.legacy_graph_run_id = graph_runs.id
		)
	`); err != nil {
		return err
	}
	return nil
}

func tableColumnSet(ctx context.Context, db execQuerier, table string) (map[string]struct{}, error) {
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
