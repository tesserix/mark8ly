// Package tenantgate provides RequireActiveTenant, a Gin middleware that
// refuses ALL admin traffic for a suspended tenant (#287).
//
// StoreMiddleware (internal/stores) already refuses a suspended tenant on
// /admin/stores/:storeId, but that group is one of FIVE admin route groups
// (four web + the mobile group, internal/handlers/admin/mobile_routes.go,
// which was missed in the original design and fixed under F1/#287). The
// other four — /admin, /admin/account, the SSO group
// /admin/tenants/:tenantId, and the mobile group's non-store-scoped routes
// (platform-support, account, /mobile/admin/stores) — are tenant-scoped,
// not store-scoped, so StoreMiddleware never runs on them. A suspended
// tenant with an existing session (or a tenant with zero stores) would
// otherwise keep full access to those groups until the session expired.
// This package closes that gap at the tenant level, independent of any
// specific store.
//
// Deliberately NOT modelled on internal/subscription/readonly: that
// middleware allowlists billing/tax/recovery routes because a read-only
// subscription is a billing state the merchant must be able to self-serve
// out of. A tenant suspension is an operator action (abuse, fraud, legal
// demand) where self-service recovery is precisely what must not be
// available. This gate allowlists NOTHING, not even GET.
package tenantgate

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// statusActive is the only tenantdirectory status that passes the gate.
const statusActive = "active"

// tenantIDContextKey is where auth.HeaderTrustAuth puts the trusted tenant
// id on the Gin context.
const tenantIDContextKey = "tenant_id"

// Lookup is the subset of tenantdirectory.Client the gate needs.
type Lookup interface {
	Get(ctx context.Context, id string) (*tenantdirectory.TenantDetail, error)
}

type cacheEntry struct {
	status    string
	fetchedAt time.Time
}

// Gate caches tenant status in-process to avoid a platform-api round trip
// on every admin request. The production admin deployment runs a single
// replica, so this cache is coherent. If it is ever scaled out, this stops
// being merely "less cache-efficient": Invalidate (called from the platform
// console's suspend/unsuspend handler) reaches only the pod that served
// that request, so every OTHER pod keeps serving its cached status until
// its own ttl expires — a suspend can be bypassed, and an unsuspend can
// stay invisible, on any pod that didn't see the invalidation, for up to
// ttl. Correctness currently depends on replicas: 1; multi-replica would
// need a shared cache (e.g. Redis) or a pub/sub invalidation fan-out.
//
// Cache growth is bounded by sweepLocked, which evicts idle ACTIVE entries
// past ttl (#345). Non-active entries are deliberately never evicted — see
// sweepLocked for why dropping one would be a security regression rather
// than a memory saving.
type Gate struct {
	lookup Lookup
	logger *slog.Logger
	ttl    time.Duration

	mu        sync.Mutex
	cache     map[string]cacheEntry
	lastSweep time.Time
	flight    singleflight.Group
}

// New builds a Gate. ttl controls how long a cached ACTIVE status is
// trusted before a refresh is attempted; a cached suspended (or any other
// non-active) status is authoritative regardless of age — see
// RequireActiveTenant.
func New(l Lookup, logger *slog.Logger, ttl time.Duration) *Gate {
	return &Gate{
		lookup: l,
		logger: logger,
		ttl:    ttl,
		cache:  make(map[string]cacheEntry),
	}
}

// RequireActiveTenant returns a Gin middleware that refuses every request
// for a non-active tenant with 403 {"error":"tenant_suspended"}.
//
// 403, not 402: 402 is RequireActive's billing meaning (readonly package)
// and does not apply here. Not 404 either: unlike StoreMiddleware's 404 for
// a store the caller is no longer entitled to know exists, the caller here
// already holds a valid session for this exact tenant, so there is nothing
// to hide about the tenant's existence — only its access is refused.
//
// Staleness handling, mirroring StoreMiddleware's asymmetry, WITH ONE
// DELIBERATE DIVERGENCE (see the last bullet):
//   - A cached suspended (non-active) status is never served AS ACCESS,
//     regardless of age — but it IS re-fetched once ttl has elapsed (see
//     the cached/entry.status != statusActive branch below), and an
//     authoritative "active" answer from that re-fetch lifts it
//     immediately. This has to be true: it is exactly what lets an
//     unsuspend take effect without waiting for every session to expire.
//     What never happens is decaying a suspended verdict into access on a
//     FAILED refresh — a stale suspended entry whose re-fetch errors stays
//     refused.
//   - A cached active status past ttl is refreshed; if the refresh fails,
//     the stale active status is served (fail open on the merchant's side).
//   - No cached value at all plus a failed lookup fails OPEN (serves). A
//     cold cache during a platform-api outage must not lock out every
//     merchant. This is the gate's one deliberate hole.
//   - Divergence from StoreMiddleware: StoreMiddleware caps its fail-open
//     behavior with an absolute StaleCeil (24h) — past that age a stale row
//     404s even if it was last known active, bounding how long an outage
//     can paper over a stale answer. This gate has NO such ceiling: a
//     cached active tenant is served indefinitely across repeated failed
//     refreshes, for as long as platform-api stays unreachable, no matter
//     how long that is. This is a deliberate divergence, not an oversight —
//     it follows the same "must not lock out every merchant" reasoning as
//     the cold-cache hole above — but it is a genuine gap versus
//     StoreMiddleware's design and is called out here so it isn't mistaken
//     for a faithful mirror of it.
//
// A nil Gate (no platform-api client configured) and a request with no
// tenant_id on the context are both treated as "this middleware cannot
// judge" and pass the request through.
func (g *Gate) RequireActiveTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || g.lookup == nil {
			c.Next()
			return
		}

		raw, ok := c.Get(tenantIDContextKey)
		if !ok {
			c.Next()
			return
		}
		tenantID, _ := raw.(string)
		if tenantID == "" {
			c.Next()
			return
		}

		g.mu.Lock()
		entry, cached := g.cache[tenantID]
		g.mu.Unlock()

		if cached && time.Since(entry.fetchedAt) < g.ttl {
			g.decide(c, entry.status)
			return
		}

		result, err, _ := g.flight.Do(tenantID, func() (interface{}, error) {
			return g.refresh(c.Request.Context(), tenantID)
		})

		switch {
		case err == nil:
			g.decide(c, result.(string))
		case cached && entry.status != statusActive:
			// Cached suspended is authoritative at any age: a failed
			// refresh must not decay it into access.
			refuse(c)
		case cached:
			// Cached active, refresh failed: fail open on the stale value.
			g.warn("serving stale active tenant status", tenantID, err)
			c.Next()
		default:
			// Cold cache and a failed lookup: fail open rather than lock
			// out every merchant during a platform-api outage.
			g.warn("tenant status lookup failed with no cached value; failing open", tenantID, err)
			c.Next()
		}
	}
}

func (g *Gate) decide(c *gin.Context, status string) {
	if status != statusActive {
		refuse(c)
		return
	}
	c.Next()
}

func (g *Gate) refresh(ctx context.Context, tenantID string) (string, error) {
	detail, err := g.lookup.Get(ctx, tenantID)
	if err != nil {
		return "", err
	}
	now := time.Now()
	g.mu.Lock()
	g.cache[tenantID] = cacheEntry{status: detail.Status, fetchedAt: now}
	g.sweepLocked(now)
	g.mu.Unlock()
	return detail.Status, nil
}

// sweepLocked drops idle ACTIVE entries, bounding a cache that otherwise
// grew with the number of distinct tenants ever seen by the process rather
// than with active load (#345). The caller holds g.mu.
//
// # Only active entries, and this is the whole point
//
// A non-active entry is authoritative at ANY age — that is what stops a
// failed refresh decaying a suspension into access. Evicting one would
// drop the next request into the cold-cache branch of RequireActiveTenant,
// which fails OPEN, handing a suspended tenant access during exactly the
// platform-api outage where enforcement matters most. So non-active
// entries are kept indefinitely. That set is bounded by how many tenants
// are actually suspended, which is small, and losing one is a security
// regression rather than a memory saving.
//
// # Why dropping an idle active entry changes nothing
//
// An active entry past ttl is re-fetched on its next request regardless.
// If that refresh fails, the request fails open either way: with the entry
// present via the "cached active, refresh failed" branch, without it via
// the cold-cache branch. Same outcome, different log line. So this is a
// memory bound, not a change to the gate's access behaviour — the
// deliberate absence of an absolute staleness ceiling documented on
// RequireActiveTenant is untouched.
//
// # Throttled
//
// Scanning the map on every refresh would be O(entries) per cache miss.
// Once per ttl is enough: entries only become evictable after ttl, so a
// tighter interval could not find more of them.
func (g *Gate) sweepLocked(now time.Time) {
	if now.Sub(g.lastSweep) < g.ttl {
		return
	}
	g.lastSweep = now
	for id, entry := range g.cache {
		if entry.status == statusActive && now.Sub(entry.fetchedAt) >= g.ttl {
			delete(g.cache, id)
		}
	}
}

// Invalidate drops tenantID's cached status, so a suspend or unsuspend
// action taken through the platform console takes effect on the very next
// admin request instead of waiting out ttl. Safe on a nil *Gate — matching
// how the rest of this type degrades when unwired.
func (g *Gate) Invalidate(tenantID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.cache, tenantID)
	g.mu.Unlock()
}

func (g *Gate) warn(msg, tenantID string, err error) {
	if g.logger == nil {
		return
	}
	g.logger.Warn(msg, "tenant_id", tenantID, "error", err)
}

func refuse(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant_suspended"})
}
