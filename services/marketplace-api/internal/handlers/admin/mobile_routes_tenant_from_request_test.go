package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// TestRegisterAdminMobile_ZitadelMode_TenantFromRequestRunsBetweenBearerAuthAndRequireTenant
// pins the exact chain order for the ZitadelEnabled=true configuration:
//
//	bearerAuth (no tenant write) -> tenant-from-request -> requireTenant -> ... -> rateLimiter
//
// The FakeVerifier here deliberately carries NO tenant_id claim (mirroring
// a Zitadel token, which never mints one, and matching GIPBearerAuth's
// setTenantFromClaim=false in this mode) — so the ONLY way tenant_id ever
// becomes non-empty is if TenantFromRequest runs, and the ONLY way it can
// succeed is if it runs AFTER bearerAuth has already set user_id (the FGA
// membership check needs it) and BEFORE requireTenant (which 404s on an
// empty tenant_id before TenantFromRequest would ever get a chance to run).
// Get the order wrong either way and this request 404s instead of
// reaching the handler — so this test discriminates a swapped or missing
// wiring, not just "the app starts".
func TestRegisterAdminMobile_ZitadelMode_TenantFromRequestRunsBetweenBearerAuthAndRequireTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		ZitadelEnabled: true,
		TokenVerifier:  &auth.FakeVerifier{UserID: "user-1"},
		// GIPBearerAuth never sets tenant_id from the token when
		// ZitadelEnabled is true, so reaching the handler at all proves
		// TenantFromRequest resolved tenant_id from the
		// X-Acting-Tenant-Id header + FGA membership check, in the right
		// slot in the chain.
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "a member's stated tenant must resolve and reach the handler: %s", rec.Body.String())
}

// TestRegisterAdminMobile_ZitadelMode_NonMemberActingTenantGets404NotUnauthorized
// is the negative half of the same wiring: a caller who states a tenant
// they do NOT belong to must be refused as 404 (no store linked), never
// 401 — see require_bound_tenant.go's doc comment on why a 401 here is a
// real incident (mobile client signOut()+redirect loop). This also
// confirms TenantFromRequest's fail-closed default: an unresolved header
// must NOT leak through as a usable tenant_id.
func TestRegisterAdminMobile_ZitadelMode_NonMemberActingTenantGets404NotUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	fga := authz.NewFakeClient() // user-1 granted nothing anywhere

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		ZitadelEnabled:          true,
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"a non-member's stated tenant must be refused as 404 (no store), never 401 (which signs the mobile client out)")
}

// TestRegisterAdminMobile_ZitadelMode_NoActingTenantHeaderMeans404 proves
// TenantFromRequest is the ONLY source of tenancy when ZitadelEnabled is
// true: with no X-Acting-Tenant-Id header, the caller is refused 404 even
// though the same user IS a genuine FGA member of tenant-1 — there is
// simply no mechanism left (in this mode) to supply a tenant without the
// header.
func TestRegisterAdminMobile_ZitadelMode_NoActingTenantHeaderMeans404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		ZitadelEnabled:          true,
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	// Deliberately no X-Acting-Tenant-Id header.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestRegisterAdminMobile_GIPMode_ClaimSuppliesTenant_NoHeaderNeeded is
// THE regression test for the blocking bug found in whole-branch review:
// apps/mobile-admin (packages/mobile-shared/api/client.ts) never sends
// X-Acting-Tenant-Id — it doesn't exist anywhere outside this service —
// so with ZitadelEnabled false (today's production default), a request
// carrying ONLY a bearer token (no acting-tenant header, no
// TenantMembershipChecker even configured) must still resolve a tenant
// from the token's own claim and reach the handler, exactly as it did on
// origin/main before #524 phase 4 touched this file. If this test fails
// with 404, every merchant's mobile app is bricked on deploy.
func TestRegisterAdminMobile_GIPMode_ClaimSuppliesTenant_NoHeaderNeeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// fga backs deps.AuthzMiddleware's RequireTenantRelation check on the
	// route handler itself (a real check regardless of ZitadelEnabled) —
	// separate from TenantMembershipChecker, which is what's genuinely
	// "never consulted in this mode" below.
	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		// ZitadelEnabled defaults to false — the field is omitted here on
		// purpose, matching cfg.ZitadelEnabled's own default and every
		// deployment that has not opted into Zitadel.
		TokenVerifier: &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
		// No TenantMembershipChecker at all: in GIP mode TenantFromRequest
		// must not even be mounted, so this being nil must not matter.
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	// Deliberately no X-Acting-Tenant-Id header — this is the shape of
	// every real request apps/mobile-admin has ever sent or ever will,
	// until (if ever) a header is added to packages/mobile-shared.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"GIP mode must resolve tenant_id from the token claim with no header — this is the byte-identical-to-origin/main guarantee: %s", rec.Body.String())
}

// TestRegisterAdminMobile_GIPMode_ActingTenantHeaderIsIgnored is the "two
// writers never both active" proof for GIP mode: even when a caller sends
// an X-Acting-Tenant-Id header naming a DIFFERENT tenant than their token
// claim, and even when a TenantMembershipChecker IS configured (as it
// always is in main.go's real wiring, regardless of the flag), the header
// must have zero effect — TenantFromRequest is not mounted at all in this
// mode, so the claim is the sole and unconditional source of tenancy.
func TestRegisterAdminMobile_GIPMode_ActingTenantHeaderIsIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	fga := authz.NewFakeClient()
	// user-1 has a real relation to tenant-1 (what the claim names) so
	// deps.AuthzMiddleware's RequireTenantRelation on the route itself
	// passes — that check is unrelated to TenantFromRequest and runs
	// regardless of mode. The membership grant on tenant-2 (what the
	// header names) is what proves the point: if TenantFromRequest were
	// (wrongly) consulted for that header, tenant-2 IS a tenant this user
	// FGA-belongs to, so it would resolve and overwrite tenant_id — the
	// test would then see tenant-2, not tenant-1, in the response.
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")
	fga.Grant("user-1", authz.RoleStaff, "tenant-2")

	recorder := &recordingStoresRepo{stubStoresRepo: stubStoresRepo{}}

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(recorder, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-2")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "tenant-1", recorder.gotTenantID,
		"the claim tenant must win — TenantFromRequest must not be mounted in GIP mode, and the acting-tenant header must have no effect")
}

// recordingStoresRepo captures the tenantID ListForTenant was actually
// called with, so a test can assert on the tenant the request resolved to
// without depending on AdminStoreResponse (the wire DTO) happening to echo
// it back.
type recordingStoresRepo struct {
	stubStoresRepo
	gotTenantID string
}

func (r *recordingStoresRepo) ListForTenant(ctx context.Context, tenantID string) ([]stores.Store, error) {
	r.gotTenantID = tenantID
	return r.stubStoresRepo.ListForTenant(ctx, tenantID)
}
