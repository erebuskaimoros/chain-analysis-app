package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateBackfillsAnalysisRunsFromLegacyGraphRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrations.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE graph_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_json TEXT NOT NULL,
			actor_names TEXT NOT NULL DEFAULT '',
			node_count INTEGER NOT NULL DEFAULT 0,
			edge_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy graph_runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO graph_runs(request_json, actor_names, node_count, edge_count, created_at)
		VALUES ('{"actor_ids":[1]}', 'Alpha', 2, 1, '2026-03-10T12:00:00Z')
	`); err != nil {
		t.Fatalf("insert legacy graph run: %v", err)
	}

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var (
		runType string
		summary string
	)
	if err := db.QueryRowContext(ctx, `SELECT run_type, summary FROM analysis_runs LIMIT 1`).Scan(&runType, &summary); err != nil {
		t.Fatalf("query analysis_runs: %v", err)
	}
	if runType != "actor_tracker" {
		t.Fatalf("unexpected run_type %q", runType)
	}
	if summary != "Alpha" {
		t.Fatalf("unexpected summary %q", summary)
	}
}
