package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/stretchr/testify/assert"
)

func gipRouter(verifier auth.TokenVerifier, setTenantFromClaim bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.GIPBearerAuth(verifier, setTenantFromClaim))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"user_id":   c.GetString("user_id"),
			"tenant_id": c.GetString("tenant_id"),
		})
	})
	return r
}

func TestGIPBearerAuth_NoAuthHeader_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrNoToken}, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

// TestGIPBearerAuth_ValidToken_SetsContext is the GIP-mode
// (setTenantFromClaim=true) case: user_id AND tenant_id both come from the
// verified claims, byte-identical to before #524 phase 4.
func TestGIPBearerAuth_ValidToken_SetsContext(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{
		UserID:   "user-123",
		TenantID: "tenant-456",
	}, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.Contains(t, w.Body.String(), "tenant-456")
}

// TestGIPBearerAuth_ValidToken_ZitadelMode_NeverSetsTenantID is the
// Zitadel-mode (setTenantFromClaim=false) case: even a claim carrying a
// real tenant value must never reach "tenant_id" — only
// TenantFromRequest's FGA-validated result may, mounted separately.
func TestGIPBearerAuth_ValidToken_ZitadelMode_NeverSetsTenantID(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{
		UserID:   "user-123",
		TenantID: "attacker-chosen-tenant",
	}, false)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.NotContains(t, w.Body.String(), "attacker-chosen-tenant")
	assert.Contains(t, w.Body.String(), `"tenant_id":""`)
}

func TestGIPBearerAuth_InvalidToken_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrInvalidToken}, true)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}
