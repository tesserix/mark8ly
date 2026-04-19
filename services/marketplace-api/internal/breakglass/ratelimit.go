package breakglass

import (
	"sync"
	"time"
)

// LoginRateWindow is the sliding-window length used for login-attempt
// counting. Three failures inside one window → hard 24h lockout.
const LoginRateWindow = time.Hour

// LoginMaxFailures is the count threshold before we persist a
// break_glass_lockouts row.
const LoginMaxFailures = 3

// LoginLockoutDuration is the hard lockout persisted on the Nth failure.
const LoginLockoutDuration = 24 * time.Hour

// LoginRateLimiter is an in-memory sliding-window counter keyed by
// ip_hash. Pair it with the DB-backed lockouts table: the RL carries
// recent, fast-moving counts; the table is the durable decision.
//
// Deliberately not Redis-backed today. The single-process footprint
// of marketplace-api + the 3-strike threshold make per-pod accuracy
// good enough. When the service scales horizontally a Redis INCR/EXPIRE
// swap lands in the same interface.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// NewLoginRateLimiter returns an empty LoginRateLimiter.
func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{attempts: map[string][]time.Time{}}
}

// RecordFailure stamps now() on the bucket for key and returns the
// count of failures inside the current window AFTER the stamp.
// Callers compare against LoginMaxFailures to decide whether to
// persist a hard lockout.
func (l *LoginRateLimiter) RecordFailure(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-LoginRateWindow)

	bucket := l.attempts[key]
	// Drop anything outside the window.
	fresh := bucket[:0]
	for _, t := range bucket {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	fresh = append(fresh, now)
	l.attempts[key] = fresh
	return len(fresh)
}

// Reset clears the bucket for key. Called on successful login so a
// single slow-typist failure doesn't count against the next legitimate
// attempt.
func (l *LoginRateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// Count returns the current in-window failure count for key. Useful
// for tests; production code should use RecordFailure's return value.
func (l *LoginRateLimiter) Count(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-LoginRateWindow)
	bucket := l.attempts[key]
	count := 0
	for _, t := range bucket {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}
