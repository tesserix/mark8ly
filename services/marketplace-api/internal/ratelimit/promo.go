package ratelimit

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// PromoPerIP returns a Gin middleware that limits promo-apply requests per
// client IP: 5 requests per hour with a burst of 5 (§7.3 abuse prevention,
// pattern H). Uses the same in-memory token bucket as PerIP; no Redis.
//
// In-memory means PER POD. With N replicas the effective ceiling is N times
// this, and a rollout resets every bucket. That is accepted: this is a brake
// on code-guessing, not an accounting control, and the codes it protects are
// additionally bounded by max_redemptions on the row itself.
func PromoPerIP() gin.HandlerFunc {
	// 5/hour = 5/(3600s) ≈ 0.00139 rps, burst=5
	const rps = 5.0 / 3600.0
	const burst = 5
	return buildKeyedLimiter("promo:ip:", rps, burst, func(c *gin.Context) string {
		return extractIP(c)
	})
}

// PromoPerTenant returns a Gin middleware that limits promo-apply requests per
// tenant: 10 requests per 24 hours with a burst of 10 (§7.3 abuse prevention,
// pattern H).
//
// # It was PromoPerEmail, and keyed on an address that no longer arrives
//
// The previous version read the caller's email from an "X-Merchant-Email"
// header or an "email" query param — while its own doc claimed it also read a
// JSON body field of that name, which it never did. mark8ly#773 then removed
// the request body's `email` entirely, so the merchant's address stops at the
// handler, which resolves it from the subscription row.
//
// That left nothing for this limiter to key on. buildKeyedLimiter SKIPS a
// request whose key is empty, so wiring the old version would not have
// throttled anything — it would have been a middleware that reads as
// protection and enforces none. That failure is silent by construction, which
// is why this is a rename rather than a tweak: the name now matches what it
// can actually see.
//
// `tenant_id` is set by tenantMW, which is part of storeMW on
// /admin/stores/:storeId — so it is present for every route this can guard.
// A request that somehow lacks it is skipped rather than lumped into one
// shared bucket, which is the same fail-open choice buildKeyedLimiter makes
// and the right one: an unkeyed request is unattributable, and throttling all
// of them together would let one caller starve everyone else.
func PromoPerTenant() gin.HandlerFunc {
	// 10/24h = 10/86400s ≈ 0.0001157 rps, burst=10
	const rps = 10.0 / 86400.0
	const burst = 10
	return buildKeyedLimiter("promo:tenant:", rps, burst, func(c *gin.Context) string {
		return strings.ToLower(strings.TrimSpace(c.GetString("tenant_id")))
	})
}

// buildKeyedLimiter creates a rate-limiter middleware keyed by the string
// returned by keyFn. Entries are evicted after 2x the refill window.
func buildKeyedLimiter(prefix string, rps float64, burst int, keyFn func(*gin.Context) string) gin.HandlerFunc {
	var mu sync.Mutex
	limiters := make(map[string]*entry)

	// TTL = 2x refill window, floor at 2 minutes.
	ttl := time.Duration(float64(2*time.Second) / rps)
	if ttl < 2*time.Minute {
		ttl = 2 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			mu.Lock()
			for k, e := range limiters {
				if now.Sub(e.lastSeen) > ttl {
					delete(limiters, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		key := prefix + keyFn(c)
		if key == prefix {
			// No key available (e.g. email not sent) — skip email limiter.
			c.Next()
			return
		}

		mu.Lock()
		e, ok := limiters[key]
		if !ok {
			e = &entry{
				limiter:  rate.NewLimiter(rate.Limit(rps), burst),
				lastSeen: time.Now(),
			}
			limiters[key] = e
		} else {
			e.lastSeen = time.Now()
		}
		mu.Unlock()

		if !e.limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many promo requests, please try again later",
			})
			return
		}

		c.Next()
	}
}
