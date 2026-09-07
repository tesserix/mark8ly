package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/stretchr/testify/assert"
)

func bearerRouter(verifier auth.TokenVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.BearerAuth(verifier))
	r.GET("/test", func(c *gin.Context) {
		_, tenantSet := c.Get("tenant_id")
		c.JSON(200, gin.H{
			"user_id":          c.GetString("user_id"),
			"tenant_id":        c.GetString("tenant_id"),
			"tenant_id_exists": tenantSet,
		})
	})
	return r
}

func TestBearerAuth_NoAuthHeader_Returns401(t *testing.T) {
	r := bearerRouter(&auth.FakeVerifier{Err: auth.ErrNoToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

// A verified token puts the caller's identity — and only their identity —
// on the context.
func TestBearerAuth_ValidToken_SetsUserID(t *testing.T) {
	r := bearerRouter(&auth.FakeVerifier{UserID: "user-123"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.Contains(t, w.Body.String(), `"tenant_id_exists":false`)
}

// TestBearerAuth_TokenTenantClaim_NeverReachesContext is the core
// regression test for the two-writers bug (#524 phase 4, blocking-fix
// round), carried forward unchanged in intent from the GIP era (#786):
// a claim carrying a real tenant value must still never reach the
// context — only TenantFromRequest's FGA-validated result may. With GIP
// gone this is no longer one of two configurations; it is the ONLY
// behaviour BearerAuth has, which is why the assertion is now
// unconditional.
func TestBearerAuth_TokenTenantClaim_NeverReachesContext(t *testing.T) {
	r := bearerRouter(&auth.FakeVerifier{
		UserID:   "user-123",
		TenantID: "attacker-chosen-tenant",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.NotContains(t, w.Body.String(), "attacker-chosen-tenant",
		"a token's own tenant assertion must never become the request's tenant")
	assert.Contains(t, w.Body.String(), `"tenant_id_exists":false`,
		"BearerAuth must not set the tenant_id key at all")
}

// TestBearerAuth_NoTenantIsNot401 is the regression test for the
// mobile-admin login bounce, ported from the deleted GIP verifier's test
// file because the behaviour it guards is BearerAuth's, not the
// verifier's.
//
// A validly-signed token whose user simply has no store must NOT be
// rejected as an authentication failure. It previously returned 401
// "invalid or expired token", which the mobile client interprets as a
// dead session — it calls signOut() and the router redirects to /login,
// producing an endless oscillation with no error shown.
//
// Correct behaviour: authenticate fine, and let auth.RequireBoundTenant
// (or authz.RequireTenantRelation) answer 404, which the app already
// renders as its "No store yet" screen.
func TestBearerAuth_NoTenantIsNot401(t *testing.T) {
	r := bearerRouter(&auth.FakeVerifier{UserID: "uid-no-store"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer any-validly-signed-token")
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code,
		"an unbound caller must not get 401 — the mobile client will signOut() and bounce to /login")
	assert.Equal(t, http.StatusOK, w.Code,
		"auth succeeds; authorization is decided downstream")
	assert.Contains(t, w.Body.String(), "uid-no-store")
}

func TestBearerAuth_InvalidToken_Returns401(t *testing.T) {
	r := bearerRouter(&auth.FakeVerifier{Err: auth.ErrInvalidToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}
