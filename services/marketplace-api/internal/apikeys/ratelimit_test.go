package apikeys_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestRateLimiter_AllowsUpToBurst(t *testing.T) {
	l := apikeys.NewRateLimiter()
	defer l.Stop()
	keyID := uuid.New().String()

	// burst = perMinute, so the first 60 calls succeed.
	for i := 0; i < 60; i++ {
		require.Truef(t, l.Allow(keyID, 60), "request %d should be allowed", i)
	}
	// The 61st should fail (no tokens, refill rate is 1/sec).
	require.False(t, l.Allow(keyID, 60), "burst exhausted")
}

func TestRateLimiter_PerKeyIsolation(t *testing.T) {
	l := apikeys.NewRateLimiter()
	defer l.Stop()
	a, b := uuid.New().String(), uuid.New().String()

	for i := 0; i < 10; i++ {
		require.True(t, l.Allow(a, 10))
	}
	require.False(t, l.Allow(a, 10), "key A exhausted")
	require.True(t, l.Allow(b, 10), "key B has its own bucket")
}

func TestRateLimiter_DefaultsTo100PerMinute(t *testing.T) {
	l := apikeys.NewRateLimiter()
	defer l.Stop()
	keyID := uuid.New().String()

	// perMin=0 should be coerced to 100.
	for i := 0; i < 100; i++ {
		require.Truef(t, l.Allow(keyID, 0), "request %d under default 100/min should pass", i)
	}
	require.False(t, l.Allow(keyID, 0), "101st under default 100/min should fail")
}

func TestRateLimiter_Middleware_429OnExhaustion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := apikeys.NewRateLimiter()
	defer l.Stop()
	keyID := uuid.New().String()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("api_key_id", keyID)
		c.Set("api_key_rate_limit", 2)
		c.Next()
	})
	r.Use(l.Middleware())
	r.GET("/v1/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
		require.Equal(t, http.StatusOK, rec.Code, "request %d", i)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter_Middleware_NoOpWhenKeyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := apikeys.NewRateLimiter()
	defer l.Stop()
	r := gin.New()
	r.Use(l.Middleware())
	r.GET("/v1/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, l.Size(), "no bucket should be created when key missing")
}
