// Package emailotp issues and verifies short-lived email one-time
// codes used as a login second factor.
//
// Codes are six digits, so the keyspace is only 10^6. That is safe
// only because three limits hold simultaneously: a five minute TTL,
// a hard cap on verification attempts per challenge, and a per-email
// request rate limit. Weakening any one of them makes brute force
// practical, so they are enforced here rather than left to callers.
//
// At rest a code is an HMAC-SHA256 over (email, code) keyed by a
// server-side pepper — never plaintext, and never valid for a
// different address.
package emailotp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Defaults chosen together as one brute-force budget: an attacker gets
// at most MaxPerWindow challenges per window, each accepting
// MaxAttempts guesses, giving 25 guesses per 15 minutes against 10^6.
const (
	DefaultTTL          = 5 * time.Minute
	DefaultMaxAttempts  = 5
	DefaultMaxPerWindow = 5
	DefaultRateWindow   = 15 * time.Minute

	codeDigits = 6
	minPepper  = 16
)

var (
	ErrNoChallenge     = errors.New("emailotp: no active challenge for this address")
	ErrInvalidCode     = errors.New("emailotp: invalid code")
	ErrExpired         = errors.New("emailotp: code has expired")
	ErrAlreadyUsed     = errors.New("emailotp: code has already been used")
	ErrTooManyAttempts = errors.New("emailotp: too many incorrect attempts")
	ErrRateLimited     = errors.New("emailotp: too many codes requested")
	ErrInvalidEmail    = errors.New("emailotp: email is required")
	ErrWeakPepper      = errors.New("emailotp: pepper must be at least 16 bytes")
)

// Record is one challenge row.
type Record struct {
	ID         string
	Email      string
	CodeHash   []byte
	IPAddress  string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// Store is the persistence seam. Latest must return the most recently
// created record for the address, or ErrNoChallenge when there is none.
type Store interface {
	Insert(ctx context.Context, r Record) error
	Latest(ctx context.Context, email string) (*Record, error)
	IncrementAttempts(ctx context.Context, id string) error
	Consume(ctx context.Context, id string, at time.Time) error
	CountSince(ctx context.Context, email string, since time.Time) (int, error)
}

// Config holds Service dependencies. Only Store and Pepper are required.
type Config struct {
	Store        Store
	Pepper       string
	TTL          time.Duration
	MaxAttempts  int
	MaxPerWindow int
	RateWindow   time.Duration
	// Now is injectable so tests can advance the clock instead of sleeping.
	Now func() time.Time
}

// Service issues and verifies email OTP challenges.
type Service struct {
	store        Store
	pepper       []byte
	ttl          time.Duration
	maxAttempts  int
	maxPerWindow int
	rateWindow   time.Duration
	now          func() time.Time
}

// NewService validates config and applies defaults.
func NewService(cfg Config) (*Service, error) {
	if cfg.Store == nil {
		return nil, errors.New("emailotp: store is required")
	}
	if len(cfg.Pepper) < minPepper {
		return nil, ErrWeakPepper
	}
	s := &Service{
		store:        cfg.Store,
		pepper:       []byte(cfg.Pepper),
		ttl:          cfg.TTL,
		maxAttempts:  cfg.MaxAttempts,
		maxPerWindow: cfg.MaxPerWindow,
		rateWindow:   cfg.RateWindow,
		now:          cfg.Now,
	}
	if s.ttl == 0 {
		s.ttl = DefaultTTL
	}
	if s.maxAttempts == 0 {
		s.maxAttempts = DefaultMaxAttempts
	}
	if s.maxPerWindow == 0 {
		s.maxPerWindow = DefaultMaxPerWindow
	}
	if s.rateWindow == 0 {
		s.rateWindow = DefaultRateWindow
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// Challenge is the result of Request. Code is plaintext and exists only
// to be handed to the notification channel — it must never be logged,
// returned over the API, or persisted.
type Challenge struct {
	Code      string
	ExpiresAt time.Time
}

// Request issues a fresh code for the address, subject to the rate limit.
func (s *Service) Request(ctx context.Context, email, ip string) (*Challenge, error) {
	addr := normaliseEmail(email)
	if addr == "" {
		return nil, ErrInvalidEmail
	}

	now := s.now()
	n, err := s.store.CountSince(ctx, addr, now.Add(-s.rateWindow))
	if err != nil {
		return nil, fmt.Errorf("emailotp: count recent: %w", err)
	}
	if n >= s.maxPerWindow {
		return nil, ErrRateLimited
	}

	code, err := generateCode()
	if err != nil {
		return nil, err
	}
	rec := Record{
		ID:        uuid.NewString(),
		Email:     addr,
		CodeHash:  s.hash(addr, code),
		IPAddress: ip,
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	}
	if err := s.store.Insert(ctx, rec); err != nil {
		return nil, fmt.Errorf("emailotp: insert: %w", err)
	}
	return &Challenge{Code: code, ExpiresAt: rec.ExpiresAt}, nil
}

// Verify checks a submitted code against the newest challenge for the
// address and consumes it on success.
//
// Ordering matters: expiry, replay and the attempt cap are all checked
// before the code comparison, so a spent or stale challenge cannot be
// used as an oracle for guessing.
func (s *Service) Verify(ctx context.Context, email, code string) error {
	addr := normaliseEmail(email)
	submitted := strings.TrimSpace(code)

	// Reject malformed input before loading the record so that garbage
	// cannot drain a legitimate user's attempt budget.
	if !isSixDigits(submitted) {
		return ErrInvalidCode
	}

	rec, err := s.store.Latest(ctx, addr)
	if err != nil {
		if errors.Is(err, ErrNoChallenge) {
			return ErrNoChallenge
		}
		return fmt.Errorf("emailotp: latest: %w", err)
	}

	if rec.ConsumedAt != nil {
		return ErrAlreadyUsed
	}
	if rec.Attempts >= s.maxAttempts {
		return ErrTooManyAttempts
	}
	now := s.now()
	if !now.Before(rec.ExpiresAt) {
		return ErrExpired
	}

	if !hmac.Equal(rec.CodeHash, s.hash(addr, submitted)) {
		if err := s.store.IncrementAttempts(ctx, rec.ID); err != nil {
			return fmt.Errorf("emailotp: increment attempts: %w", err)
		}
		return ErrInvalidCode
	}

	if err := s.store.Consume(ctx, rec.ID, now); err != nil {
		return fmt.Errorf("emailotp: consume: %w", err)
	}
	return nil
}

// hash binds the address into the MAC so a code minted for one user is
// worthless against another, even on a digit collision.
func (s *Service) hash(email, code string) []byte {
	m := hmac.New(sha256.New, s.pepper)
	m.Write([]byte(email))
	m.Write([]byte{0}) // separator: keeps (email|code) unambiguous
	m.Write([]byte(code))
	return m.Sum(nil)
}

// generateCode draws a uniform six-digit code from crypto/rand.
func generateCode() (string, error) {
	max := big.NewInt(1)
	for i := 0; i < codeDigits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("emailotp: rand: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n), nil
}

func normaliseEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}

func isSixDigits(s string) bool {
	if len(s) != codeDigits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
