package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
)

// stubTenantLookup is a minimal tenantgate.Lookup double: it always
// returns the configured status for whatever tenant id is asked for.
type stubTenantLookup struct {
	status string
}

func (s *stubTenantLookup) Get(_ context.Context, id string) (*tenantdirectory.TenantDetail, error) {
	return &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: id, Status: s.status},
	}, nil
}

// stubStoresRepo implements stores.Repository. Only ListForTenant — the
// one method StoresHandler.List calls — returns anything; the rest are
// inert stubs, mirroring fakeLifecycleStoreRepo in
// internal/handlers/platformadmin/tenant_lifecycle_test.go.
//
// It exists so the route below has a WORKING terminus: with it wired, a
// request that gets past the tenant gate reaches the handler and is
// answered 200. That is what makes the 403 assertion the thing that
// fails when the gate is removed (#342) rather than a nil-dereference
// somewhere in the middleware chain.
type stubStoresRepo struct{}

func (r *stubStoresRepo) GetByIDForTenant(_ context.Context, _, _ string) (*stores.Store, error) {
	return nil, stores.ErrNotFound
}
func (r *stubStoresRepo) GetBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, stores.ErrNotFound
}
func (r *stubStoresRepo) ListForTenant(_ context.Context, tenantID string) ([]stores.Store, error) {
	return []stores.Store{{
		ID: "store-1", TenantID: tenantID, Slug: "the-bondi-store",
		Name: "The Bondi Store", CountryCode: "AU", CurrencyCode: "AUD",
		Timezone: "Australia/Sydney", Status: "active",
	}}, nil
}
func (r *stubStoresRepo) Upsert(_ context.Context, _ *stores.Store) error { return nil }
func (r *stubStoresRepo) GetProductsWatermark(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (r *stubStoresRepo) CountActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *stubStoresRepo) CountActiveOrSoftDeletedRestorableTx(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *stubStoresRepo) ListActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) ([]stores.Store, error) {
	return nil, nil
}
func (r *stubStoresRepo) InFlightOrderCount(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *stubStoresRepo) SuspendActiveForTenant(_ context.Context, _ string) error { return nil }
func (r *stubStoresRepo) MarkStaleForTenant(_ context.Context, _ string) error     { return nil }

var _ stores.Repository = (*stubStoresRepo)(nil)

// TestRegisterAdminMobile_SuspendedTenantRefusedOnNonStoreRoute is the F1
// regression test (#287 review): mobile_routes.go is the FIFTH admin route
// group, and it never applied deps.TenantGate — so a suspended tenant's
// phone could keep reaching non-store-scoped mobile routes (platform
// support chat, account deletion, GET /mobile/admin/stores) even though
// the same tenant is refused everywhere on the web admin table.
//
// This exercises GET /api/v1/mobile/admin/stores specifically: it is
// tenant-wide, NOT store-scoped, so deps.StoresMiddleware never runs on
// it and only deps.TenantGate can catch a suspended tenant here.
//
// Everything downstream of the gate is wired to WORK (#342): a real FGA
// fake granting the caller staff on the tenant, and a stores repo that
// returns a row. Delete the TenantGate line from mobile_routes.go and
// this request is answered 200 — so the assertion that fails is the 403
// below, not a panic on a nil FGA client. A mutation that crashes proves
// only that something crashed; this one proves the assertion discriminates.
func TestRegisterAdminMobile_SuspendedTenantRefusedOnNonStoreRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	gate := tenantgate.New(&stubTenantLookup{status: "suspended"}, nil, time.Minute)

	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			TenantGate:      gate.RequireActiveTenant(),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		ZitadelEnabled: true,
		TokenVerifier:  &auth.FakeVerifier{UserID: "user-1"},
		// In ZitadelEnabled mode, tenant_id comes from TenantFromRequest's
		// FGA membership check against the caller-stated
		// X-Acting-Tenant-Id header below, not from the token.
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code,
		"a suspended tenant must be refused on the mobile route, exactly as on the web admin table")
	require.Contains(t, rec.Body.String(), "tenant_suspended")
}

// TestRegisterAdminMobile_GIPMode_SuspendedTenantRefusedOnNonStoreRoute is
// the same F1 regression, but for the ZitadelEnabled=false / GIP
// configuration (today's production default), where tenant_id comes from
// the token claim rather than an acting-tenant header. TenantGate must
// refuse a suspended tenant here too — the flag-gated tenancy source
// change (blocking-fix round) must not have quietly disabled this guard
// for the mode almost every deployment actually runs in.
func TestRegisterAdminMobile_GIPMode_SuspendedTenantRefusedOnNonStoreRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	gate := tenantgate.New(&stubTenantLookup{status: "suspended"}, nil, time.Minute)

	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			TenantGate:      gate.RequireActiveTenant(),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		// ZitadelEnabled omitted (false, the default): tenant_id must
		// come from the token claim, with no header involved at all.
		TokenVerifier: &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code,
		"a suspended tenant must be refused on the mobile route in GIP mode too")
	require.Contains(t, rec.Body.String(), "tenant_suspended")
}
