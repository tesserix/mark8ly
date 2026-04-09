// services/marketplace-api/internal/stores/slug_cache.go
package stores

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/singleflight"
)

// SlugCache is a pull-through cache for storefront store-slug lookups.
// It wraps a Repository + Client + singleflight to give anonymous
// storefront traffic the same freshness guarantees as the admin path
// has via StoreMiddleware.
type SlugCache struct {
	repo   Repository
	client Client
	flight *singleflight.Group
	ttl    time.Duration
}

// NewSlugCache constructs a SlugCache. ttl defaults to 5 minutes when zero.
// flight may be nil; a fresh singleflight.Group is allocated in that case.
func NewSlugCache(repo Repository, client Client, flight *singleflight.Group, ttl time.Duration) *SlugCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	if flight == nil {
		flight = &singleflight.Group{}
	}
	return &SlugCache{repo: repo, client: client, flight: flight, ttl: ttl}
}

// Get returns the store row for the given slug. On miss or stale, it
// refreshes via the platform client (coalesced through singleflight) and
// upserts the projection table. Returns ErrNotFound when the store
// doesn't exist in either place. Falls back to stale-but-cached-within-
// 24h on platform-api errors.
func (c *SlugCache) Get(ctx context.Context, slug string) (*Store, error) {
	cached, cachedErr := c.repo.GetBySlug(ctx, slug)
	if cachedErr == nil && !IsStale(cached, c.ttl) {
		return cached, nil
	}
	result, refreshErr, _ := c.flight.Do("slug:"+slug, func() (interface{}, error) {
		fresh, ferr := c.client.GetStoreBySlug(ctx, slug)
		if ferr != nil {
			return nil, ferr
		}
		if fresh == nil {
			return nil, ErrNotFound
		}
		fresh.SyncedAt = time.Now()
		if err := c.repo.Upsert(ctx, fresh); err != nil {
			return nil, err
		}
		return fresh, nil
	})
	if refreshErr == nil && result != nil {
		return result.(*Store), nil
	}
	// Stale-fallback window: if we have a cached row less than 24h old,
	// prefer it over bubbling up a platform-api outage.
	if cachedErr == nil && cached != nil && time.Since(cached.SyncedAt) < 24*time.Hour {
		return cached, nil
	}
	if errors.Is(refreshErr, ErrNotFound) {
		return nil, ErrNotFound
	}
	return nil, ErrNotFound
}
