package apikeys

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// CacheEntry is what the auth middleware caches after a successful bcrypt
// verify. Storing only the verified payload (never the hash) means a cache
// hit short-circuits to a context populate and rate-limit check.
type CacheEntry struct {
	KeyID           uuid.UUID
	TenantID        uuid.UUID
	StoreID         uuid.UUID
	Scopes          []string
	RateLimitPerMin int
	RevokedAt       *time.Time // nil = active; future = rotation overlap
	insertedAt      time.Time
}

// Cache is a TTL-bounded sync.Map-backed lookup keyed by full bearer token.
// 60s TTL per §18.4 design note F. Concurrent-safe.
type Cache struct {
	ttl time.Duration
	m   sync.Map // map[string]*CacheEntry
}

// NewCache constructs a Cache with the given per-entry TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl}
}

// Get returns the entry for key when present and within TTL. Misses
// (including expired entries) return ok=false.
func (c *Cache) Get(key string) (CacheEntry, bool) {
	raw, ok := c.m.Load(key)
	if !ok {
		return CacheEntry{}, false
	}
	e := raw.(*CacheEntry)
	if time.Since(e.insertedAt) > c.ttl {
		c.m.Delete(key)
		return CacheEntry{}, false
	}
	return *e, true
}

// Put stores entry under key. Overwrites silently.
func (c *Cache) Put(key string, e CacheEntry) {
	e.insertedAt = time.Now()
	c.m.Store(key, &e)
}

// Invalidate drops a single entry. Called immediately on revoke/rotate so
// the cache never serves a stale verified payload.
func (c *Cache) Invalidate(key string) {
	c.m.Delete(key)
}

// InvalidateByKeyID drops every entry whose verified payload references the
// given APIKey ID. O(n) over cache size; acceptable because revoke + rotate
// are slow-path operations and the cache is bounded by hot-key count.
func (c *Cache) InvalidateByKeyID(keyID uuid.UUID) {
	c.m.Range(func(k, v any) bool {
		if e := v.(*CacheEntry); e.KeyID == keyID {
			c.m.Delete(k)
		}
		return true
	})
}

// Size returns the current entry count. Test-only.
func (c *Cache) Size() int {
	n := 0
	c.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
