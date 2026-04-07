package gip

import (
	"context"
	"sync"
)

// FakeVerifier is an in-memory Verifier for unit tests.
//
// Tests pre-register valid tokens via Add(). Calling VerifyToken with a
// known token returns the recorded VerifiedToken; unknown tokens fail.
//
// Use FailNextN to simulate transient verification failures (network blips,
// JWKS rotation gaps) for retry tests.
type FakeVerifier struct {
	mu        sync.Mutex
	tokens    map[string]*VerifiedToken
	failNextN int
}

// NewFakeVerifier constructs an empty FakeVerifier.
func NewFakeVerifier() *FakeVerifier {
	return &FakeVerifier{tokens: map[string]*VerifiedToken{}}
}

// Add registers a token string with the VerifiedToken it should resolve to.
// The expectedTenantID passed to VerifyToken must match v.TenantID.
func (f *FakeVerifier) Add(token string, v VerifiedToken) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[token] = &v
}

// FailNextN makes the next n VerifyToken calls return ErrTokenInvalid.
// Used for retry tests.
func (f *FakeVerifier) FailNextN(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNextN = n
}

// VerifyToken returns the registered VerifiedToken if present, or
// ErrTokenInvalid if not. Returns ErrTenantMismatch if expectedTenantID
// doesn't match the registered tenant.
func (f *FakeVerifier) VerifyToken(ctx context.Context, idToken, expectedTenantID string) (*VerifiedToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failNextN > 0 {
		f.failNextN--
		return nil, ErrTokenInvalid
	}

	tok, ok := f.tokens[idToken]
	if !ok {
		return nil, ErrTokenInvalid
	}
	if tok.TenantID != expectedTenantID {
		return nil, ErrTenantMismatch
	}
	return tok, nil
}
