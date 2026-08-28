package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// breakGlassMountedRouter builds the REAL router, via platformadmin.Register,
// with exactly the dependencies GET /admin/break-glass needs to mount:
// BreakGlass and TenantDirectory (routes.go's mount guard), alongside the
// base auth wiring every route on this surface sits behind.
func breakGlassMountedRouter(t *testing.T, nonces platformadmin.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:            &stubRepo{},
		Secret:          testSecret,
		NonceStore:      nonces,
		TenantDirectory: &stubBreakGlassDirectory{},
		BreakGlass: platformadmin.BreakGlassListerFunc(
			func(_ context.Context, _ *gorm.DB, _ breakglass.PlatformListFilter,
				_ time.Time) (breakglass.PlatformListResult, error) {
				return breakglass.PlatformListResult{}, nil
			}),
	})
	return r
}

// TestBreakGlassRouteHonoursTheReadCapabilityGate is the full-surface proof
// that GET /api/v1/platform/admin/break-glass is mounted AND sits behind
// RequiredReadCapabilities' declared "rotate-credentials" requirement — not
// merely that the map has the right entry (routes_capability_coverage_test.go
// covers that), but that a real signed request is actually admitted or
// refused by it.
//
// "platform" is asserted 403, not just "any wrong value", because it is the
// value the console's audit module sends TODAY (see RequiredReadCapabilities'
// doc comment in middleware.go) — the exact near-miss that would be easy to
// wave through with a lattice or prefix match, which this surface
// deliberately does not implement (#275).
func TestBreakGlassRouteHonoursTheReadCapabilityGate(t *testing.T) {
	r := breakGlassMountedRouter(t, newMemNonces())

	okRec := httptest.NewRecorder()
	r.ServeHTTP(okRec, signedRequest(t, http.MethodGet,
		"/api/v1/platform/admin/break-glass", nil,
		withCapability("rotate-credentials"), withCurrentTimestamp))
	require.Equal(t, http.StatusOK, okRec.Code, okRec.Body.String())

	forbiddenRec := httptest.NewRecorder()
	r.ServeHTTP(forbiddenRec, signedRequest(t, http.MethodGet,
		"/api/v1/platform/admin/break-glass", nil,
		withCapability("platform"), withCurrentTimestamp))
	require.Equal(t, http.StatusForbidden, forbiddenRec.Code, forbiddenRec.Body.String())
}
