package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Jobs run long analyses (graph builds, expansions, live-holdings refreshes)
// in the background so they do not depend on one HTTP request surviving.
// Clients poll a job for progress, partial results, and the final result.

type JobKind string

const (
	JobActorGraphBuild  JobKind = "actor_graph_build"
	JobActorGraphExpand JobKind = "actor_graph_expand"
	JobAddressExplorer  JobKind = "address_explorer_build"
	JobLiveHoldings     JobKind = "live_holdings"
)

type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

type JobSnapshot struct {
	ID         string     `json:"id"`
	Kind       JobKind    `json:"kind"`
	Status     JobStatus  `json:"status"`
	Stage      string     `json:"stage,omitempty"`
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	Message    string     `json:"message,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	LogPath    string     `json:"log_path,omitempty"`
	// PartialCounts summarises the latest partial result (for example nodes
	// and edges so far) so polls stay small; Partial carries the full value
	// and is only returned on request.
	PartialCounts map[string]int  `json:"partial_counts,omitempty"`
	Partial       json.RawMessage `json:"partial,omitempty"`
	Result        any             `json:"result,omitempty"`
}

const jobRetention = 30 * time.Minute

type job struct {
	mu       sync.Mutex
	snap     JobSnapshot
	progress *buildProgress
	cancel   context.CancelFunc
}

func (j *job) snapshot() JobSnapshot {
	j.mu.Lock()
	snap := j.snap
	j.mu.Unlock()
	if snap.Status == JobRunning && j.progress != nil {
		progress := j.progress.snapshot()
		snap.Stage = progress.Stage
		snap.Done = progress.Done
		snap.Total = progress.Total
		snap.Message = progress.Message
		if progress.UpdatedAt.After(snap.UpdatedAt) {
			snap.UpdatedAt = progress.UpdatedAt
		}
	}
	return snap
}

func (j *job) setPartial(counts map[string]int, raw json.RawMessage) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snap.PartialCounts = counts
	j.snap.Partial = raw
	j.snap.UpdatedAt = time.Now().UTC()
}

type jobPartialCtxKey struct{}

// publishJobPartial hands the running job a partial result. build is only
// called when a job is listening, so callers can pass expensive snapshots. The
// value is serialized immediately so readers never share the caller's live
// state.
func publishJobPartial(ctx context.Context, build func() (any, map[string]int)) {
	if ctx == nil {
		return
	}
	j, ok := ctx.Value(jobPartialCtxKey{}).(*job)
	if !ok || j == nil {
		return
	}
	value, counts := build()
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	j.setPartial(counts, raw)
}

type jobRunner struct {
	mu     sync.Mutex
	jobs   map[string]*job
	logDir string
}

func newJobRunner(logDir string) *jobRunner {
	return &jobRunner{jobs: map[string]*job{}, logDir: strings.TrimSpace(logDir)}
}

// jobFunc does the work; it returns the result and the captured run log is
// written to the job's log file. lastRunLogPath, when set, also receives it.
type jobFunc func(ctx context.Context) (any, error)

func (r *jobRunner) start(kind JobKind, timeout time.Duration, lastRunLogPath string, fn jobFunc) JobSnapshot {
	now := time.Now().UTC()
	id := newJobID()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	j := &job{
		snap: JobSnapshot{
			ID:        id,
			Kind:      kind,
			Status:    JobRunning,
			StartedAt: now,
			UpdatedAt: now,
		},
		progress: &buildProgress{snap: BuildProgressSnapshot{Token: id, Stage: "starting", UpdatedAt: now}},
		cancel:   cancel,
	}
	if r.logDir != "" {
		j.snap.LogPath = filepath.Join(r.logDir, id+".jsonl")
	}

	r.mu.Lock()
	r.evictLocked(now)
	r.jobs[id] = j
	r.mu.Unlock()

	go func() {
		defer cancel()
		capturedCtx, capture := withRunLogCapture(ctx)
		runCtx := context.WithValue(withBuildProgress(capturedCtx, j.progress), jobPartialCtxKey{}, j)
		result, err := fn(runCtx)

		lines := capture.snapshot()
		if j.snap.LogPath != "" {
			writeRunLog(j.snap.LogPath, lines)
		}
		if lastRunLogPath != "" {
			writeRunLog(lastRunLogPath, lines)
		}

		finished := time.Now().UTC()
		j.mu.Lock()
		defer j.mu.Unlock()
		j.snap.UpdatedAt = finished
		j.snap.FinishedAt = &finished
		j.snap.Stage = "done"
		switch {
		case err == nil:
			j.snap.Status = JobSucceeded
			j.snap.Result = result
			j.snap.Partial = nil
			j.snap.PartialCounts = nil
		case errors.Is(err, context.Canceled) && ctx.Err() == context.Canceled:
			j.snap.Status = JobCanceled
			j.snap.Error = "canceled"
		default:
			j.snap.Status = JobFailed
			j.snap.Error = err.Error()
		}
	}()
	return j.snapshot()
}

func (r *jobRunner) get(id string) (JobSnapshot, bool) {
	r.mu.Lock()
	j, ok := r.jobs[strings.TrimSpace(id)]
	r.mu.Unlock()
	if !ok {
		return JobSnapshot{}, false
	}
	return j.snapshot(), true
}

func (r *jobRunner) cancel(id string) bool {
	r.mu.Lock()
	j, ok := r.jobs[strings.TrimSpace(id)]
	r.mu.Unlock()
	if !ok {
		return false
	}
	j.cancel()
	return true
}

func (r *jobRunner) evictLocked(now time.Time) {
	cutoff := now.Add(-jobRetention)
	for id, j := range r.jobs {
		snap := j.snapshot()
		if snap.FinishedAt != nil && snap.FinishedAt.Before(cutoff) {
			delete(r.jobs, id)
		}
	}
}

func newJobID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(buf[:])
}

func writeRunLog(path string, lines []string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if len(lines) == 0 {
		lines = []string{`{"level":"info","event":"run_log_capture_empty"}`}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
