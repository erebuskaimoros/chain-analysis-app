package app

import (
	"context"
	"sync"
	"time"
)

const (
	protocolDirectoryCacheTTL = time.Minute
	priceBookCacheTTL         = 15 * time.Second
)

type cachedMetadataResult[T any] struct {
	once sync.Once

	mu        sync.Mutex
	ttl       time.Duration
	clone     func(T) T
	ready     bool
	value     T
	expiresAt time.Time
	inflight  chan struct{}
}

func (c *cachedMetadataResult[T]) init(ttl time.Duration, clone func(T) T) {
	c.once.Do(func() {
		c.ttl = ttl
		c.clone = clone
	})
}

func (c *cachedMetadataResult[T]) get(ctx context.Context, load func(context.Context) (T, error)) (T, error) {
	for {
		now := time.Now()

		c.mu.Lock()
		if c.ready && now.Before(c.expiresAt) {
			value := c.cloneValue(c.value)
			c.mu.Unlock()
			return value, nil
		}
		if wait := c.inflight; wait != nil {
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				var zero T
				return zero, ctx.Err()
			}
		}

		wait := make(chan struct{})
		c.inflight = wait
		c.mu.Unlock()

		value, err := load(ctx)

		c.mu.Lock()
		if err == nil {
			c.value = c.cloneValue(value)
			c.expiresAt = time.Now().Add(c.ttl)
			c.ready = true
		}
		c.inflight = nil
		close(wait)
		c.mu.Unlock()

		if err != nil {
			return c.cloneValue(value), err
		}
		return c.cloneValue(value), nil
	}
}

func (c *cachedMetadataResult[T]) cloneValue(value T) T {
	if c.clone == nil {
		return value
	}
	return c.clone(value)
}

func cloneProtocolDirectory(src protocolDirectory) protocolDirectory {
	out := protocolDirectory{
		AddressKinds:    make(map[string]protocolAddress, len(src.AddressKinds)),
		SupportedChains: make(map[string]struct{}, len(src.SupportedChains)),
	}
	for key, value := range src.AddressKinds {
		out.AddressKinds[key] = value
	}
	for key, value := range src.SupportedChains {
		out.SupportedChains[key] = value
	}
	return out
}

func clonePriceBook(src priceBook) priceBook {
	out := priceBook{
		NativeUSD:     make(map[string]float64, len(src.NativeUSD)),
		AssetUSD:      make(map[string]float64, len(src.AssetUSD)),
		PoolAssets:    make(map[string]struct{}, len(src.PoolAssets)),
		PoolSnapshots: make(map[string]MidgardPool, len(src.PoolSnapshots)),
		PoolProtocols: make(map[string]string, len(src.PoolProtocols)),
		HasPoolData:   src.HasPoolData,
	}
	for key, value := range src.NativeUSD {
		out.NativeUSD[key] = value
	}
	for key, value := range src.AssetUSD {
		out.AssetUSD[key] = value
	}
	for key, value := range src.PoolAssets {
		out.PoolAssets[key] = value
	}
	for key, value := range src.PoolSnapshots {
		out.PoolSnapshots[key] = value
	}
	for key, value := range src.PoolProtocols {
		out.PoolProtocols[key] = value
	}
	return out
}
