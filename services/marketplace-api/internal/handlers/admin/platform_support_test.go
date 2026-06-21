package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottoclient"
)

// TestPlatformSupport_PinsPlatformTenantAndAttributesMerchant covers #119:
// a merchant admin opens a platform chat. The bridge must pin otto to the
// platform tenant/store and carry the merchant's own tenant as
// X-Client-Tenant-Id so the Tesserix agent sees who is talking.
func TestPlatformSupport_PinsPlatformTenantAndAttributesMerchant(t *testing.T) {
	var sawTenant, sawStore, sawUser, sawClientTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawTenant = r.Header.Get("X-Tenant-Id")
		sawStore = r.Header.Get("X-Store-Id")
		sawUser = r.Header.Get("X-User-Id")
		sawClientTenant = r.Header.Get("X-Client-Tenant-Id")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"conversation":{"id":"p1"}}`))
	}))
	defer srv.Close()

	gin.SetMode(gin.TestMode)
	h := NewPlatformSupportHandler(ottoclient.New(srv.URL, "secret"), "wss://api.test", nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Simulates the admin GIP bearer auth middleware.
		c.Set("user_id", "admin-1")
		c.Set("tenant_id", "merchant-9")
		c.Next()
	})
	h.Register(r.Group("/platform-support"))

	rec := httptest.NewRecorder()
	body := `{"message":"billing help","reason":"billing","status_info":"x"}`
	req, _ := http.NewRequest(http.MethodPost, "/platform-support/conversations", strings.NewReader(body))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if sawTenant != "platform" || sawStore != "default" {
		t.Errorf("scope: tenant=%q store=%q, want platform/default", sawTenant, sawStore)
	}
	if sawUser != "admin-1" {
		t.Errorf("X-User-Id=%q, want admin-1", sawUser)
	}
	if sawClientTenant != "merchant-9" {
		t.Errorf("X-Client-Tenant-Id=%q, want merchant-9", sawClientTenant)
	}
}
