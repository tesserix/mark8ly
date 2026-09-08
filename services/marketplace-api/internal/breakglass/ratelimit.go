package breakglass

import (
	"context"
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

// LoginWindow is the sliding window the login path counts against.
//
// An interface so the window can be per-pod or estate-wide without the
// handler knowing which. `LoginRateLimiter` below is the in-memory one;
// `DBLoginWindow` is the durable one backed by break_glass_login_attempts.
//
// The methods take a context because the durable implementation does I/O.
// The in-memory one ignores it, which is the cost of having one interface
// rather than two call sites.
//
// # Why a second implementation exists (#846)
//
// The in-memory window is per-POD. That was fine while the admin deployment
// was pinned to a single replica — and the pin was the problem: with
// `maxSurge: 0` the only pod is scaled to zero before its replacement
// starts, so every marketplace-api deploy took the whole platform-admin
// surface offline for the length of an image pull. Every federated read from
// the tesserix console failed for that window.
//
// The pin could not simply be lifted. The durable lockout row is enforced
// estate-wide (the login path reads `IsIPLocked` on every attempt), but the
// COUNTING toward the 3-strike threshold was per-pod, so N replicas meant an
// attacker spreading attempts across them needed 3N failures rather than 3.
// Making the window durable is what makes the replica count a free choice.
//
// The original note here said a Redis INCR/EXPIRE swap would land in this
// interface. Redis has since been removed from marketplace-api entirely —
// it is not in go.mod — so the swap landed on Postgres, which the login path
// was already reading on every attempt anyway.
type LoginWindow interface {
	// RecordFailure stamps a failure and returns the in-window count
	// INCLUDING it. Callers compare against LoginMaxFailures.
	RecordFailure(ctx context.Context, key LoginKey) (int, error)
	// Reset clears the bucket for key.
	Reset(ctx context.Context, key LoginKey) error
	// Count returns the current in-window failure count for key.
	Count(ctx context.Context, key LoginKey) (int, error)
}

// LoginKey is a bucket key. It carries BOTH shapes the two implementations
// need — the truncated string the in-memory map is keyed by, and the full
// ip_hash the durable table stores — so a caller cannot hand one
// implementation a key shaped for the other.
//
// This is the same hazard `LoginRateLimitKey` was extracted to prevent, one
// level up: a key shaped differently in the login path than in the
// clear-lockout path means clear-lockout resets a bucket nobody reads.
type LoginKey struct {
	// IPHash is the full HMAC. The durable window stores and queries it.
	IPHash []byte
}

// Bucket is the in-memory map key for this LoginKey — the first 16 bytes of
// the ip_hash, exactly what LoginRateLimitKey has always produced.
func (k LoginKey) Bucket() string { return LoginRateLimitKey(k.IPHash) }

// LoginRateLimiter is an in-memory sliding-window counter keyed by
// ip_hash. Pair it with the DB-backed lockouts table: the RL carries
// recent, fast-moving counts; the table is the durable decision.
//
// Still used as the DEGRADED fallback when the lockout store cannot be
// read (#468) — see the login handler — so it is not dead code even where
// DBLoginWindow is wired.
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
func (l *LoginRateLimiter) RecordFailure(_ context.Context, k LoginKey) (int, error) {
	return l.recordFailure(k.Bucket()), nil
}

func (l *LoginRateLimiter) recordFailure(key string) int {
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
func (l *LoginRateLimiter) Reset(_ context.Context, k LoginKey) error {
	l.reset(k.Bucket())
	return nil
}

func (l *LoginRateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// LoginRateLimitKey shapes a LoginRateLimiter bucket key from an
// ip_hash. THE single source of truth for this shape — both
// internal/handlers/admin (the login path, which records failures and
// resets on success) and internal/handlers/platformadmin (the
// clear-lockout write path, which resets the same bucket) call this
// directly rather than each keeping their own copy. Before this
// existed, the two packages each defined a byte-for-byte identical
// private function with a prose comment promising they'd stay in sync
// — a promise nothing enforced. A key shaped differently in one place
// than the other means clear-lockout resets a bucket the login path
// never reads: the durable DB lockout clears, the operator sees
// success, and the in-memory limiter silently keeps refusing the IP.
//
// Keeping only the first 16 bytes of ip_hash avoids memory bloat across
// many concurrent IPs; 16 bytes of a HMAC-SHA256 output is still
// effectively collision-free for this purpose.
func LoginRateLimitKey(ipHash []byte) string {
	n := len(ipHash)
	if n > 16 {
		n = 16
	}
	return string(ipHash[:n])
}

// Count returns the current in-window failure count for key. Useful
// for tests; production code should use RecordFailure's return value.
func (l *LoginRateLimiter) Count(_ context.Context, k LoginKey) (int, error) {
	return l.count(k.Bucket()), nil
}

func (l *LoginRateLimiter) count(key string) int {
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
