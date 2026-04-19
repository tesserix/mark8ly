package apikeys

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// Middleware authenticates requests against the public R/W API by validating
// a `Bearer mk8_live_...` token. Lookups are O(log n) via the (tenant_id,
// key_prefix) index; verifies are bcrypt with a 60s hot-key cache so repeat
// requests skip the bcrypt cost.
type Middleware struct {
	repo     *Repo
	cache    *Cache
	lastUsed *LastUsedWorker
	now      func() time.Time
}

// NewMiddleware constructs a Middleware. cache and lastUsed may both be nil
// — useful for tests but not production. cache=nil disables the 60s hot-key
// short-circuit; lastUsed=nil skips the async last_used_at write.
func NewMiddleware(repo *Repo, cache *Cache, lastUsed *LastUsedWorker) *Middleware {
	return &Middleware{
		repo:     repo,
		cache:    cache,
		lastUsed: lastUsed,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Authenticate is the Gin middleware. On any failure it responds 401 with a
// generic body — never leak which step failed (prefix vs hash vs revoked).
func (m *Middleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := extractBearer(c)
		if !ok {
			metrics.APIKeyAuthFailedTotal.WithLabelValues("missing_bearer").Inc()
			abort401(c)
			return
		}

		if m.cache != nil {
			if e, hit := m.cache.Get(key); hit && (e.RevokedAt == nil || e.RevokedAt.After(m.now())) {
				populateContext(c, e)
				m.touchLastUsed(c, e.KeyID)
				metrics.APIKeyUsedTotal.WithLabelValues("cache_hit").Inc()
				c.Next()
				return
			}
		}

		prefix, ok := ExtractPrefix(key)
		if !ok {
			_ = VerifyDummy(key)
			metrics.APIKeyAuthFailedTotal.WithLabelValues("wrong_prefix").Inc()
			abort401(c)
			return
		}

		candidates, err := m.repo.FindByPrefix(c.Request.Context(), prefix)
		if err != nil || len(candidates) == 0 {
			_ = VerifyDummy(key)
			metrics.APIKeyAuthFailedTotal.WithLabelValues("unknown_key").Inc()
			abort401(c)
			return
		}

		now := m.now()
		for _, row := range candidates {
			if !row.IsUsable(now) {
				continue
			}
			if Verify(row.KeyHash, key) == nil {
				e := CacheEntry{
					KeyID:           row.ID,
					TenantID:        row.TenantID,
					StoreID:         row.StoreID,
					Scopes:          []string(row.Scopes),
					RateLimitPerMin: row.RateLimitPerMin,
					RevokedAt:       row.RevokedAt,
				}
				if m.cache != nil {
					m.cache.Put(key, e)
				}
				populateContext(c, e)
				m.touchLastUsed(c, e.KeyID)
				metrics.APIKeyUsedTotal.WithLabelValues("cold_lookup").Inc()
				c.Next()
				return
			}
		}
		_ = VerifyDummy(key)
		metrics.APIKeyAuthFailedTotal.WithLabelValues("revoked_or_mismatch").Inc()
		abort401(c)
	}
}

// extractBearer pulls the bearer key off the Authorization header. Returns
// ok=false for missing / wrong-prefix headers.
func extractBearer(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	key := strings.TrimPrefix(h, "Bearer ")
	if key == "" {
		return "", false
	}
	if !strings.HasPrefix(key, livePrefix) && !strings.HasPrefix(key, testPrefix) {
		return "", false
	}
	return key, true
}

// populateContext writes the verified key payload onto the Gin context. The
// downstream scope middleware reads `api_key_scopes`; rate limiter reads
// `api_key_id` + `api_key_rate_limit`; handlers read `tenant_id`/`store_id`
// the same way they do under admin auth.
func populateContext(c *gin.Context, e CacheEntry) {
	c.Set("tenant_id", e.TenantID.String())
	c.Set("store_id", e.StoreID.String())
	c.Set("api_key_id", e.KeyID.String())
	c.Set("api_key_scopes", e.Scopes)
	c.Set("api_key_rate_limit", e.RateLimitPerMin)
	c.Set("auth_method", "api_key")
}

func abort401(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":   "unauthorized",
		"message": "invalid_api_key",
	})
}

// touchLastUsed enqueues an async last_used_at + ip_hash write. Non-blocking
// and nil-safe so the auth path is never gated on the worker queue depth.
func (m *Middleware) touchLastUsed(c *gin.Context, keyID uuid.UUID) {
	if m.lastUsed == nil {
		return
	}
	m.lastUsed.Submit(keyID, c.ClientIP())
}
