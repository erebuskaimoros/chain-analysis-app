package app

import (
	"context"
	"strings"
	"sync"
	"time"
)

type trackerThrottleState struct {
	sem         chan struct{}
	mu          sync.Mutex
	lastRequest time.Time
}

type trackerThrottleStore struct {
	mu     sync.Mutex
	states map[string]*trackerThrottleState
}

func newTrackerThrottleStore() *trackerThrottleStore {
	return &trackerThrottleStore{
		states: map[string]*trackerThrottleState{},
	}
}

func trackerThrottlePolicy(provider, chain string) (int, time.Duration) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	switch {
	case provider == "nodereal" && chain == "BSC":
		return 1, 350 * time.Millisecond
	case provider == "etherscan":
		// Keep Etherscan requests below free-tier burst limits to reduce
		// "result is string" NOTOK payloads that drop transfer ingestion.
		return 1, 250 * time.Millisecond
	case provider == "trongrid" && chain == "TRON":
		// Trongrid can enforce 1 req/s on some tiers; keep a buffer to avoid 429 suspension.
		return 1, 1100 * time.Millisecond
	case provider == "solana":
		// Solana public RPC enforces strict per-second limits; serialize and space requests.
		return 1, 200 * time.Millisecond
	default:
		return 0, 0
	}
}

func (s *trackerThrottleStore) acquire(ctx context.Context, provider, chain string) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	concurrency, minSpacing := trackerThrottlePolicy(provider, chain)
	if concurrency < 1 {
		return func() {}, nil
	}

	key := trackerHealthKey(provider, chain)
	if key == "" {
		return func() {}, nil
	}

	s.mu.Lock()
	state, ok := s.states[key]
	if !ok {
		state = &trackerThrottleState{
			sem: make(chan struct{}, concurrency),
		}
		s.states[key] = state
	}
	s.mu.Unlock()

	select {
	case state.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	release := func() {
		select {
		case <-state.sem:
		default:
		}
	}

	state.mu.Lock()
	wait := time.Until(state.lastRequest.Add(minSpacing))
	state.mu.Unlock()
	if wait > 0 && !sleepWithContext(ctx, wait) {
		release()
		return nil, ctx.Err()
	}

	state.mu.Lock()
	state.lastRequest = time.Now().UTC()
	state.mu.Unlock()
	return release, nil
}
