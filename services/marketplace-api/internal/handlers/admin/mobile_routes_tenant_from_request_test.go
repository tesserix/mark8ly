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

// TestRegisterAdminMobile_TenantFromRequestRunsBetweenBearerAuthAndRequireTenant
// pins the exact chain order for the mobile admin group:
//
//	bearerAuth (no tenant write) -> tenant-from-request -> requireTenant -> ... -> rateLimiter
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
		// BearerAuth never sets tenant_id from the token, so reaching
		// the handler at all proves TenantFromRequest resolved it from the
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

// TestRegisterAdminMobile_NonMemberActingTenantGets404NotUnauthorized
// is the negative half of the same wiring: a caller who states a tenant
// they do NOT belong to must be refused as 404 (no store linked), never
// 401 — see require_bound_tenant.go's doc comment on why a 401 here is a
// real incident (mobile client signOut()+redirect loop). This also
// confirms TenantFromRequest's fail-closed default: an unresolved header
// must NOT leak through as a usable tenant_id.
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

// TestRegisterAdminMobile_NoActingTenantHeaderMeans404 proves
// TenantFromRequest is the ONLY source of tenancy: with no
// X-Acting-Tenant-Id header, the caller is refused 404 even though the
// same user IS a genuine FGA member of tenant-1 — since #786 there is no
// mechanism left to supply a tenant without the header.
func TestRegisterAdminMobile_NoActingTenantHeaderMeans404(t *testing.T) {
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
	// Deliberately no X-Acting-Tenant-Id header.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestRegisterAdminMobile_TokenClaimNeverOverridesValidatedHeader is THE
// safety test for the single-writer contract, carried forward from the
// dual-issuer suite deleted in #786.
//
// A token may still ASSERT a tenant — auth.TokenClaims keeps the field,
// and a future verifier could populate it — but that assertion is
// unvalidated input. The X-Acting-Tenant-Id header has been checked
// against real FGA membership. If the claim ever wins, an unvalidated
// value overrides a validated decision: the precise bug #524 phase 4
// removed, and the one thing collapsing to a single issuer must not
// quietly reintroduce.
func TestRegisterAdminMobile_TokenClaimNeverOverridesValidatedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	fga := authz.NewFakeClient()
	// The caller is a genuine member of both, so the route's own
	// RequireTenantRelation check passes either way and the assertion
	// below discriminates the RESOLVED TENANT, not merely a status code.
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
		// The token claims tenant-1; the validated header states tenant-2.
		TokenVerifier:           &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
		TenantMembershipChecker: fga,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set(auth.ActingTenantHeader, "tenant-2")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "tenant-2", recorder.gotTenantID,
		"tenant_id must be the FGA-validated tenant-2, never the token's claimed tenant-1")
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
