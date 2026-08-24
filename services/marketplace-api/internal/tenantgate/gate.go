// Package tenantgate provides RequireActiveTenant, a Gin middleware that
// refuses ALL admin traffic for a suspended tenant (#287).
//
// StoreMiddleware (internal/stores) already refuses a suspended tenant on
// /admin/stores/:storeId, but that group is one of four admin route groups.
// The other three — /admin, /admin/account and the SSO group
// /admin/tenants/:tenantId — are tenant-scoped, not store-scoped, so
// StoreMiddleware never runs on them. A suspended tenant with an existing
// session (or a tenant with zero stores) would otherwise keep full access
// to those groups until the session expired. This package closes that gap
// at the tenant level, independent of any specific store.
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
// replica, so this cache is coherent; if it is ever scaled out, each pod
// simply fetches more often, which stays correct — just less cache-efficient.
type Gate struct {
	lookup Lookup
	logger *slog.Logger
	ttl    time.Duration

	mu     sync.Mutex
	cache  map[string]cacheEntry
	flight singleflight.Group
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
// Staleness handling, mirroring StoreMiddleware's asymmetry:
//   - A cached suspended (non-active) status is authoritative at ANY age —
//     never re-fetch to give the tenant the benefit of the doubt.
//   - A cached active status past ttl is refreshed; if the refresh fails,
//     the stale active status is served (fail open on the merchant's side).
//   - No cached value at all plus a failed lookup fails OPEN (serves). A
//     cold cache during a platform-api outage must not lock out every
//     merchant. This is the gate's one deliberate hole.
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
	g.mu.Lock()
	g.cache[tenantID] = cacheEntry{status: detail.Status, fetchedAt: time.Now()}
	g.mu.Unlock()
	return detail.Status, nil
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
