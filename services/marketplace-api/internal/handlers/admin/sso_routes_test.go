package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

// ssoRouterForRole mounts the real admin routes with an FGA fake that grants
// the caller exactly role on the tenant, so the assertions exercise the
// production middleware chain rather than a hand-rolled copy of it.
func ssoRouterForRole(t *testing.T, role authz.Role, userID, tenantID string) *gin.Engine {
	t.Helper()
	fga := authz.NewFakeClient()
	fga.Grant(userID, role, tenantID)
	r := gin.New()
	RegisterAdmin(r.Group("/"), Deps{
		SSOConfigHandler: NewSSOConfigHandler(nil, nil, nil),
		AuthzMiddleware:  authz.NewMiddleware(fga, nil),
	})
	return r
}

func ssoRoutePaths() [][2]string {
	return [][2]string{
		{http.MethodGet, "/sso/config"},
		{http.MethodPost, "/sso/config"},
		{http.MethodDelete, "/sso/config"},
		{http.MethodPost, "/sso/test"},
	}
}

// Every SSO route must be registered — otherwise the 404 assertions below
// would pass against a router that simply has no such routes.
func TestSSORoutes_AreRegistered(t *testing.T) {
	r := ssoRouterForRole(t, authz.RoleOwner, uuid.NewString(), uuid.NewString())
	registered := map[string]bool{}
	for _, rt := range r.Routes() {
		registered[rt.Method+" "+rt.Path] = true
	}
	for _, p := range ssoRoutePaths() {
		key := p[0] + " /admin/tenants/:tenantId" + p[1]
		require.True(t, registered[key], "route not registered: %s", key)
	}
}

// A tenant member with only viewer rights must not read or overwrite the
// tenant's identity-provider configuration.
func TestSSORoutes_ViewerDenied(t *testing.T) {
	userID, tenantID := uuid.NewString(), uuid.NewString()
	r := ssoRouterForRole(t, authz.RoleViewer, userID, tenantID)

	for _, p := range ssoRoutePaths() {
		req := httptest.NewRequest(p[0], "/admin/tenants/"+tenantID+p[1], nil)
		req.Header.Set("X-User-Id", userID)
		req.Header.Set("X-Tenant-Id", tenantID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "%s %s", p[0], p[1])
	}
}

// Staff and admin may not rewrite the IdP config either — changing it
// redirects the whole tenant's authentication.
func TestSSORoutes_MutationsRequireOwner(t *testing.T) {
	for _, role := range []authz.Role{authz.RoleStaff, authz.RoleAdmin} {
		userID, tenantID := uuid.NewString(), uuid.NewString()
		r := ssoRouterForRole(t, role, userID, tenantID)

		for _, p := range ssoRoutePaths() {
			if p[0] == http.MethodGet {
				continue
			}
			req := httptest.NewRequest(p[0], "/admin/tenants/"+tenantID+p[1], nil)
			req.Header.Set("X-User-Id", userID)
			req.Header.Set("X-Tenant-Id", tenantID)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code, "%s %s as %s", p[0], p[1], role)
		}
	}
}
