package app

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type trackerHealthState struct {
	Provider            string
	Chain               string
	LastSuccessAt       time.Time
	LastErrorAt         time.Time
	LastError           string
	ConsecutiveFailures int
	LastStatusCode      int
	RetryAfter          string
	RateLimitRemaining  string
	RateLimitReset      string
	CacheHits           int64
	CacheMisses         int64
	Requests            int64
	Successes           int64
	Failures            int64
}

type trackerHealthStore struct {
	mu     sync.Mutex
	states map[string]*trackerHealthState
}

func newTrackerHealthStore() *trackerHealthStore {
	return &trackerHealthStore{
		states: map[string]*trackerHealthState{},
	}
}

func trackerHealthKey(provider, chain string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if provider == "" || chain == "" {
		return ""
	}
	return provider + "|" + chain
}

func (s *trackerHealthStore) state(provider, chain string) *trackerHealthState {
	if s == nil {
		return nil
	}
	key := trackerHealthKey(provider, chain)
	if key == "" {
		return nil
	}
	if existing, ok := s.states[key]; ok {
		return existing
	}
	state := &trackerHealthState{
		Provider: strings.ToLower(strings.TrimSpace(provider)),
		Chain:    strings.ToUpper(strings.TrimSpace(chain)),
	}
	s.states[key] = state
	return state
}

func (s *trackerHealthStore) markCache(provider, chain string, hit bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state(provider, chain)
	if state == nil {
		return
	}
	if hit {
		state.CacheHits++
		return
	}
	state.CacheMisses++
}

func (s *trackerHealthStore) recordAttempt(provider, chain string, statusCode int, headers http.Header, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state(provider, chain)
	if state == nil {
		return
	}
	now := time.Now().UTC()
	state.Requests++
	if statusCode > 0 {
		state.LastStatusCode = statusCode
	}
	if headers != nil {
		state.RetryAfter = firstNonEmpty(
			strings.TrimSpace(headers.Get("Retry-After")),
			state.RetryAfter,
		)
		state.RateLimitRemaining = firstNonEmpty(
			strings.TrimSpace(headers.Get("X-RateLimit-Remaining")),
			strings.TrimSpace(headers.Get("RateLimit-Remaining")),
			state.RateLimitRemaining,
		)
		state.RateLimitReset = firstNonEmpty(
			strings.TrimSpace(headers.Get("X-RateLimit-Reset")),
			strings.TrimSpace(headers.Get("RateLimit-Reset")),
			state.RateLimitReset,
		)
	}
	if err != nil || statusCode >= http.StatusBadRequest {
		state.Failures++
		state.LastErrorAt = now
		if err != nil {
			state.LastError = err.Error()
		}
		state.ConsecutiveFailures++
		return
	}
	state.Successes++
	state.LastSuccessAt = now
	state.LastError = ""
	state.ConsecutiveFailures = 0
}

func (s *trackerHealthStore) snapshot() map[string]any {
	if s == nil {
		return map[string]any{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for key, state := range s.states {
		entry := map[string]any{
			"provider":             state.Provider,
			"chain":                state.Chain,
			"degraded":             s.isStateDegraded(state),
			"consecutive_failures": state.ConsecutiveFailures,
			"last_status_code":     state.LastStatusCode,
			"retry_after":          state.RetryAfter,
			"rate_limit_remaining": state.RateLimitRemaining,
			"rate_limit_reset":     state.RateLimitReset,
			"cache_hits":           state.CacheHits,
			"cache_misses":         state.CacheMisses,
			"requests":             state.Requests,
			"successes":            state.Successes,
			"failures":             state.Failures,
		}
		if !state.LastSuccessAt.IsZero() {
			entry["last_success_at"] = state.LastSuccessAt.Format(time.RFC3339)
		}
		if !state.LastErrorAt.IsZero() {
			entry["last_error_at"] = state.LastErrorAt.Format(time.RFC3339)
		}
		if state.LastError != "" {
			entry["last_error"] = trimForLog(state.LastError, 200)
		}
		out[key] = entry
	}
	return out
}

func (s *trackerHealthStore) isDegraded(provider, chain string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isStateDegraded(s.state(provider, chain))
}

func (s *trackerHealthStore) isStateDegraded(state *trackerHealthState) bool {
	if state == nil {
		return false
	}
	if state.ConsecutiveFailures < 3 {
		return false
	}
	if state.LastErrorAt.IsZero() {
		return false
	}
	return time.Since(state.LastErrorAt) < 5*time.Minute
}

type trackerRequestMeta struct {
	Provider string
	Chain    string
}

type trackerRequestMetaContextKey struct{}

func withTrackerRequestMeta(ctx context.Context, provider, chain string) context.Context {
	meta := trackerRequestMeta{
		Provider: strings.ToLower(strings.TrimSpace(provider)),
		Chain:    strings.ToUpper(strings.TrimSpace(chain)),
	}
	if meta.Provider == "" || meta.Chain == "" {
		return ctx
	}
	return context.WithValue(ctx, trackerRequestMetaContextKey{}, meta)
}

func trackerRequestMetaFromContext(ctx context.Context) (trackerRequestMeta, bool) {
	meta, ok := ctx.Value(trackerRequestMetaContextKey{}).(trackerRequestMeta)
	if !ok || meta.Provider == "" || meta.Chain == "" {
		return trackerRequestMeta{}, false
	}
	return meta, true
}
