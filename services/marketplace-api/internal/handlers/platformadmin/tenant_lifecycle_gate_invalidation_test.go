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

// TestLifecycle_NoopDoesNotInvalidateGate asserts the negative: a call
// upstream reports as unchanged (Changed: false) must NOT touch the
// cache, consistent with the audit side effect also being skipped for a
// no-op.
func TestLifecycle_NoopDoesNotInvalidateGate(t *testing.T) {
	lookup := &switchableLookup{status: "active"}
	gate := tenantgate.New(lookup, nil, time.Hour)
	adminRouter := buildAdminGateRouter(t, gate)

	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code)

	// Upstream is now suspended, but this call reports Changed: false (the
	// tenant was already suspended, say) — the cache must stay untouched.
	lookup.set("suspended")
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	h := platformadmin.NewTenantLifecycleHandler(stub, &fakeLifecycleStoreRepo{}, discardAudit, gate, nil)
	rec := postLifecycleTenant(t, h, testTenant, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, http.StatusOK, doAdminGet(adminRouter).Code,
		"a no-op (Changed: false) must not invalidate the gate cache")
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
