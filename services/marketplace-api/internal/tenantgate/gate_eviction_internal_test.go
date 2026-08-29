package tenantgate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// #345: the cache had no eviction and no size bound. Entries were added on
// lookup and removed only by Invalidate, so the map grew with the number of
// DISTINCT TENANTS EVER SEEN by the process rather than with active load.
//
// These tests are in-package deliberately: asserting on the map means
// reading g.cache directly, and the alternative — an exported accessor
// that exists only for tests — would put test-only API on a production
// type.

// evictionLookup answers for any id, so a sweep can be triggered by
// refreshing a tenant that is not the one under test.
type evictionLookup struct {
	status string
	err    error
}

func (l *evictionLookup) Get(_ context.Context, id string) (*tenantdirectory.TenantDetail, error) {
	if l.err != nil {
		return nil, l.err
	}
	return &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: id, Status: l.status},
	}, nil
}

// seed puts an entry in the cache with an explicit age.
func (g *Gate) seed(id, status string, age time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cache[id] = cacheEntry{status: status, fetchedAt: time.Now().Add(-age)}
}

func (g *Gate) cached(id string) (cacheEntry, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.cache[id]
	return e, ok
}

func (g *Gate) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.cache)
}

// triggerRefresh drives one request for tenantID through the middleware,
// which is what causes a refresh (and therefore a sweep).
func triggerRefresh(g *Gate, tenantID string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", tenantID) })
	r.GET("/x", g.RequireActiveTenant(), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
}

// An idle tenant's active entry is dropped once it is past its TTL. It
// would have been re-fetched on its next request anyway, so dropping it
// costs nothing and is what stops the map growing forever.
func TestGate_SweepDropsIdleActiveEntries(t *testing.T) {
	g := New(&evictionLookup{status: statusActive}, nil, time.Minute)
	g.seed("idle-1", statusActive, time.Hour)
	g.seed("idle-2", statusActive, time.Hour)

	triggerRefresh(g, "newcomer")

	_, ok1 := g.cached("idle-1")
	_, ok2 := g.cached("idle-2")
	require.False(t, ok1, "an idle active entry past its TTL must be evicted")
	require.False(t, ok2, "an idle active entry past its TTL must be evicted")
	require.Equal(t, 1, g.size(), "only the tenant just refreshed should remain")
}

// The security-critical half. A suspended entry is authoritative at ANY
// age — that is what stops a failed refresh decaying a suspension into
// access. Evicting it would drop the request into the cold-cache branch,
// which fails OPEN, handing a suspended tenant access during exactly the
// platform-api outage where enforcement matters most.
func TestGate_SweepNeverEvictsSuspendedEntriesAtAnyAge(t *testing.T) {
	g := New(&evictionLookup{status: statusActive}, nil, time.Minute)
	g.seed("suspended-tenant", "suspended", 30*24*time.Hour)

	triggerRefresh(g, "newcomer")

	entry, ok := g.cached("suspended-tenant")
	require.True(t, ok,
		"a suspended entry must survive eviction at any age: dropping it falls "+
			"through to the cold-cache branch, which fails OPEN")
	require.Equal(t, "suspended", entry.status)
}

// The end-to-end statement of the same property: after a sweep, a
// suspended tenant is still refused even when the lookup is failing.
func TestGate_SuspendedStaysRefusedAfterSweep(t *testing.T) {
	g := New(&evictionLookup{status: statusActive}, nil, time.Minute)
	g.seed("suspended-tenant", "suspended", 30*24*time.Hour)
	g.seed("idle-active", statusActive, time.Hour)

	triggerRefresh(g, "newcomer") // causes the sweep

	// Now the lookup starts failing, so the cached verdict is all there is.
	g.lookup = &evictionLookup{err: errors.New("platform-api unreachable")}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "suspended-tenant") })
	r.GET("/x", g.RequireActiveTenant(), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusForbidden, rec.Code,
		"a suspended tenant must still be refused after a sweep, with the lookup down")
}

// A tenant in active use must not be evicted out from under itself.
func TestGate_SweepKeepsFreshActiveEntries(t *testing.T) {
	g := New(&evictionLookup{status: statusActive}, nil, time.Hour)
	g.seed("fresh", statusActive, time.Second)

	triggerRefresh(g, "newcomer")

	_, ok := g.cached("fresh")
	require.True(t, ok, "an entry still inside its TTL must not be evicted")
}
