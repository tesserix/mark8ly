package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
)

// switchableLookup is a tenantgate.Lookup whose status can be flipped
// mid-test, standing in for platform-api's tenant status actually
// changing (via a real suspend/unsuspend call) between two requests
// through the admin gate.
type switchableLookup struct {
	mu     sync.Mutex
	status string
}

func (s *switchableLookup) set(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *switchableLookup) Get(_ context.Context, id string) (*tenantdirectory.TenantDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &tenantdirectory.TenantDetail{Tenant: tenantdirectory.Tenant{ID: id, Status: s.status}}, nil
}

// buildAdminGateRouter wires a minimal admin route behind the real
// tenantgate.Gate, exactly the shape the admin group in
// internal/handlers/admin/routes.go uses. A long TTL means the only way
// the cache clears between two requests is an explicit Invalidate call —
// which is exactly what this file is testing.
func buildAdminGateRouter(t *testing.T, gate *tenantgate.Gate) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", testTenant) })
	r.GET("/admin/account", gate.RequireActiveTenant(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func doAdminGet(r *gin.Engine) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/account", nil))
	return rec
}

// TestSuspend_InvalidatesGate_NoTTLWait proves the EFFECT of invalidation,
// not merely that Invalidate was called: a request being served from a
// long-TTL cached "active" entry is refused on the very next call once a
// changed suspend goes through the invalidator — no TTL wait, no second
// cache-refresh cycle needed.
func TestSuspend_InvalidatesGate_NoTTLWait(t *testing.T) {
	lookup := &switchableLookup{status: "active"}
	gate := tenantgate.New(lookup, nil, time.Hour) // long TTL: only Invalidate clears it
	adminRouter := buildAdminGateRouter(t, gate)

	// Warm the cache with an active status — this is the request the
	// merchant was making right before the suspension.
	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code)

	// Upstream now reports suspended (simulating the real state change a
	// successful suspend call causes), but the gate's cache still holds
	// "active" within its TTL — prove the stale cache would otherwise
	// keep serving, so the fixture actually discriminates.
	lookup.set("suspended")
	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code,
		"fixture check: without invalidation the long TTL must still serve stale active")

	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 1, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, &fakeLifecycleStoreRepo{}, discardAudit, gate, nil)
	rec := postLifecycleTenant(t, h, testTenant, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, http.StatusForbidden, doAdminGet(adminRouter).Code,
		"suspend must invalidate the gate cache so the very next admin request is refused, with no TTL wait")
}

// TestUnsuspend_InvalidatesGate_NoTTLWait mirrors the suspend case:
// reinstating a tenant must not leave it waiting out the TTL to regain
// access.
func TestUnsuspend_InvalidatesGate_NoTTLWait(t *testing.T) {
	lookup := &switchableLookup{status: "suspended"}
	gate := tenantgate.New(lookup, nil, time.Hour)
	adminRouter := buildAdminGateRouter(t, gate)

	require.Equal(t, http.StatusForbidden, doAdminGet(adminRouter).Code)

	lookup.set("active")
	require.Equal(t, http.StatusForbidden, doAdminGet(adminRouter).Code,
		"fixture check: without invalidation the long TTL must still serve stale suspended")

	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "active", StoresAffected: 1, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, &fakeLifecycleStoreRepo{}, discardAudit, gate, nil)
	rec := postLifecycleTenant(t, h, testTenant, "unsuspend", `{"reason_code":"resolved"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code,
		"unsuspend must invalidate the gate cache so the very next admin request is served, with no TTL wait")
}

// TestLifecycle_NoopStillInvalidatesGate replaces an earlier assertion
// that a no-op must NOT invalidate the cache (#344).
//
// That assertion justified itself as "consistent with the audit side
// effect also being skipped for a no-op". The consistency is superficial
// and the two are not alike: an audit row RECORDS a change, so skipping
// it for a no-op is right; cache invalidation ENFORCES a status, and the
// status is a fact about the tenant whether or not this particular call
// is what changed it.
//
// Read the scenario below and the old expectation is plainly wrong.
// Upstream is suspended. The gate holds a cached "active". The old test
// required the admin gate to keep serving that tenant as active — for up
// to the TTL, five minutes in production — and called that correct. It
// pinned under-enforcement in place.
//
// It also made #344 unfixable: after a failed projection update the only
// recourse is a retry, and a retry always reports Changed: false, so
// every repair path was closed.
//
// Invalidation is cheap — it drops one cache entry and costs one refetch
// — so there is no reason to withhold it from the call that is trying to
// make enforcement true.
func TestLifecycle_NoopStillInvalidatesGate(t *testing.T) {
	lookup := &switchableLookup{status: "active"}
	gate := tenantgate.New(lookup, nil, time.Hour)
	adminRouter := buildAdminGateRouter(t, gate)

	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code)

	// Upstream is now suspended, but this call reports Changed: false —
	// the tenant was already suspended, e.g. an operator retrying after a
	// failed local projection update.
	lookup.set("suspended")
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	h := platformadmin.NewTenantLifecycleHandler(stub, &fakeLifecycleStoreRepo{}, discardAudit, gate, nil)
	rec := postLifecycleTenant(t, h, testTenant, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, http.StatusForbidden, doAdminGet(adminRouter).Code,
		"a retry must drop the stale cached status so the suspension is enforced "+
			"on the very next admin request, rather than lingering for the TTL")
}

// TestLifecycle_NilInvalidatorDoesNotPanic asserts the handler degrades
// gracefully with no gate wired at all — matching how the audit func's
// nil case is tolerated (though for a different reason: an unwired
// invalidator is a lag, not a lost record, so it is not mount-gated).
func TestLifecycle_NilInvalidatorDoesNotPanic(t *testing.T) {
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 1, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, &fakeLifecycleStoreRepo{}, discardAudit, nil, nil)

	require.NotPanics(t, func() {
		rec := postLifecycleTenant(t, h, testTenant, "suspend", `{"reason_code":"abuse"}`)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
