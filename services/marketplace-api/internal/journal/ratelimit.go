package journal

import (
	"sync"
	"time"
)

// SubscribeRateWindow is the sliding-window length used for per-IP
// counting on the public journal subscribe endpoint.
const SubscribeRateWindow = 10 * time.Minute

// SubscribeRateMax is the number of requests a single IP may make inside
// SubscribeRateWindow before Allow starts returning false.
const SubscribeRateMax = 5

// RateLimiter is an in-memory sliding-window counter keyed by client IP.
// It follows the same shape as internal/breakglass.LoginRateLimiter, but
// lives in this package rather than importing that one: breakglass is an
// auth-lockout concern with its own constants and semantics (a hard,
// DB-persisted lockout on the Nth failure), and reaching into it from a
// public, unauthenticated marketing endpoint would be a stranger coupling
// than duplicating the ~20 lines of sliding-window bookkeeping here.
//
// Deliberately not shared/Redis-backed, for the same reason breakglass
// isn't: single-process is good enough for this traffic shape, and a
// horizontal-scale swap lands behind the same Allow(key) interface.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// NewRateLimiter returns an empty RateLimiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{attempts: map[string][]time.Time{}}
}

// Allow records an attempt for key (the client IP) and reports whether
// it is within SubscribeRateMax for the current window.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-SubscribeRateWindow)

	bucket := l.attempts[key]
	fresh := bucket[:0]
	for _, t := range bucket {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= SubscribeRateMax {
		l.attempts[key] = fresh
		return false
	}
	fresh = append(fresh, now)
	l.attempts[key] = fresh
	return true
}
