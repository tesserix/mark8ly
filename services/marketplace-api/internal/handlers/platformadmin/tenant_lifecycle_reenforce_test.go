package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
)

// #344: a retry must be able to re-apply local enforcement.
//
// The projection update is best-effort — when it fails, the upstream
// write has already succeeded, so the request still returns 200 with
// changed:true. Local enforcement has NOT been applied.
//
// The operator's only recourse is to retry. But platform-api has already
// suspended the tenant, so the retry comes back changed:false, and when
// the projection update runs only under `if res.Changed` the retry is a
// no-op that cannot repair anything. There is no way to force
// re-enforcement through the API at all.
//
// Running the projection update regardless of Changed makes the retry
// mean something. Both updates are idempotent — SuspendActiveForTenant
// filters on `status = active`, so on a genuine no-op it matches no rows.
func TestSuspend_NoOpStillReappliesLocalProjection(t *testing.T) {
	repo := &fakeLifecycleStoreRepo{}
	noop := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	h := platformadmin.NewTenantLifecycleHandler(noop, repo, discardAudit, nil, nil)

	rec := postLifecycle(t, h, "suspend", `{"reason_code":"abuse"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{testTenant}, repo.suspendedTenants,
		"a retry after a failed projection update must re-apply it, or the operator "+
			"has no way to force local enforcement")
	require.Empty(t, repo.staleTenants, "suspend must never mark rows stale")
}

func TestUnsuspend_NoOpStillMarksProjectionStale(t *testing.T) {
	repo := &fakeLifecycleStoreRepo{}
	noop := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "active", StoresAffected: 0, Changed: false}}
	h := platformadmin.NewTenantLifecycleHandler(noop, repo, discardAudit, nil, nil)

	rec := postLifecycle(t, h, "unsuspend", `{"reason_code":"resolved"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{testTenant}, repo.staleTenants,
		"a retry must be able to force the refetch, for the same reason as suspend")
	require.Empty(t, repo.suspendedTenants, "unsuspend must never call the suspend-projection path")
}

// countingInvalidator records Invalidate calls. Deliberately a distinct
// type from the gate used in tenant_lifecycle_gate_invalidation_test.go:
// that file proves the real Gate drops its cache, this one only needs to
// know whether the handler dispatched at all.
type countingInvalidator struct{ tenants []string }

func (c *countingInvalidator) Invalidate(tenantID string) {
	c.tenants = append(c.tenants, tenantID)
}

// The gate is what enforces on non-store-scoped admin routes, so a retry
// that cannot re-invalidate it is as stuck as one that cannot re-apply
// the projection.
func TestSuspend_NoOpStillInvalidatesTenantGate(t *testing.T) {
	inv := &countingInvalidator{}
	noop := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	h := platformadmin.NewTenantLifecycleHandler(noop, &fakeLifecycleStoreRepo{}, discardAudit, inv, nil)

	rec := postLifecycle(t, h, "suspend", `{"reason_code":"abuse"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{testTenant}, inv.tenants,
		"a no-op must still drop the gate's cached status so a retry re-enforces")
}
