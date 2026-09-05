package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

type fakeTenantLister struct {
	got    string
	result []teamproxy.TenantMembership
	err    error
}

func (f *fakeTenantLister) ListMyTenants(_ context.Context, id string) ([]teamproxy.TenantMembership, error) {
	f.got = id
	return f.result, f.err
}

// THE test for this route. Every other mobile admin route is behind
// RequireBoundTenant, which 404s a caller with no tenant. This one MUST
// NOT be — it is how a Zitadel-authenticated client discovers the tenant
// it will then send as X-Acting-Tenant-Id. Gate it and the mobile Zitadel
// flow deadlocks: no tenant, therefore no way to learn the tenant.
func TestMobileMyTenants_IsReachableWithoutABoundTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{
		{TenantID: "e638b731", Name: "Mumbai Spice Co", Role: "owner"},
	}}

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
			StoresMiddleware: func(c *gin.Context) { c.Next() },
		},
		ZitadelEnabled: true,
		DualIssuer:     true,
		// No tenant claim, and NO X-Acting-Tenant-Id on the request:
		// exactly the state a client is in immediately after signing in.
		TokenVerifier:           &auth.FakeVerifier{UserID: "389396765696066342"},
		TenantMembershipChecker: authz.NewFakeClient(),
		MyTenantsHandler:        NewMobileMyTenantsHandler(lister, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/me/tenants", nil)
	req.Header.Set("Authorization", "Bearer zitadel-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"tenant discovery must not require a tenant, or the Zitadel mobile flow can never start: %s", rec.Body.String())

	var body struct {
		Data []teamproxy.TenantMembership `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Equal(t, "e638b731", body.Data[0].TenantID)

	// The identity must come from the VERIFIED token, never from anything
	// the caller supplies — this endpoint would otherwise be an
	// arbitrary-identity membership lookup.
	require.Equal(t, "389396765696066342", lister.got)
}

// Authentication is still required: it is only the TENANT gate that is
// lifted, not the bearer check.
func TestMobileMyTenants_StillRequiresAValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
			StoresMiddleware: func(c *gin.Context) { c.Next() },
		},
		ZitadelEnabled:   true,
		DualIssuer:       true,
		TokenVerifier:    &auth.FakeVerifier{Err: auth.ErrInvalidToken},
		MyTenantsHandler: NewMobileMyTenantsHandler(&fakeTenantLister{}, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/me/tenants", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// A user with no memberships gets 200 + [], not 404. The client shows
// "finish onboarding", which is a different screen from "something broke".
func TestMobileMyTenants_ZeroTenantsIsAnEmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
			StoresMiddleware: func(c *gin.Context) { c.Next() },
		},
		ZitadelEnabled:   true,
		DualIssuer:       true,
		TokenVerifier:    &auth.FakeVerifier{UserID: "u-new"},
		MyTenantsHandler: NewMobileMyTenantsHandler(&fakeTenantLister{result: nil}, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/me/tenants", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[]}`, rec.Body.String(),
		"zero tenants must serialise as [], never null — the client's schema rejects null")
}

// platform-api being down must not read as "you have no stores", which
// would send a legitimate merchant to the onboarding screen.
func TestMobileMyTenants_PlatformFailureIsNotAnEmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
			StoresMiddleware: func(c *gin.Context) { c.Next() },
		},
		ZitadelEnabled:   true,
		DualIssuer:       true,
		TokenVerifier:    &auth.FakeVerifier{UserID: "u-1"},
		MyTenantsHandler: NewMobileMyTenantsHandler(&fakeTenantLister{err: errors.New("platform down")}, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/me/tenants", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code,
		"an upstream failure must be distinguishable from an empty membership list")
}
