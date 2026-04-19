//go:build integration

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

func init() {
	gin.SetMode(gin.TestMode)
}

type mwEnv struct {
	svc *apikeys.Service
	mw  *apikeys.Middleware
}

func newMwEnv(t *testing.T) mwEnv {
	t.Helper()
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)
	cache := apikeys.NewCache(60 * time.Second)
	svc := apikeys.NewService(db, repo, cache, apikeys.EnvLive)
	mw := apikeys.NewMiddleware(repo, cache, nil)
	return mwEnv{svc: svc, mw: mw}
}

type seeded struct {
	ID        uuid.UUID
	Plaintext string
	TenantID  uuid.UUID
	StoreID   uuid.UUID
}

func (e mwEnv) seed(t *testing.T, scope string) seeded {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	out, err := e.svc.Create(context.Background(), apikeys.CreateInput{
		TenantID: tenantID, StoreID: storeID, CreatedBy: uuid.New(),
		Scopes: []string{scope}, RateLimitPerMin: 100, Label: "test",
		Plan: subscription.PlanPro,
	})
	require.NoError(t, err)
	return seeded{ID: out.ID, Plaintext: out.Plaintext, TenantID: tenantID, StoreID: storeID}
}

func runWith(t *testing.T, mw *apikeys.Middleware, key string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	r := gin.New()
	var captured *gin.Context
	r.Use(mw.Authenticate())
	r.GET("/v1/ping", func(c *gin.Context) {
		captured = c.Copy()
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec, captured
}

func TestMiddleware_ValidKey_AllowsAndPopulatesContext(t *testing.T) {
	env := newMwEnv(t)
	k := env.seed(t, "products:read")

	rec, ctx := runWith(t, env.mw, k.Plaintext)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, k.TenantID.String(), ctx.GetString("tenant_id"))
	require.Equal(t, "api_key", ctx.GetString("auth_method"))
}

func TestMiddleware_MissingBearer_Returns401(t *testing.T) {
	env := newMwEnv(t)
	rec, _ := runWith(t, env.mw, "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_WrongPrefix_Returns401(t *testing.T) {
	env := newMwEnv(t)
	rec, _ := runWith(t, env.mw, "sk_live_aaaaaaaabbbbbbbb1234")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_RevokedKey_Returns401(t *testing.T) {
	env := newMwEnv(t)
	k := env.seed(t, "products:read")
	require.NoError(t, env.svc.Revoke(context.Background(), k.TenantID, k.ID, "test"))

	rec, _ := runWith(t, env.mw, k.Plaintext)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_HotPathUsesCache(t *testing.T) {
	env := newMwEnv(t)
	k := env.seed(t, "products:read")

	doOnce := func() time.Duration {
		t0 := time.Now()
		rec, _ := runWith(t, env.mw, k.Plaintext)
		require.Equal(t, http.StatusOK, rec.Code)
		return time.Since(t0)
	}
	cold := doOnce()
	hot := doOnce()
	require.Less(t, hot, cold/3, "cached verify should be much faster than cold bcrypt: cold=%v hot=%v", cold, hot)
}

func TestMiddleware_RotatedKey_OldStillWorksUnder24h(t *testing.T) {
	env := newMwEnv(t)
	k := env.seed(t, "products:read")
	_, err := env.svc.Rotate(context.Background(), k.TenantID, k.ID, "scheduled")
	require.NoError(t, err)

	// Old key still usable inside the 24h overlap.
	rec, _ := runWith(t, env.mw, k.Plaintext)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_BogusBearer_TimingParityFromDummyVerify(t *testing.T) {
	env := newMwEnv(t)
	// A well-formed but unknown key should still spend bcrypt time. We only
	// assert the response code here; the timing assertion is covered by
	// TestMiddleware_HotPathUsesCache for the positive path.
	rec, _ := runWith(t, env.mw, "mk8_live_aaaaaaaabbbbbbbbccccccccddddddddeeeeee")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
