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
func PromoPerIP() gin.HandlerFunc {
	// 5/hour = 5/(3600s) ≈ 0.00139 rps, burst=5
	const rps = 5.0 / 3600.0
	const burst = 5
	return buildKeyedLimiter("promo:ip:", rps, burst, func(c *gin.Context) string {
		return extractIP(c)
	})
}

// PromoPerEmail returns a Gin middleware that limits promo-apply requests per
// merchant email address: 10 requests per 24 hours with a burst of 10
// (§7.3 abuse prevention, pattern H). The email is read from the "email"
// query param or the JSON body field "email"; callers may also set the
// "X-Merchant-Email" header as a fallback.
func PromoPerEmail() gin.HandlerFunc {
	// 10/24h = 10/86400s ≈ 0.0001157 rps, burst=10
	const rps = 10.0 / 86400.0
	const burst = 10
	return buildKeyedLimiter("promo:email:", rps, burst, func(c *gin.Context) string {
		email := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Merchant-Email")))
		if email == "" {
			email = strings.ToLower(strings.TrimSpace(c.Query("email")))
		}
		return email
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
