package app

import (
	"context"
	"sync"
	"time"
)

// BuildProgressSnapshot is what the progress polling endpoint returns while an
// actor-graph build is running.
type BuildProgressSnapshot struct {
	Token     string    `json:"token"`
	Stage     string    `json:"stage"`
	Done      int       `json:"done"`
	Total     int       `json:"total"`
	Message   string    `json:"message,omitempty"`
	Finished  bool      `json:"finished"`
	UpdatedAt time.Time `json:"updated_at"`
}

type buildProgress struct {
	mu   sync.Mutex
	snap BuildProgressSnapshot
}

func (p *buildProgress) set(stage string, done, total int, message string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap.Stage = stage
	p.snap.Done = done
	p.snap.Total = total
	p.snap.Message = message
	p.snap.UpdatedAt = time.Now().UTC()
}

func (p *buildProgress) finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap.Finished = true
	p.snap.UpdatedAt = time.Now().UTC()
}

func (p *buildProgress) snapshot() BuildProgressSnapshot {
	if p == nil {
		return BuildProgressSnapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap
}

const buildProgressRetention = 10 * time.Minute

type progressRegistry struct {
	mu      sync.Mutex
	entries map[string]*buildProgress
}

func newProgressRegistry() *progressRegistry {
	return &progressRegistry{entries: map[string]*buildProgress{}}
}

func (r *progressRegistry) start(token string) *buildProgress {
	if r == nil || token == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Evict finished entries older than the retention window so tokens from
	// abandoned polls don't accumulate.
	cutoff := time.Now().UTC().Add(-buildProgressRetention)
	for key, entry := range r.entries {
		snap := entry.snapshot()
		if snap.Finished && snap.UpdatedAt.Before(cutoff) {
			delete(r.entries, key)
		}
	}
	progress := &buildProgress{
		snap: BuildProgressSnapshot{
			Token:     token,
			Stage:     "starting",
			UpdatedAt: time.Now().UTC(),
		},
	}
	r.entries[token] = progress
	return progress
}

func (r *progressRegistry) lookup(token string) (BuildProgressSnapshot, bool) {
	if r == nil || token == "" {
		return BuildProgressSnapshot{}, false
	}
	r.mu.Lock()
	entry, ok := r.entries[token]
	r.mu.Unlock()
	if !ok {
		return BuildProgressSnapshot{}, false
	}
	return entry.snapshot(), true
}

type buildProgressCtxKey struct{}

func withBuildProgress(ctx context.Context, progress *buildProgress) context.Context {
	if progress == nil {
		return ctx
	}
	return context.WithValue(ctx, buildProgressCtxKey{}, progress)
}

// buildProgressFromContext returns nil when no progress is attached; all
// *buildProgress methods are nil-safe so callers can report unconditionally.
func buildProgressFromContext(ctx context.Context) *buildProgress {
	if ctx == nil {
		return nil
	}
	progress, _ := ctx.Value(buildProgressCtxKey{}).(*buildProgress)
	return progress
}
