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
		TokenVerifier: &auth.FakeVerifier{UserID: "user-1"},
		// GIPBearerAuth never sets tenant_id from the token at all, so
		// reaching the handler at all proves TenantFromRequest resolved
		// tenant_id from the X-Acting-Tenant-Id header + FGA membership
		// check, in the right slot in the chain.
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
// see require_bound_tenant.go's doc comment on why a 401 here is a real
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

// TestRegisterAdminMobile_NoActingTenantHeaderMeans404EvenIfVerifierKnowsATenant
// is the task-4 regression test: a bearer token's own opinion about the
// caller's tenant must never reach "tenant_id" on the context, no matter
// what value it carries or how it got there. Before task 4, GIPBearerAuth
// copied TokenClaims.TenantID straight onto the context; a token whose
// claim happened to name a tenant the caller legitimately belongs to (as
// this test's FakeVerifier would have, pre-task-4) reached the handler
// with NO X-Acting-Tenant-Id header and NO FGA check at all — the exact
// unvalidated-claim-wins-the-race bug this task removes. TokenClaims no
// longer has a field to carry that value, so with no acting-tenant header
// there is now no possible source for tenant_id and the caller is refused
// as 404, even though the same user IS a genuine FGA member of tenant-1
// (proving this is the claim path being closed, not a membership
// failure).
func TestRegisterAdminMobile_NoActingTenantHeaderMeans404EvenIfVerifierKnowsATenant(t *testing.T) {
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
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	// Deliberately no X-Acting-Tenant-Id header — the only remaining
	// source of tenancy is absent, so even a genuine FGA member must be
	// refused rather than let some other signal (a token claim) fill in.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
