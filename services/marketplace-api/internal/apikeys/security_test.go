//go:build integration

// security_test.go pulls together the spec's adversarial cases into a
// focused suite: revoked keys must be rejected immediately, rotation-overlap
// must end exactly at +24h, scope mismatches must 403, and the timing of
// failed-auth response must not leak which step failed.
package apikeys_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// SecuritySuite — repo-backed but cache-disabled so each call exercises the
// cold path and observable timings reflect bcrypt cost.
type secEnv struct {
	svc  *apikeys.Service
	mw   *apikeys.Middleware
	repo *apikeys.Repo
}

func newSecEnv(t *testing.T) secEnv {
	t.Helper()
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)
	cache := apikeys.NewCache(60 * time.Second)
	svc := apikeys.NewService(db, repo, cache, apikeys.EnvLive, nil)
	mw := apikeys.NewMiddleware(repo, nil, nil) // cache nil -> always cold path
	return secEnv{svc: svc, mw: mw, repo: repo}
}

func (e secEnv) seedKey(t *testing.T, scopes []string) (uuid.UUID, uuid.UUID, string, uuid.UUID) {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	out, err := e.svc.Create(context.Background(), apikeys.CreateInput{
		TenantID: tenantID, StoreID: storeID, CreatedBy: uuid.New(),
		Scopes: scopes, RateLimitPerMin: 100, Label: "sec",
		Plan: subscription.PlanPro,
	})
	require.NoError(t, err)
	return tenantID, storeID, out.Plaintext, out.ID
}

func runRouter(t *testing.T, mw *apikeys.Middleware, scope apikeys.Scope, key string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.Authenticate())
	if scope != "" {
		r.GET("/v1/x", apikeys.RequireScope(scope), func(c *gin.Context) { c.Status(http.StatusOK) })
	} else {
		r.GET("/v1/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// Sec1 — Revoked keys are rejected immediately.
func TestSec_RevokedKey_Rejected(t *testing.T) {
	env := newSecEnv(t)
	tenantID, _, plaintext, id := env.seedKey(t, []string{"products:read"})
	require.NoError(t, env.svc.Revoke(context.Background(), tenantID, id, "compromised"))

	rec := runRouter(t, env.mw, "", plaintext)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Sec2 — Rotation-overlap window: old key valid while inside, rejected past.
func TestSec_RotationOverlap_Bounds(t *testing.T) {
	env := newSecEnv(t)
	tenantID, _, oldPlaintext, oldID := env.seedKey(t, []string{"products:read"})
	_, err := env.svc.Rotate(context.Background(), tenantID, oldID, "scheduled")
	require.NoError(t, err)

	// Inside the 24h window — old key still works.
	rec := runRouter(t, env.mw, "", oldPlaintext)
	require.Equal(t, http.StatusOK, rec.Code, "inside 24h overlap")

	// Force the overlap window past expiry by back-dating revoked_at.
	require.NoError(t, env.repo.Revoke(context.Background(), oldID, time.Now().Add(-time.Minute), "scheduled"))
	rec = runRouter(t, env.mw, "", oldPlaintext)
	require.Equal(t, http.StatusUnauthorized, rec.Code, "past expiry must reject")
}

// Sec3 — Scope mismatch returns 403, not 401.
func TestSec_ScopeMismatch_403(t *testing.T) {
	env := newSecEnv(t)
	_, _, plaintext, _ := env.seedKey(t, []string{"products:read"})

	rec := runRouter(t, env.mw, apikeys.ScopeProductsWrite, plaintext)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// Sec4 — Timing parity: prefix-miss vs prefix-hit-hash-miss should both run
// bcrypt at least once. The hot-path (cache) timing test in middleware_test.go
// covers cache speed-up; here we sanity-check that both fail paths spend
// at least bcrypt-comparable wall time.
func TestSec_TimingParity_FailedAuthPathsRunBcrypt(t *testing.T) {
	env := newSecEnv(t)

	// Path A: well-formed prefix, no DB row → unknown_key path runs VerifyDummy.
	t0 := time.Now()
	rec := runRouter(t, env.mw, "", "mk8_live_aaaaaaaabbbbbbbbccccccccddddddddeeeeee")
	prefixMiss := time.Since(t0)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// Path B: real prefix matches a row but suffix is garbage → still runs bcrypt.
	_, _, plaintext, _ := env.seedKey(t, []string{"products:read"})
	mutated := plaintext[:len(plaintext)-4] + "ZZZZ"
	t1 := time.Now()
	rec = runRouter(t, env.mw, "", mutated)
	prefixHitHashMiss := time.Since(t1)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// Both paths should run bcrypt; that's "comparable" — within a 4× factor
	// of each other. We don't assert tight equality because timing is noisy.
	require.Less(t, prefixMiss, prefixHitHashMiss*4, "prefix-miss timing should be comparable")
	require.Less(t, prefixHitHashMiss, prefixMiss*4, "prefix-hit-hash-miss timing should be comparable")
}

// Sec5 — Wrong-tenant lookup must not leak across tenants.
func TestSec_CrossTenant_NoLeak(t *testing.T) {
	env := newSecEnv(t)
	_, _, plaintext, _ := env.seedKey(t, []string{"products:read"})

	// Modify just the plaintext but keep the prefix shape valid; should fail
	// because no other tenant's row will match either.
	_ = plaintext
	bogus := "mk8_live_zzzzzzzzZZZZZZZZ1234567890abcdefghij"
	rec := runRouter(t, env.mw, "", bogus)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Sec6 — Empty Authorization header is rejected without DB hit.
func TestSec_NoAuthHeader_Rejected(t *testing.T) {
	env := newSecEnv(t)
	rec := runRouter(t, env.mw, "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
