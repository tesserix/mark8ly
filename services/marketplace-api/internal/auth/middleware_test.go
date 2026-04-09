package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
)

func init() { gin.SetMode(gin.TestMode) }

func newRouter(secret string) (*gin.Engine, *struct{ userID, tenantID string }) {
	captured := &struct{ userID, tenantID string }{}
	r := gin.New()
	r.Use(auth.HeaderTrustAuth(secret))
	r.GET("/x", func(c *gin.Context) {
		captured.userID = c.GetString("user_id")
		captured.tenantID = c.GetString("tenant_id")
		c.Status(http.StatusOK)
	})
	return r, captured
}

func do(r *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHeaderTrustAuth_HappyPath(t *testing.T) {
	r, captured := newRouter("")
	w := do(r, map[string]string{"X-User-Id": "u1", "X-Tenant-Id": "t1"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if captured.userID != "u1" || captured.tenantID != "t1" {
		t.Fatalf("captured = %+v", captured)
	}
}

func TestHeaderTrustAuth_MissingUserID_Returns401(t *testing.T) {
	r, _ := newRouter("")
	w := do(r, map[string]string{"X-Tenant-Id": "t1"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHeaderTrustAuth_MissingTenantID_Returns401(t *testing.T) {
	r, _ := newRouter("")
	w := do(r, map[string]string{"X-User-Id": "u1"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHeaderTrustAuth_NoInternalSecret_AcceptsAnything(t *testing.T) {
	r, _ := newRouter("")
	w := do(r, map[string]string{"X-User-Id": "u1", "X-Tenant-Id": "t1"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHeaderTrustAuth_InternalSecretMismatch_Returns401(t *testing.T) {
	r, _ := newRouter("s3cret")
	w := do(r, map[string]string{"X-User-Id": "u1", "X-Tenant-Id": "t1", "X-Internal-Auth": "wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHeaderTrustAuth_InternalSecretMatch_AllowsRequest(t *testing.T) {
	r, _ := newRouter("s3cret")
	w := do(r, map[string]string{"X-User-Id": "u1", "X-Tenant-Id": "t1", "X-Internal-Auth": "s3cret"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
