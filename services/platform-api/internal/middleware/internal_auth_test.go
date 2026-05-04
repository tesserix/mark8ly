package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func newRouter(secret string) *gin.Engine {
	r := gin.New()
	r.Use(RequireInternalAuth(secret))
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
