package auth

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type perUserLimiter struct {
	limiters sync.Map
	rps      rate.Limit
	burst    int
}

// NewPerUserRateLimiter returns a Gin middleware that enforces a per-user
// token-bucket rate limit. After the auth middleware sets "user_id" on the
// gin context, this middleware checks (or creates) a limiter for that user.
// Returns 429 Too Many Requests when the bucket is empty.
func NewPerUserRateLimiter(requestsPerMinute int, burst int) gin.HandlerFunc {
	l := &perUserLimiter{
		rps:   rate.Limit(float64(requestsPerMinute) / 60.0),
		burst: burst,
	}
	return l.handle
}

func (l *perUserLimiter) handle(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.Next()
		return
	}
	val, _ := l.limiters.LoadOrStore(userID, rate.NewLimiter(l.rps, l.burst))
	limiter := val.(*rate.Limiter)
	if !limiter.Allow() {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":   "rate_limited",
			"message": "too many requests, please slow down",
		})
		return
	}
	c.Next()
}
