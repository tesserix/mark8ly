//go:build integration

package platformadmin_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedIntegrationStore inserts one ACTIVE store row for tenantID into the
// real `stores` table via repo.Upsert. marketplace-api's stores projection
// carries no reference-data FKs (that constraint lives only in
// platform-api), so plausible-but-arbitrary country/currency/timezone
// values are fine here.
func seedIntegrationStore(t *testing.T, repo stores.Repository, tenantID string) *stores.Store {
	t.Helper()
	s := &stores.Store{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Slug:         "acme-" + uuid.NewString()[:8],
		Name:         "Acme",
		CountryCode:  "DE",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Berlin",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, repo.Upsert(t.Context(), s))
	return s
}

// TestIntegration_Suspend_UpdatesLocalProjectionImmediately is Step 5's
// proof that the local `stores` projection reflects a changed suspension
// in the SAME request — no waiting for StoreMiddleware's 5-minute
// FreshTTL. A stub tenantlifecycle.Client stands in for platform-api
// itself (that HTTP boundary is T4's concern, already covered in
// internal/tenantlifecycle); what's under test here is this handler's own
// write to the real DB-backed stores.Repository.
func TestIntegration_Suspend_UpdatesLocalProjectionImmediately(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)

	tenantID := uuid.NewString()
	seeded := seedIntegrationStore(t, repo, tenantID)

	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: tenantID, Status: "suspended", StoresAffected: 1, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, repo, nil, nil)

	rec := postLifecycleTenant(t, h, tenantID, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	got, err := repo.GetByIDForTenant(t.Context(), seeded.ID, tenantID)
	require.NoError(t, err)
	require.Equal(t, stores.StatusSuspended, got.Status,
		"local projection must already read suspended, without waiting for any TTL")
	require.False(t, stores.IsStale(got, 5*time.Minute),
		"the eager write must also refresh synced_at, or the row would immediately look stale")
}

// TestIntegration_Unsuspend_MarksLocalProjectionStaleNotActive is the
// asymmetric half: an unsuspend must NOT flip the local row back to
// active (this projection cannot tell a cascade-suspended store from an
// individually-suspended one), it must force a refetch by marking the row
// stale.
func TestIntegration_Unsuspend_MarksLocalProjectionStaleNotActive(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)

	tenantID := uuid.NewString()
	seeded := seedIntegrationStore(t, repo, tenantID)
	seeded.Status = stores.StatusSuspended
	require.NoError(t, repo.Upsert(t.Context(), seeded))

	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: tenantID, Status: "active", StoresAffected: 1, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, repo, nil, nil)

	rec := postLifecycleTenant(t, h, tenantID, "unsuspend", `{"reason_code":"resolved"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	got, err := repo.GetByIDForTenant(t.Context(), seeded.ID, tenantID)
	require.NoError(t, err)
	require.Equal(t, stores.StatusSuspended, got.Status,
		"unsuspend must NOT eagerly flip the local row to active")
	require.True(t, stores.IsStale(got, 5*time.Minute),
		"unsuspend must force the row stale so the next read refetches from platform-api")
}
