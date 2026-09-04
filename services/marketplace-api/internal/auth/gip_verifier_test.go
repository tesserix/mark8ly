package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// tenant-service writes tenant_id as a multi-value array (its
// UpdateUserAttributes appends into []interface{}), so the array form is what
// production tokens actually carry. The scalar form is tolerated defensively.
func TestTenantIDFromClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]interface{}
		want   string
	}{
		{
			name:   "array form — what tenant-service actually writes",
			claims: map[string]interface{}{"tenant_id": []interface{}{"11111111-2222-3333-4444-555555555555"}},
			want:   "11111111-2222-3333-4444-555555555555",
		},
		{
			name:   "scalar form — tolerated",
			claims: map[string]interface{}{"tenant_id": "11111111-2222-3333-4444-555555555555"},
			want:   "11111111-2222-3333-4444-555555555555",
		},
		{
			name:   "multi-tenant user — first entry wins",
			claims: map[string]interface{}{"tenant_id": []interface{}{"tenant-a", "tenant-b"}},
			want:   "tenant-a",
		},
		{
			name:   "claim absent",
			claims: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "nil claims",
			claims: nil,
			want:   "",
		},
		{
			name:   "empty array",
			claims: map[string]interface{}{"tenant_id": []interface{}{}},
			want:   "",
		},
		{
			name:   "empty string in array is skipped",
			claims: map[string]interface{}{"tenant_id": []interface{}{"", "tenant-b"}},
			want:   "tenant-b",
		},
		{
			name:   "empty scalar",
			claims: map[string]interface{}{"tenant_id": ""},
			want:   "",
		},
		{
			name:   "wrong element type is ignored",
			claims: map[string]interface{}{"tenant_id": []interface{}{42, "tenant-b"}},
			want:   "tenant-b",
		},
		{
			name:   "wholly wrong type",
			claims: map[string]interface{}{"tenant_id": 42},
			want:   "",
		},
		{
			name:   "[]string form",
			claims: map[string]interface{}{"tenant_id": []string{"tenant-a"}},
			want:   "tenant-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantIDFromClaims(tt.claims); got != tt.want {
				t.Errorf("tenantIDFromClaims() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
// Correct behaviour: authenticate fine with an empty tenant_id, and let
// authz.RequireTenantRelation (or auth.RequireBoundTenant) answer 404,
// which the app already renders as its "No store yet" screen. This test
// runs GIPBearerAuth with setTenantFromClaim=true, the GIP/ZITADEL_ENABLED
// =false configuration this verifier is actually wired into.
func TestGIPBearerAuth_NoTenantClaimIsNot401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{claims: &TokenClaims{UserID: "uid-no-store", TenantID: ""}}, true))
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

// A genuinely bad token must still be 401, regardless of setTenantFromClaim.
func TestGIPBearerAuth_InvalidTokenIsStill401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{err: ErrInvalidToken}, true))
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an invalid token", w.Code)
	}
}

// TestGIPBearerAuth_SetTenantFromClaimFalse_NeverSetsTenantID is the core
// regression test for the two-writers bug (#524 phase 4, blocking-fix
// round): with setTenantFromClaim=false (the Zitadel/ZITADEL_ENABLED=true
// configuration), a claim carrying a real tenant value must still never
// reach the context — only TenantFromRequest's FGA-validated result may.
func TestGIPBearerAuth_SetTenantFromClaimFalse_NeverSetsTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{claims: &TokenClaims{UserID: "uid-1", TenantID: "attacker-chosen-tenant"}}, false))
	r.GET("/probe", func(c *gin.Context) {
		_, exists := c.Get("tenant_id")
		c.JSON(http.StatusOK, gin.H{
			"user_id":          c.GetString("user_id"),
			"tenant_id_exists": exists,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer any-validly-signed-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if strings.Contains(w.Body.String(), "attacker-chosen-tenant") {
		t.Fatalf("a claim tenant value leaked onto the context with setTenantFromClaim=false: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"tenant_id_exists":true`) {
		t.Errorf("tenant_id key was set at all with setTenantFromClaim=false: %s", w.Body.String())
	}
}

// TestGIPBearerAuth_SetTenantFromClaimTrue_UsesClaimValue is the
// complementary positive case: with setTenantFromClaim=true (the GIP
// configuration), the claim IS the source of tenancy, exactly as it was
// before #524 phase 4 — this is the byte-identical-to-origin/main
// guarantee for the flag-off path.
func TestGIPBearerAuth_SetTenantFromClaimTrue_UsesClaimValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GIPBearerAuth(stubVerifier{claims: &TokenClaims{UserID: "uid-1", TenantID: "tenant-from-claim"}}, true))
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

	if !strings.Contains(w.Body.String(), "tenant-from-claim") {
		t.Errorf("claim tenant value did not reach the context with setTenantFromClaim=true: %s", w.Body.String())
	}
}
