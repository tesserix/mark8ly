package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequireBoundTenant_BlocksEmptyTenant is the fail-closed half of the
// login-bounce fix. GIPBearerAuth deliberately admits a validly-signed token
// with no tenant_id; without this guard, routes that lack an explicit
// RequireTenantRelation (platform-support, push-tokens) would become
// reachable by a user with no tenant, carrying an empty tenant scope.
func TestRequireBoundTenant_BlocksEmptyTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reached := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", ""); c.Next() })
	r.Use(RequireBoundTenant())
	r.GET("/probe", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if reached {
		t.Fatal("handler ran with an empty tenant scope — fail-open")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	// 401 would make the mobile client signOut() and bounce to /login —
	// the exact loop this change set removes.
	if w.Code == http.StatusUnauthorized {
		t.Error("guard returned 401; must not trigger client-side sign-out")
	}
}

func TestRequireBoundTenant_AllowsBoundTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-1"); c.Next() })
	r.Use(RequireBoundTenant())
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a caller with a tenant", w.Code)
	}
}

// A completely absent key must behave like an empty one.
func TestRequireBoundTenant_BlocksMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequireBoundTenant())
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when tenant_id was never set", w.Code)
	}
}
