package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/stretchr/testify/assert"
)

func gipRouter(verifier auth.TokenVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.GIPBearerAuth(verifier))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"user_id":   c.GetString("user_id"),
			"tenant_id": c.GetString("tenant_id"),
		})
	})
	return r
}

func TestGIPBearerAuth_NoAuthHeader_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrNoToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestGIPBearerAuth_ValidToken_SetsContext(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{
		UserID:   "user-123",
		TenantID: "tenant-456",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.Contains(t, w.Body.String(), "tenant-456")
}

func TestGIPBearerAuth_InvalidToken_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrInvalidToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}
