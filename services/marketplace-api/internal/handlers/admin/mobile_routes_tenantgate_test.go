package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
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
func TestRegisterAdminMobile_SuspendedTenantRefusedOnNonStoreRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	gate := tenantgate.New(&stubTenantLookup{status: "suspended"}, nil, time.Minute)

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			StoresHandler:   &StoresHandler{},
			AuthzMiddleware: authz.NewMiddleware(nil, nil),
			TenantGate:      gate.RequireActiveTenant(),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		TokenVerifier: &auth.FakeVerifier{UserID: "user-1", TenantID: "tenant-1"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/admin/stores", nil)
	req.Header.Set("Authorization", "Bearer fake")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code,
		"a suspended tenant must be refused on the mobile route, exactly as on the web admin table")
	require.Contains(t, rec.Body.String(), "tenant_suspended")
}
