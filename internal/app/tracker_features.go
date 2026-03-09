package app

import (
	"context"
	"strings"
	"sync"
)

type trackerFeatureStore struct {
	mu          sync.Mutex
	unsupported map[string]struct{}
}

func newTrackerFeatureStore() *trackerFeatureStore {
	return &trackerFeatureStore{
		unsupported: map[string]struct{}{},
	}
}

func trackerFeatureKey(provider, chain, feature string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	feature = strings.ToLower(strings.TrimSpace(feature))
	if provider == "" || chain == "" || feature == "" {
		return ""
	}
	return provider + "|" + chain + "|" + feature
}

func (s *trackerFeatureStore) isUnsupported(provider, chain, feature string) bool {
	if s == nil {
		return false
	}
	key := trackerFeatureKey(provider, chain, feature)
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.unsupported[key]
	return ok
}

func (s *trackerFeatureStore) markSupported(provider, chain, feature string) {
	if s == nil {
		return
	}
	key := trackerFeatureKey(provider, chain, feature)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.unsupported, key)
}

func (s *trackerFeatureStore) markUnsupported(provider, chain, feature string) {
	if s == nil {
		return
	}
	key := trackerFeatureKey(provider, chain, feature)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsupported[key] = struct{}{}
}

func (a *App) isTrackerFeatureUnsupportedFromContext(ctx context.Context, feature string) bool {
	if a == nil || a.trackerFeatures == nil {
		return false
	}
	meta, ok := trackerRequestMetaFromContext(ctx)
	if !ok {
		return false
	}
	return a.trackerFeatures.isUnsupported(meta.Provider, meta.Chain, feature)
}

func (a *App) markTrackerFeatureSupportedFromContext(ctx context.Context, feature string) {
	if a == nil || a.trackerFeatures == nil {
		return
	}
	meta, ok := trackerRequestMetaFromContext(ctx)
	if !ok {
		return
	}
	a.trackerFeatures.markSupported(meta.Provider, meta.Chain, feature)
}

func (a *App) markTrackerFeatureUnsupportedFromContext(ctx context.Context, feature string) {
	if a == nil || a.trackerFeatures == nil {
		return
	}
	meta, ok := trackerRequestMetaFromContext(ctx)
	if !ok {
		return
	}
	a.trackerFeatures.markUnsupported(meta.Provider, meta.Chain, feature)
}
