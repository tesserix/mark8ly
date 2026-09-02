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
//     persists in memory for the whole TTL window. Get evicts an expired
//     entry the next time that reference is read (see the ordering note
//     on Get), so a reference on an active request path never retains
//     stale plaintext beyond one TTL past its last read. NOTE: a
//     reference that is cached once and then never read, Put, or
//     Destroyed again for the rest of the process's life is not swept
//     proactively — there is no background goroutine here, by design —
//     so its entry remains until the pod recycles. This is the accepted
//     shape of a purely on-demand (no janitor) TTL cache; it does not
//     affect the credentials this decorator targets, which are read on
//     every checkout / shipping-rate / payment-webhook request.
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
//     in the map untouched so future misses can keep serving it too, for
//     as long as the outage lasts.
//   - and no entry exists at all, the error propagates unchanged. This
//     is the behaviour that matters most: stale-on-error must never
//     become swallow-all-errors, or a total backend outage would present
//     as an empty credential to a payment gateway instead of a visible
//     failure.
//
// Eviction ordering: an expired entry is NOT removed from the map the
// moment expiry is detected. It is removed only once the inner call
// SUCCEEDS, at the exact point that entry is replaced with the fresh
// value. This ordering is deliberate — evicting the expired entry before
// calling inner would destroy the only fallback stale-on-error has if
// that inner call then fails, which is precisely the case that must keep
// checkout alive during an OpenBao blip. Deleting only on the success
// path still keeps the doc comment's "up to the TTL" residency bound
// honest for the request paths this cache targets (checkout,
// shipping-rates, payment-webhook): any reference that keeps being read
// has its expired entry replaced at the next successful read, rather
// than accumulating stale duplicate state.
func (c *CachingStore) Get(ctx context.Context, reference string) (string, error) {
	now := c.clock()

	c.mu.Lock()
	entry, found := c.entries[reference]
	expired := found && now.Sub(entry.fetchedAt) >= c.ttl
	c.mu.Unlock()

	if found && !expired {
		return entry.plaintext, nil
	}

	plaintext, err := c.inner.Get(ctx, reference)
	if err != nil {
		if found {
			// entry was captured above, before any map mutation, so it
			// is still available here as the stale fallback even though
			// (by design) we have not touched the map on this call.
			c.counter(StaleReadMetric, 1)
			return entry.plaintext, nil
		}
		return "", err
	}

	// The inner call succeeded: this is the only point where it is safe
	// to evict the expired entry (if any) rather than merely overwriting
	// it, so the map never retains more than one entry per reference and
	// never silently disagrees with what a fresh read just confirmed.
	c.mu.Lock()
	delete(c.entries, reference)
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
//
// When a rewrap DID happen, both the old and new references are
// invalidated in this cache. The inner rewrap writes the new reference
// through the inner store directly (ChainStore.Put), bypassing
// CachingStore.Put entirely — so without this, a stale cache entry from
// earlier in the process's life (or one populated by a concurrent read
// racing this rewrap) would keep serving old plaintext under the newly
// minted reference for up to the TTL, and oldRef would keep resolving a
// value that a fresh read should no longer reach through this path.
func (c *CachingStore) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	rw, ok := c.inner.(Rewrapper)
	if !ok {
		return "", false
	}
	newRef, changed := rw.MaybeRewrap(ctx, oldRef, scope, plaintext)
	if changed {
		c.invalidate(oldRef)
		c.invalidate(newRef)
	}
	return newRef, changed
}

var _ Store = (*CachingStore)(nil)
var _ Rewrapper = (*CachingStore)(nil)
