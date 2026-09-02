package carriersecrets

import (
	"context"
	"sync"
	"time"
)

// StaleReadMetric is the label passed to CounterFn when CachingStore
// serves a stale (expired) cached value because the inner store's Get
// failed. This is the only signal that OpenBao (or GCP SM) had a blip
// that checkout/shipping-rates/payment-webhook rode out on a cached
// credential instead of failing outright.
const StaleReadMetric = "carriersecrets_stale_read"

// cacheEntry holds one cached reference's plaintext and the clock time it
// was fetched at.
type cacheEntry struct {
	plaintext string
	fetchedAt time.Time
}

// CachingStore is a TTL-caching decorator around a Store. It is a
// decorator, not logic folded into ChainStore, so it can be independently
// tested and omitted entirely in tests that don't want caching semantics.
//
// Two consequences of the cache are accepted deliberately:
//
//   - Decrypted credentials live in process memory for up to the TTL
//     (production wires this with ttl=60s, clock=time.Now — see Task 7).
//     A cache hit never touches the inner store, so a plaintext credential
//     that would otherwise only exist for the duration of one request now
//     persists in memory for the whole TTL window.
//   - A credential rotation (Put via some OTHER process instance, or a
//     row updated out from under this process) takes up to the TTL to
//     become visible to reads through this cache. Put and Destroy invoked
//     THROUGH this same CachingStore instance invalidate immediately; a
//     rotation performed elsewhere does not.
//
// On an inner Get error, CachingStore serves a stale (expired) cached
// entry if one exists, incrementing the stale counter, so a transient
// OpenBao blip does not fail checkout. If no cached entry exists at all,
// the inner error propagates unchanged — this store never converts a
// total backend outage into a silent empty-credential miss.
type CachingStore struct {
	inner   Store
	ttl     time.Duration
	clock   func() time.Time
	counter CounterFn

	mu      sync.Mutex
	entries map[string]cacheEntry
}

// NewCachingStore constructs a CachingStore wrapping inner. clock is
// injected (production passes time.Now) so tests can control expiry
// without sleeping. counter may be nil, in which case a no-op is
// installed, matching NewChainStore's convention.
func NewCachingStore(inner Store, ttl time.Duration, clock func() time.Time, counter CounterFn) *CachingStore {
	if counter == nil {
		counter = func(string, int64) {}
	}
	return &CachingStore{
		inner:   inner,
		ttl:     ttl,
		clock:   clock,
		counter: counter,
		entries: make(map[string]cacheEntry),
	}
}

// Get resolves reference, serving a fresh cached value when one exists
// within the TTL. On a cache miss (or expiry) it consults the inner
// store. If the inner store errors:
//
//   - and a stale (expired) entry exists for reference, that stale value
//     is served and the stale counter is incremented — the entry is left
//     in place so future misses can keep serving it.
//   - and no entry exists at all, the error propagates unchanged. This
//     is the behaviour that matters most: stale-on-error must never
//     become swallow-all-errors, or a total backend outage would present
//     as an empty credential to a payment gateway instead of a visible
//     failure.
func (c *CachingStore) Get(ctx context.Context, reference string) (string, error) {
	now := c.clock()

	c.mu.Lock()
	entry, found := c.entries[reference]
	c.mu.Unlock()

	if found && now.Sub(entry.fetchedAt) < c.ttl {
		return entry.plaintext, nil
	}

	plaintext, err := c.inner.Get(ctx, reference)
	if err != nil {
		if found {
			c.counter(StaleReadMetric, 1)
			return entry.plaintext, nil
		}
		return "", err
	}

	c.mu.Lock()
	c.entries[reference] = cacheEntry{plaintext: plaintext, fetchedAt: now}
	c.mu.Unlock()

	return plaintext, nil
}

// Put writes through to the inner store and invalidates any cached entry
// for the returned reference, so a rotation is visible to the writer
// immediately instead of waiting out the TTL.
func (c *CachingStore) Put(ctx context.Context, scope Scope, plaintext string) (string, error) {
	reference, err := c.inner.Put(ctx, scope, plaintext)
	if err != nil {
		return "", err
	}
	c.invalidate(reference)
	return reference, nil
}

// Destroy deletes through to the inner store and invalidates any cached
// entry for reference — a destroyed credential must not keep resolving
// from cache, which is a security property, not a performance one.
func (c *CachingStore) Destroy(ctx context.Context, reference string) error {
	if err := c.inner.Destroy(ctx, reference); err != nil {
		return err
	}
	c.invalidate(reference)
	return nil
}

func (c *CachingStore) invalidate(reference string) {
	c.mu.Lock()
	delete(c.entries, reference)
	c.mu.Unlock()
}

// MaybeRewrap forwards to the inner store's Rewrapper implementation when
// it has one. A wrapped ChainStore must stay rewrappable through the
// cache decorator, or lazy migration silently stops working the moment a
// handler's Store dependency is a *CachingStore instead of a *ChainStore
// directly. If inner does not implement Rewrapper, this reports no
// rewrap performed.
func (c *CachingStore) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	rw, ok := c.inner.(Rewrapper)
	if !ok {
		return "", false
	}
	return rw.MaybeRewrap(ctx, oldRef, scope, plaintext)
}

var _ Store = (*CachingStore)(nil)
var _ Rewrapper = (*CachingStore)(nil)
