package apikeys

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter is an in-memory per-key token bucket. Buckets are created
// lazily on first request and evicted by a periodic janitor when their
// last-seen-at falls outside the eviction TTL. Production deployments with
// >1 replica should swap this for a Redis-backed implementation; the rate
// shape (requests/min) is the same.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*entry
	evictAfter  time.Duration
	now         func() time.Time
	stop        chan struct{}
}

type entry struct {
	limiter    *rate.Limiter
	lastSeenAt time.Time
}

// NewRateLimiter constructs an in-memory limiter and starts the janitor.
// Call Stop on shutdown.
func NewRateLimiter() *RateLimiter {
	l := &RateLimiter{
		buckets:    make(map[string]*entry),
		evictAfter: 10 * time.Minute,
		now:        time.Now,
		stop:       make(chan struct{}),
	}
	go l.janitor()
	return l
}

// Stop halts the janitor goroutine. Idempotent.
func (l *RateLimiter) Stop() {
	select {
	case <-l.stop:
		return
	default:
		close(l.stop)
	}
}

// Allow consumes one token from the (key, perMinute) bucket. Returns true
// when the request may proceed.
func (l *RateLimiter) Allow(key string, perMinute int) bool {
	if perMinute <= 0 {
		perMinute = 100
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.buckets[key]
	if !ok {
		// rate.Limit is per-second; perMinute requests per minute = perMinute/60 per second.
		// Burst = full minute's worth so steady-state usage doesn't trip on the first request.
		e = &entry{
			limiter: rate.NewLimiter(rate.Limit(float64(perMinute)/60.0), perMinute),
		}
		l.buckets[key] = e
	}
	e.lastSeenAt = l.now()
	return e.limiter.Allow()
}

// Middleware is the Gin handler that enforces the per-key bucket. Reads
// `api_key_id` + `api_key_rate_limit` from the context (populated by the
// auth middleware). Responds 429 on bucket exhaustion.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID := c.GetString("api_key_id")
		if keyID == "" {
			c.Next()
			return
		}
		perMin, _ := c.Get("api_key_rate_limit")
		limit, _ := perMin.(int)
		if !l.Allow(keyID, limit) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "API key rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.evictStale()
		case <-l.stop:
			return
		}
	}
}

func (l *RateLimiter) evictStale() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.evictAfter)
	for k, e := range l.buckets {
		if e.lastSeenAt.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// Size returns the current bucket count. Test-only.
func (l *RateLimiter) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
