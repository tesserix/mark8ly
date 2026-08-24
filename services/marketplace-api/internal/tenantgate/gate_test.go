package tenantgate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
)

const testTenant = "tenant-1"

// stubLookup returns a canned tenant and counts calls so the cache can be
// asserted. Values are distinct and non-zero so nothing passes on a zero.
type stubLookup struct {
	status string
	err    error
	calls  int32
}

func (s *stubLookup) Get(_ context.Context, id string) (*tenantdirectory.TenantDetail, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: id, Status: s.status},
	}, nil
}

func buildGateRouter(t *testing.T, l tenantgate.Lookup, ttl time.Duration, reached *bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", testTenant) })
	g := tenantgate.New(l, nil, ttl)
	r.GET("/admin/account", g.RequireActiveTenant(), func(c *gin.Context) {
		*reached = true
		c.String(http.StatusOK, "ok")
	})
	return r
}

func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// An active tenant passes through.
func TestGate_ActiveTenantPasses(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "active"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, reached)
}

// A suspended tenant is refused on a NON-store-scoped route — the whole
// point of this task, since StoreMiddleware never sees these.
func TestGate_SuspendedTenantIsRefusedOnNonStoreRoute(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "suspended"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, reached, "handler must not run for a suspended tenant")
}

// NOTHING is allowlisted — not even a GET. This is the assertion that
// catches someone copying readonly.DefaultAllowlist wholesale.
func TestGate_NoAllowlistNotEvenGET(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "suspended"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusForbidden, rec.Code, "a GET is NOT exempt for a suspended tenant")
	require.False(t, reached)
}

// The cache is used: two requests inside the TTL cause ONE lookup.
func TestGate_CachesWithinTTL(t *testing.T) {
	reached := false
	l := &stubLookup{status: "active"}
	r := buildGateRouter(t, l, time.Minute, &reached)
	doGet(r, "/admin/account")
	doGet(r, "/admin/account")
	require.Equal(t, int32(1), atomic.LoadInt32(&l.calls), "second request must be served from cache")
}

// Cached suspended is authoritative at ANY age: even with the TTL expired
// and the upstream now saying active, the gate keeps refusing until it has
// successfully re-read. Assert the refusal, not the call count.
func TestGate_CachedSuspendedIsAuthoritativeWhenLookupFails(t *testing.T) {
	reached := false
	l := &stubLookup{status: "suspended"}
	r := buildGateRouter(t, l, time.Nanosecond, &reached) // instantly stale
	require.Equal(t, http.StatusForbidden, doGet(r, "/admin/account").Code)

	l.err = tenantdirectory.ErrUnavailable // upstream now unreachable
	require.Equal(t, http.StatusForbidden, doGet(r, "/admin/account").Code,
		"a cached suspended status must not decay into access")
	require.False(t, reached)
}

// Stale ACTIVE plus a failed refresh still serves — fail-open, so an
// outage does not lock out every merchant.
func TestGate_StaleActiveWithFailedRefreshStillServes(t *testing.T) {
	reached := false
	l := &stubLookup{status: "active"}
	r := buildGateRouter(t, l, time.Nanosecond, &reached)
	require.Equal(t, http.StatusOK, doGet(r, "/admin/account").Code)

	l.err = tenantdirectory.ErrUnavailable
	require.Equal(t, http.StatusOK, doGet(r, "/admin/account").Code,
		"stale active must still serve when the refresh fails")
}

// Cold cache plus a failed lookup fails OPEN. Deliberate: assert it so the
// behaviour is a decision on the record rather than an accident.
func TestGate_ColdCacheWithFailedLookupFailsOpen(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t,
		&stubLookup{err: tenantdirectory.ErrUnavailable}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code,
		"cold cache + outage must not lock out every merchant")
	require.True(t, reached)
}

// No tenant on the context means this middleware cannot judge: pass through
// and let the auth layer deal with it, rather than 403-ing every request.
func TestGate_NoTenantOnContextPassesThrough(t *testing.T) {
	reached := false
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := tenantgate.New(&stubLookup{status: "suspended"}, nil, time.Minute)
	r.GET("/admin/account", g.RequireActiveTenant(), func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "ok")
	})
	rec := doGet(r, "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, reached)
}

// A nil Gate (no platform-api client wired) must be a no-op, not a panic —
// matching how other client-backed features degrade when unconfigured.
func TestGate_NilGateIsNoOp(t *testing.T) {
	reached := false
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", testTenant) })
	var g *tenantgate.Gate
	r.GET("/admin/account", g.RequireActiveTenant(), func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "ok")
	})
	rec := doGet(r, "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, reached)
}
