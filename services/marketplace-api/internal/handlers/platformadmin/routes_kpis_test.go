package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// Any one of the three KPI dependencies missing must leave /admin/kpis
// unmounted rather than panic at request time — a partial handler is worse
// than no handler, so the mount itself must not happen.
func TestRegisterKPIsIsNilSafeWhenAnyDepMissing(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	cases := map[string]platformadmin.Deps{
		"all missing": {Repo: &stubRepo{}},
		"estate only": {Repo: &stubRepo{}, EstateCounts: estate},
		"funnel only": {Repo: &stubRepo{}, OnboardingFunnel: funnel},
		"subs only":   {Repo: &stubRepo{}, Subscriptions: subs},
		"missing subs": {
			Repo: &stubRepo{}, EstateCounts: estate, OnboardingFunnel: funnel,
		},
	}

	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			platformadmin.Register(r.Group("/api/v1/platform"), deps)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
				"/api/v1/platform/admin/kpis", nil))
			require.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

// Mounted but with no secret: 503 not_configured, same as every other
// route on this surface.
func TestKPIsFailsClosedWithoutSecret(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:             &stubRepo{},
		EstateCounts:     estate,
		OnboardingFunnel: funnel,
		Subscriptions:    subs,
		Secret:           "",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/kpis", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}
