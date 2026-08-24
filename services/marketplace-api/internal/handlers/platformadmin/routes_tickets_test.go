package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// A nil Tickets dependency must leave /admin/tickets unmounted rather than
// panic at request time — matching the nil-safe pattern for TenantDirectory,
// OnboardingFunnel, and the KPI trio.
func TestRegisterTicketsIsNilSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo: &stubRepo{},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/tickets", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// The converse: with Tickets set (and a secret, so the route answers rather
// than 503s), the route IS mounted. This catches a guard that always
// refuses just as readily as it catches a missing guard.
func TestRegisterTicketsMountsWhenDepPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:    &stubRepo{},
		Tickets: &stubTicketLister{},
		Secret:  "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/tickets", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when Tickets is set")
}
