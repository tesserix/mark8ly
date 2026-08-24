package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// With no secret configured the surface must be inert — 503, not 200 and not
// 404. This is what makes rollout steps 2 and 3 safe to separate.
func TestRegisterMountsBehindAuthAndFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/audit-logs", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}

// A nil repo must leave the routes unmounted rather than panic at request
// time — matching the nil-safe pattern used for optional admin handlers.
func TestRegisterIsNilSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	require.NotPanics(t, func() {
		platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/audit-logs", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// /admin/health mounts whenever the surface itself mounts. It needs only
// DB, so unlike the client-backed routes it has no nil-dependency guard.
func TestRegisterMountsHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "test-secret",
	})

	found := false
	for _, route := range r.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/platform/admin/health" {
			found = true
		}
	}
	require.True(t, found, "/admin/health must mount")
}
