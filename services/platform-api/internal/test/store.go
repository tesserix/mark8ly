// Package test provides e2e test helpers — endpoints and in-memory state
// that bypass the email send during Playwright runs.
//
// EVERYTHING IN THIS PACKAGE IS GATED to non-production environments. The
// HTTP routes are only registered when cfg.Env != "prod"; the
// TokenRecorder is only wired into the verification service in the same
// branch. There is no code path that exposes test endpoints in prod.
package test

import "sync"

// TokenRecorder captures the plaintext magic-link token for an email so
// the e2e test helper endpoint can hand it back to Playwright. The
// verification service calls Record after generating each token; in prod
// the recorder is nil and the call is a no-op.
//
// The recorder stores ONLY the most-recent token per email. A second
// SendMagicLink for the same email overwrites the first, mirroring the
// "only the latest link is valid" semantics in the verification service.
type TokenRecorder struct {
	mu     sync.RWMutex
	tokens map[string]string // email → latest plaintext token
}

// NewTokenRecorder constructs an empty recorder.
func NewTokenRecorder() *TokenRecorder {
	return &TokenRecorder{tokens: make(map[string]string)}
}

// Record stores the latest token for the given email. Safe for concurrent
// use. Used by verification.Service when running in dev/test.
func (r *TokenRecorder) Record(email, token string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[email] = token
}

// Latest returns the most recent recorded token for the email and a
// boolean indicating whether one exists.
func (r *TokenRecorder) Latest(email string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tok, ok := r.tokens[email]
	return tok, ok
}

// InvitationTokenRecorder captures plaintext invitation tokens for the
// e2e test helper. Keyed on tenant+email so the same email can have
// pending invitations on multiple tenants without collision — the
// Phase P test scenarios rely on this for the multi-tenant story.
type InvitationTokenRecorder struct {
	mu     sync.RWMutex
	tokens map[string]string // tenantID|email → latest plaintext token
}

// NewInvitationTokenRecorder constructs an empty recorder.
func NewInvitationTokenRecorder() *InvitationTokenRecorder {
	return &InvitationTokenRecorder{tokens: make(map[string]string)}
}

// Record stores the latest invitation token for the given tenant+email.
// Safe for concurrent use. Called by invitation.Service in dev/test.
func (r *InvitationTokenRecorder) Record(tenantID, email, token string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[tenantID+"|"+email] = token
}

// LatestByEmail returns the most recent token for an email across
// any tenant. Used by the e2e helper endpoint which doesn't know
// the tenant id of a freshly-invited user.
func (r *InvitationTokenRecorder) LatestByEmail(email string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	suffix := "|" + email
	for key, tok := range r.tokens {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			return tok, true
		}
	}
	return "", false
}
