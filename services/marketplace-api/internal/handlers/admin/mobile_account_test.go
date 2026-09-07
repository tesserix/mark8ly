package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

// withIdentity injects the tenant_id + user_id context keys that
// BearerAuth sets on the mobile path, so handler tests don't need the
// real middleware.
func withIdentity(userID, tenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

// TestMobileAccount_Delete_Proxies204 covers the happy path: platform-api
// accepts the deletion (204), the handler forwards it as 204, and the
// internal-auth secret rode the outbound request.
func TestMobileAccount_Delete_Proxies204(t *testing.T) {
	var sawAuth, sawMethod, sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("X-Internal-Auth")
		sawMethod = r.Method
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	gin.SetMode(gin.TestMode)
	h := NewMobileAccountHandler(teamproxy.NewClient(srv.URL, "secret", nil), nil)
	r := gin.New()
	r.Use(withIdentity("u1", "t1"))
	r.DELETE("/account", h.Delete)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/account", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if sawAuth != "secret" {
		t.Errorf("X-Internal-Auth=%q, want secret", sawAuth)
	}
	if sawMethod != http.MethodDelete {
		t.Errorf("method=%q, want DELETE", sawMethod)
	}
	if sawPath != "/internal/tenants/t1/account" {
		t.Errorf("path=%q, want /internal/tenants/t1/account", sawPath)
	}
}

// TestMobileAccount_Delete_ForwardsPlatformError covers platform-api
// rejecting the deletion (e.g. sole-owner guard): the handler must forward
// the real status and error code rather than collapsing everything to a 502.
func TestMobileAccount_Delete_ForwardsPlatformError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"sole owner cannot self-delete"}`))
	}))
	defer srv.Close()

	gin.SetMode(gin.TestMode)
	h := NewMobileAccountHandler(teamproxy.NewClient(srv.URL, "secret", nil), nil)
	r := gin.New()
	r.Use(withIdentity("u1", "t1"))
	r.DELETE("/account", h.Delete)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/account", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if want := `"error":"forbidden"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body=%s, want to contain %s", rec.Body.String(), want)
	}
	if want := `sole owner cannot self-delete`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body=%s, want to contain %s", rec.Body.String(), want)
	}
}
