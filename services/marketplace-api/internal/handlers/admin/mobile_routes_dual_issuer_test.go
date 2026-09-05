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

// The three tests below pin the tenancy contract for DUAL-ISSUER mode
// (#686), the configuration that lets a Zitadel-capable app release and
// already-installed GIP apps work at the same time.
//
// The incumbent design made the two tenancy sources mutually exclusive per
// DEPLOYMENT (ZitadelEnabled picked one). That cannot express "both kinds
// of token are in flight", which is exactly what a store-app rollout is —
// old installs cannot be forced to update. Dual mode makes the choice
// per TOKEN instead: a GIP token carries a tenant claim and uses it, a
// Zitadel token carries none and resolves via the FGA-checked header.
//
// The safety invariant that replaces mutual exclusion is ORDERING: an
// FGA-VALIDATED value may overwrite an unvalidated claim, never the
// reverse. TestDualIssuer_ValidatedHeaderBeatsUnvalidatedClaim is what
// holds that line.

// dualIssuerRouter builds the mobile admin routes in dual-issuer mode and
// captures the tenant_id the middleware chain actually resolved, so tests
// can assert on the RESOLVED VALUE rather than only on a status code —
// status alone cannot tell tenant-1 from tenant-2.
func dualIssuerRouter(t *testing.T, fga *authz.FakeClient, verifier auth.TokenVerifier, seen *string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   NewStoresHandler(&stubStoresRepo{}, nil),
			AuthzMiddleware: authz.NewMiddleware(fga, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
			// TenantGate sits inside tenantMW, immediately after
			// TenantFromRequest and requireTenant, so it observes the
			// FINAL resolved tenant_id. StoresMiddleware would not: it is
			// mounted only on the store-scoped group, not on
			// /mobile/admin/stores, so capturing there silently records
			// nothing and the assertion passes vacuously.
			TenantGate: func(c *gin.Context) {
				if seen != nil {
					*seen = c.GetString("tenant_id")
				}
				c.Next()
			},
		},
		ZitadelEnabled:          true,
		DualIssuer:              true,
		TokenVerifier:           verifier,
		TenantMembershipChecker: fga,
	})
	return r
}

// A GIP token still works with NO acting-tenant header. This is the
// already-installed app, and it is the whole reason dual mode exists: in
// the incumbent ZitadelEnabled=true wiring this request 404s, because the
// claim write is switched off and the old client sends no header.
func TestDualIssuer_GIPTokenResolvesTenantFromItsClaim(t *testing.T) {
	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	r := dualIssuerRouter(t, fga, &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer gip-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"an existing GIP install sends no X-Acting-Tenant-Id; its claim must still resolve a tenant: %s", rec.Body.String())
}

// A Zitadel token carries no claim, so it must resolve through the
// FGA-checked header — the new app's path, unchanged from Zitadel mode.
func TestDualIssuer_ZitadelTokenResolvesTenantFromHeader(t *testing.T) {
	fga := authz.NewFakeClient()
	fga.Grant("user-1", authz.RoleStaff, "tenant-1")

	r := dualIssuerRouter(t, fga, &auth.FakeVerifier{UserID: "user-1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer zitadel-token")
	req.Header.Set(auth.ActingTenantHeader, "tenant-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"a Zitadel token has no tenant claim and must resolve via the FGA-checked header: %s", rec.Body.String())
}

// THE SAFETY TEST. With both a token claim and a stated header present,
// the FGA-VALIDATED header must win. The claim is unvalidated input; the
// header has been checked against real membership. If this ever inverts,
// an unvalidated claim can override a validated decision — the precise
// bug the old mutual-exclusion rule existed to prevent, and the one thing
// dual mode must not reintroduce.
func TestDualIssuer_ValidatedHeaderBeatsUnvalidatedClaim(t *testing.T) {
	fga := authz.NewFakeClient()
	// The caller is a genuine member of tenant-2 only. Their token claims
	// tenant-1, which they are NOT a member of.
	fga.Grant("user-1", authz.RoleStaff, "tenant-2")

	var resolved string
	r := dualIssuerRouter(t, fga, &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"}, &resolved)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer gip-token")
	req.Header.Set(auth.ActingTenantHeader, "tenant-2")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"the FGA-validated header must resolve, overriding the token's unvalidated claim: %s", rec.Body.String())
	require.Equal(t, "tenant-2", resolved,
		"tenant_id must be the FGA-validated tenant-2, never the claimed tenant-1")
}
