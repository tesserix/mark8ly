package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/middleware"
)

func strictRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", middleware.RequireInternalAuthStrict(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// The whole point of this middleware. RequireInternalAuth no-ops on an empty
// secret; this one must not, because the route it guards returns every
// tenant on the platform.
func TestStrictRefusesWhenSecretUnset(t *testing.T) {
	rec := httptest.NewRecorder()
	strictRouter("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "not_configured")
}

func TestStrictRefusesWrongSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Internal-Auth", "wrong")

	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStrictRefusesMissingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStrictAllowsCorrectSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Internal-Auth", "right")

	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

// The permissive variant must keep its escape hatch — other /internal routes
// rely on it during the cutover before the secret is provisioned.
func TestPermissiveStillNoOpsOnEmptySecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/y", middleware.RequireInternalAuth(""), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/y", nil))

	require.Equal(t, http.StatusOK, rec.Code)
}

// Restored from commit 889e5952: original RequireInternalAuth test coverage.
// These four tests ensure the permissive variant's behavior (match allows, missing/mismatch reject)
// is not changed by future modifications.

func init() { gin.SetMode(gin.TestMode) }

func newRouter(secret string) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequireInternalAuth(secret))
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func do(t *testing.T, r *gin.Engine, header string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if header != "" {
		req.Header.Set("X-Internal-Auth", header)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestRequireInternalAuth_EmptySecretAllows(t *testing.T) {
	if got := do(t, newRouter(""), ""); got != http.StatusOK {
		t.Fatalf("expected 200 with empty secret, got %d", got)
	}
}

func TestRequireInternalAuth_MatchAllows(t *testing.T) {
	if got := do(t, newRouter("s3cret"), "s3cret"); got != http.StatusOK {
		t.Fatalf("expected 200 on match, got %d", got)
	}
}

func TestRequireInternalAuth_MissingRejects(t *testing.T) {
	if got := do(t, newRouter("s3cret"), ""); got != http.StatusUnauthorized {
		t.Fatalf("expected 401 on missing header, got %d", got)
	}
}

func TestRequireInternalAuth_MismatchRejects(t *testing.T) {
	if got := do(t, newRouter("s3cret"), "wrong"); got != http.StatusUnauthorized {
		t.Fatalf("expected 401 on mismatch, got %d", got)
	}
}
