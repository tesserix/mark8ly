package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// The route must be mounted, and must sit BEHIND the signature check. An
// unsigned request gets 401 (or 503 when the surface is unconfigured) —
// never 404, which would mean the route does not exist, and never 200,
// which would mean a cross-tenant read is open.
func TestOutboxRouteIsMountedBehindAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "test-secret",
		Outbox: platformadmin.OutboxListerFunc(nil),
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/outbox", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code, "route must be mounted")
	require.NotEqual(t, http.StatusOK, rec.Code, "route must not answer an unsigned request")
}

// A nil dependency leaves the route unmounted, matching the nil-safe
// pattern every other read route on this surface uses.
func TestOutboxRouteUnmountedWhenDependencyNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/outbox", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
