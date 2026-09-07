package onboarding

import (
	"sync"
	"time"
)

// PromoRateWindow and PromoRateMax bound how often one client may test promo
// codes. A code checker is a guessing surface by nature, and this route is
// unauthenticated like the rest of the onboarding wizard (mark8ly#620).
//
// Set deliberately loose. A merchant retyping a code they hold gets several
// attempts; someone enumerating a namespace gets nowhere near enough. The
// codes themselves are the real defence — the console mints human campaign
// codes, and the per-email cap means a guessed code is still only redeemable
// once per address.
const (
	PromoRateWindow = 10 * time.Minute
	PromoRateMax    = 20
)

// PromoRateLimiter is an in-memory sliding-window counter keyed by client IP,
// the same shape marketplace-api's journal.RateLimiter uses.
//
// # What this limiter can and cannot see
//
// The onboarding app calls this route from a SERVER action, so the IP gin
// reports is that app's pod, not the visitor's. This limiter is therefore a
// coarse, service-wide cap rather than a per-visitor one, and it is the second
// of two layers: the onboarding app applies its own per-visitor limit, keyed
// on the address it can actually see, before it ever calls here.
//
// Stated because the distinction is invisible from this file, and a reader who
// assumed "per IP" meant "per visitor" would over-trust it — and might tighten
// PromoRateMax to a number that locks out every merchant at once.
type PromoRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// NewPromoRateLimiter returns an empty PromoRateLimiter.
func NewPromoRateLimiter() *PromoRateLimiter {
	return &PromoRateLimiter{attempts: map[string][]time.Time{}}
}

// Allow records an attempt for key and reports whether it is within
// PromoRateMax for the current window.
func (l *PromoRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-PromoRateWindow)
	bucket := l.attempts[key]
	fresh := bucket[:0]
	for _, t := range bucket {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= PromoRateMax {
		l.attempts[key] = fresh
		return false
	}
	l.attempts[key] = append(fresh, time.Now())
	return true
}
