package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TraceRun is a saved trace: its request, a summary for listings, and (when
// loaded by ID) the full result.
type TraceRun struct {
	ID        int64          `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Title     string         `json:"title"`
	Direction string         `json:"direction"`
	Policy    string         `json:"policy"`
	Request   TraceRequest   `json:"request"`
	Summary   TraceSummary   `json:"summary"`
	Response  *TraceResponse `json:"response,omitempty"`
}

type TraceSummary struct {
	Seeds       []TraceSeed `json:"seeds"`
	SeedUSD     float64     `json:"seed_usd"`
	SinkUSD     float64     `json:"sink_usd"`
	FrontierUSD float64     `json:"frontier_usd"`
	Sinks       int         `json:"sinks"`
	Frontier    int         `json:"frontier"`
	Edges       int         `json:"edges"`
	TopSink     string      `json:"top_sink,omitempty"`
}

func traceRunTitle(resp TraceResponse) string {
	var seeds []string
	for _, seed := range resp.Query.Seeds {
		seeds = append(seeds, shortAddress(seed.Address))
	}
	verb := "Forward"
	if resp.Query.Direction == TraceBackward {
		verb = "Backward"
	}
	return fmt.Sprintf("%s %s trace from %s", verb, strings.ToUpper(resp.Query.Policy), strings.Join(seeds, ", "))
}

func saveTraceRun(ctx context.Context, db *sql.DB, req TraceRequest, resp TraceResponse) (int64, error) {
	summary := TraceSummary{
		Seeds:       resp.Query.Seeds,
		SeedUSD:     resp.Totals.SeedUSD,
		SinkUSD:     resp.Totals.SinkUSD,
		FrontierUSD: resp.Totals.FrontierUSD,
		Sinks:       len(resp.Sinks),
		Frontier:    len(resp.Frontier),
		Edges:       len(resp.Edges),
	}
	if len(resp.Sinks) > 0 {
		summary.TopSink = resp.Sinks[0].Label
	}
	request, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return 0, err
	}
	response, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO trace_runs(created_at, title, direction, policy, request_json, summary_json, response_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, time.Now().UTC().Format(time.RFC3339Nano), traceRunTitle(resp), resp.Query.Direction, resp.Query.Policy,
		string(request), string(summaryJSON), string(response))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func listTraceRuns(ctx context.Context, db *sql.DB, limit int) ([]TraceRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, created_at, title, direction, policy, request_json, summary_json
		FROM trace_runs ORDER BY created_at DESC, id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []TraceRun{}
	for rows.Next() {
		run, err := scanTraceRun(rows, false)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func getTraceRun(ctx context.Context, db *sql.DB, id int64) (TraceRun, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, created_at, title, direction, policy, request_json, summary_json, response_json
		FROM trace_runs WHERE id = ?
	`, id)
	return scanTraceRun(row, true)
}

func deleteTraceRun(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM trace_runs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type traceRunScanner interface {
	Scan(dest ...any) error
}

func scanTraceRun(row traceRunScanner, withResponse bool) (TraceRun, error) {
	var run TraceRun
	var createdAt, request, summary, response string
	dest := []any{&run.ID, &createdAt, &run.Title, &run.Direction, &run.Policy, &request, &summary}
	if withResponse {
		dest = append(dest, &response)
	}
	if err := row.Scan(dest...); err != nil {
		return TraceRun{}, err
	}
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if err := json.Unmarshal([]byte(request), &run.Request); err != nil {
		return TraceRun{}, fmt.Errorf("decode trace request: %w", err)
	}
	if err := json.Unmarshal([]byte(summary), &run.Summary); err != nil {
		return TraceRun{}, fmt.Errorf("decode trace summary: %w", err)
	}
	if withResponse {
		var resp TraceResponse
		if err := json.Unmarshal([]byte(response), &resp); err != nil {
			return TraceRun{}, fmt.Errorf("decode trace response: %w", err)
		}
		resp.RunID = run.ID
		run.Response = &resp
	}
	return run, nil
}

// ValidateTraceRequest reports request errors before a trace job starts.
func (a *App) ValidateTraceRequest(req TraceRequest) error {
	if _, err := normalizeTraceRequest(req); err != nil {
		return fmt.Errorf("invalid trace request: %w", err)
	}
	return nil
}

// StartTraceJob runs a trace in the background and saves the result.
func (a *App) StartTraceJob(req TraceRequest) JobSnapshot {
	return a.jobs.start(JobTrace, analysisJobTimeout, "", func(ctx context.Context) (any, error) {
		resp, err := a.Trace(ctx, req)
		if err != nil {
			return nil, err
		}
		id, err := saveTraceRun(ctx, a.db, req, resp)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "trace was not saved: "+err.Error())
		}
		resp.RunID = id
		return resp, nil
	})
}

func (a *App) ListTraceRuns(ctx context.Context) ([]TraceRun, error) {
	return listTraceRuns(ctx, a.db, 50)
}

func (a *App) GetTraceRun(ctx context.Context, id int64) (TraceRun, error) {
	return getTraceRun(ctx, a.db, id)
}

func (a *App) DeleteTraceRun(ctx context.Context, id int64) error {
	return deleteTraceRun(ctx, a.db, id)
}
