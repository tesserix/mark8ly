package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// TestRegisterAdminMobile_TenantFromRequestRunsBetweenBearerAuthAndRequireTenant
// pins the exact chain order the task brief calls the "main risk":
//
//	bearerAuth -> tenant-from-request -> requireTenant -> ... -> rateLimiter
//
// The FakeVerifier here deliberately carries NO tenant_id claim (mirroring
// a Zitadel token, which never mints one) — so the ONLY way tenant_id ever
// becomes non-empty is if TenantFromRequest runs, and the ONLY way it can
// succeed is if it runs AFTER bearerAuth has already set user_id (the FGA
// membership check needs it) and BEFORE requireTenant (which 404s on an
// empty tenant_id before TenantFromRequest would ever get a chance to run).
// Get the order wrong either way and this request 404s instead of
// reaching the handler — so this test discriminates a swapped or missing
// wiring, not just "the app starts".
func TestRegisterAdminMobile_TenantFromRequestRunsBetweenBearerAuthAndRequireTenant(t *testing.T) {
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
		TokenVerifier: &auth.FakeVerifier{UserID: "user-1", TenantID: ""},
		// No tenant claim on the token (as a real Zitadel token would
		// carry), so reaching the handler at all proves TenantFromRequest
		// resolved tenant_id from the X-Acting-Tenant-Id header + FGA
		// membership check, in the right slot in the chain.
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "a member's stated tenant must resolve and reach the handler: %s", rec.Body.String())
}

// TestRegisterAdminMobile_NonMemberActingTenantGets404NotUnauthorized is
// the negative half of the same wiring: a caller who states a tenant they
// do NOT belong to must be refused as 404 (no store linked), never 401 —
// see require_tenant_claim.go's doc comment on why a 401 here is a real
// incident (mobile client signOut()+redirect loop). This also confirms
// TenantFromRequest's fail-closed default: an unresolved header must NOT
// leak through as a usable tenant_id.
func TestRegisterAdminMobile_NonMemberActingTenantGets404NotUnauthorized(t *testing.T) {
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
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1", TenantID: ""},
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

// TestRegisterAdminMobile_NoActingTenantHeaderUnaffectedByTenantFromRequest
// is the "byte-identical when unused" guarantee: a caller that never
// sends X-Acting-Tenant-Id (every pre-existing test, and every GIP token
// that already carries its own tenant_id claim) must behave exactly as
// before — TenantFromRequest is a no-op when the header is absent, even
// with a real TenantMembershipChecker wired.
func TestRegisterAdminMobile_NoActingTenantHeaderUnaffectedByTenantFromRequest(t *testing.T) {
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
		// The token itself already carries tenant-1 (the GIP shape),
		// same as every existing mobile_routes test.
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	// Deliberately no X-Acting-Tenant-Id header.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
