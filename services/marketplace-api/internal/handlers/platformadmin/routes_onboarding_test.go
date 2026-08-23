package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// A nil OnboardingFunnel must leave the routes unmounted rather than panic
// at request time — matching the nil-safe pattern for TenantDirectory.
func TestRegisterOnboardingFunnelIsNilSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo: &stubRepo{},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/onboarding/funnel", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// Mounted but with no secret: 503 not_configured, same as every other route
// on this surface.
func TestOnboardingFunnelFailsClosedWithoutSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:             &stubRepo{},
		OnboardingFunnel: &stubFunnelClient{},
		Secret:           "",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/onboarding/funnel", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}
