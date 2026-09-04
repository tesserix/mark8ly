package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// stubVerifier lets us exercise GIPBearerAuth's status-code contract
// without a real Firebase client.
type stubVerifier struct {
	claims *TokenClaims
	err    error
}

func (s stubVerifier) Verify(_ context.Context, _ string) (*TokenClaims, error) {
	return s.claims, s.err
}

// TestGIPBearerAuth_NoTenantClaimIsNot401 is the regression test for the
// mobile-admin login bounce.
//
// A validly-signed token whose user simply has no store must NOT be
// rejected as an authentication failure. It previously returned 401
// "invalid or expired token", which the mobile client interprets as a
// dead session — it calls signOut() and the router redirects to /login,
// producing an endless oscillation with no error shown. Every tenant
// onboarded through the wizard hit this, because nothing ever set the
// tenant_id custom claim.
//
// Correct behaviour: authenticate fine with no tenant bound to the
// context, and let auth.RequireBoundTenant (or authz.RequireTenantRelation)
// answer 404, which the app renders as its existing "No store yet" screen.
func TestGIPBearerAuth_NoTenantClaimIsNot401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{claims: &TokenClaims{UserID: "uid-no-store"}}))
	r.GET("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":   c.GetString("user_id"),
			"tenant_id": c.GetString("tenant_id"),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer any-validly-signed-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("missing tenant_id returned 401 — the mobile client will signOut() and bounce to /login")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (auth succeeds; authorization is decided downstream)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "uid-no-store") {
		t.Errorf("user_id not propagated to context: %s", w.Body.String())
	}
}

// A genuinely bad token must still be 401.
func TestGIPBearerAuth_InvalidTokenIsStill401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{err: ErrInvalidToken}))
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an invalid token", w.Code)
	}
}
