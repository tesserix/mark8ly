package storefront_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- GIPBearerAuth tests ---

func TestGIPBearerAuth_MissingToken_Returns401(t *testing.T) {
	r := gin.New()
	r.Use(storefront.GIPBearerAuth(true))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "bearer token required")
}

func TestGIPBearerAuth_EmptyBearer_Returns401(t *testing.T) {
	r := gin.New()
	r.Use(storefront.GIPBearerAuth(true))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// GIPBearerAuth is a scaffold guard — it no longer exposes a devMode
// accept-any-token shortcut (previously: any non-empty Bearer token was
// accepted when devMode=true, which would be catastrophic if the flag
// leaked into production). Callers must chain a real verifier
// (MobileCustomerAuth) after this middleware; the test suite validates
// that contract below.
func TestGIPBearerAuth_DevMode_DoesNotSetIdentityWithoutVerifier(t *testing.T) {
	r := gin.New()
	r.Use(storefront.GIPBearerAuth(true)) // devMode param now ignored
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"gip_uid": c.GetString("customer_gip_uid"),
			"email":   c.GetString("customer_email"),
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	r.ServeHTTP(w, req)

	// No verifier chained → no identity leaked into context. Reaching the
	// handler must not expose any dev-* placeholder values.
	assert.Equal(t, 200, w.Code)
	assert.NotContains(t, w.Body.String(), "dev-uid")
	assert.NotContains(t, w.Body.String(), "dev@example.com")
}

func TestGIPBearerAuth_MissingBearer_Returns401(t *testing.T) {
	r := gin.New()
	r.Use(storefront.GIPBearerAuth(false))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	// No Authorization header at all.
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// --- OptionalGIPBearerAuth tests ---

func TestOptionalGIPBearerAuth_MissingToken_ContinuesAsGuest(t *testing.T) {
	r := gin.New()
	r.Use(storefront.OptionalGIPBearerAuth(true))
	r.GET("/test", func(c *gin.Context) {
		gipUID := c.GetString("customer_gip_uid")
		c.JSON(200, gin.H{"has_auth": gipUID != ""})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"has_auth":false`)
}

// OptionalGIPBearerAuth no longer populates the context from a raw bearer
// token — that was the accept-any-token pattern. Identity must come from a
// verifier-backed middleware (MobileCustomerAuth) chained after this one.
func TestOptionalGIPBearerAuth_DevMode_DoesNotSetIdentityWithoutVerifier(t *testing.T) {
	r := gin.New()
	r.Use(storefront.OptionalGIPBearerAuth(true)) // devMode param now ignored
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"gip_uid": c.GetString("customer_gip_uid"),
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.NotContains(t, w.Body.String(), "dev-uid")
}

// --- MobileCustomerAuth tests ---

type fakeVerifier struct {
	gipUID string
	email  string
	err    error
}

func (f *fakeVerifier) VerifyCustomerToken(_ string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.gipUID, f.email, nil
}

func TestMobileCustomerAuth_MissingToken_Returns401(t *testing.T) {
	r := gin.New()
	r.Use(storefront.MobileCustomerAuth(&fakeVerifier{}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestMobileCustomerAuth_ValidToken_SetsContext(t *testing.T) {
	r := gin.New()
	r.Use(storefront.MobileCustomerAuth(&fakeVerifier{
		gipUID: "uid-abc",
		email:  "test@example.com",
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"gip_uid": c.GetString("customer_gip_uid"),
			"email":   c.GetString("customer_email"),
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "uid-abc")
	assert.Contains(t, w.Body.String(), "test@example.com")
}

func TestMobileCustomerAuth_InvalidToken_Returns401(t *testing.T) {
	r := gin.New()
	r.Use(storefront.MobileCustomerAuth(&fakeVerifier{
		err: assert.AnError,
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}
