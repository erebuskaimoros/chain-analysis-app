package app

import (
	"fmt"
	"strings"
	"sync"
)

type trackerBlockNumberStore struct {
	mu     sync.Mutex
	values map[string]int64
}

func newTrackerBlockNumberStore() *trackerBlockNumberStore {
	return &trackerBlockNumberStore{
		values: map[string]int64{},
	}
}

func trackerBlockNumberKey(provider, chain, closest string, timestamp int64) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	closest = strings.ToLower(strings.TrimSpace(closest))
	if provider == "" || chain == "" || closest == "" || timestamp <= 0 {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%d", provider, chain, closest, timestamp)
}

func (s *trackerBlockNumberStore) get(provider, chain, closest string, timestamp int64) (int64, bool) {
	if s == nil {
		return 0, false
	}
	key := trackerBlockNumberKey(provider, chain, closest, timestamp)
	if key == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok
}

func (s *trackerBlockNumberStore) set(provider, chain, closest string, timestamp, block int64) {
	if s == nil || block <= 0 {
		return
	}
	key := trackerBlockNumberKey(provider, chain, closest, timestamp)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = block
}
