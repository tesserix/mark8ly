// Package ratelimit provides per-IP rate-limiting Gin middleware using
// an in-memory token bucket. No Redis dependency.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// entry holds a limiter and the time it was last used, for TTL eviction.
type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// PerIP returns a Gin middleware that limits requests per client IP.
// rps is the sustained requests-per-second; burst is the maximum burst.
// For example, PerIP(0.167, 10) gives ~10 req/min with a burst of 10.
//
// Amendment FIX 4: includes a cleanup goroutine that evicts entries
// older than 2x the window duration every 5 minutes.
func PerIP(rps float64, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	limiters := make(map[string]*entry)

	// Background cleanup goroutine — evicts stale entries.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		// TTL = 2x the refill window. For 10 req/min (rps=0.167),
		// that's ~12 minutes. Floor at 2 minutes.
		ttl := time.Duration(float64(2*time.Second) / rps)
		if ttl < 2*time.Minute {
			ttl = 2 * time.Minute
		}
		for range ticker.C {
			now := time.Now()
			mu.Lock()
			for ip, e := range limiters {
				if now.Sub(e.lastSeen) > ttl {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := extractIP(c)

		mu.Lock()
		e, ok := limiters[ip]
		if !ok {
			e = &entry{
				limiter:  rate.NewLimiter(rate.Limit(rps), burst),
				lastSeen: time.Now(),
			}
			limiters[ip] = e
		} else {
			e.lastSeen = time.Now()
		}
		mu.Unlock()

		if !e.limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests, please try again later",
			})
			return
		}

		c.Next()
	}
}

// extractIP gets the client IP from X-Forwarded-For (Cloudflare/Istio),
// falling back to RemoteAddr.
func extractIP(c *gin.Context) string {
	// Trust first X-Forwarded-For value (set by Cloudflare).
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// Take the first IP in the comma-separated list.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Fallback to RemoteAddr (strip port).
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}
