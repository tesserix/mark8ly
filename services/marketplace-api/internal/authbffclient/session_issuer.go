// Package authbffclient exposes the narrow surface marketplace-api
// uses to talk to auth-bff. Today that's the SessionIssuer — the
// contract by which break-glass login + SSO callback routes ask
// auth-bff to mint the standard Mark8ly session cookie.
//
// The production implementation POSTs to auth-bff's internal-only
// POST /internal/mint-session endpoint, authenticated with the shared
// X-Internal-Auth header (internalauth.Equal on the auth-bff side) —
// NOT mTLS. See docs/superpowers/plans/2026-09-07-break-glass-activation-v2.md,
// D2: mTLS was never real for this endpoint; the header is the pattern
// that already exists and works for every other /internal route.
// It returns the raw Set-Cookie header value for marketplace-api to
// pass through to the caller. Cookie cryptography stays owned by
// auth-bff — marketplace-api must NEVER sign session cookies itself.
package authbffclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrIssuerUnavailable is returned by the stub SessionIssuer when no
// real implementation has been wired. Production main.go must swap in
// a concrete issuer before serving any route that calls Issue.
var ErrIssuerUnavailable = errors.New("authbffclient: session issuer not configured")

// SessionIssuer mints a Mark8ly session cookie for (tenantID, userID)
// with a TTL and returns the full Set-Cookie header value, ready to
// hand to c.Writer.Header().Add("Set-Cookie", ...).
//
// authContext is one of auth-bff's allow-listed values ("staff",
// "customer", "break_glass") and is passed by the CALLER, never
// defaulted here. This is the interface-widening decision recorded in
// the v2 plan (Task 3): the Task 0 consumer audit found a missing
// email is safe (every consumer treats it as an optional attribution
// label with a fallback), but auth_context is not — it is the one
// safety control the mint-session endpoint exists to enforce, and a
// callee-supplied default would silently defeat it for whichever
// caller forgot to think about it. Widening also means the SSO
// callback (internal/handlers/public/sso_login.go) and break-glass
// login (internal/handlers/admin/break_glass_login.go) both say what
// they are minting explicitly, in one interface, rather than one of
// them being hidden inside HTTPIssuer's implementation.
//
// Implementations MUST be safe for concurrent use from request
// goroutines — marketplace-api treats issuance as a synchronous,
// request-scoped call.
type SessionIssuer interface {
	Issue(ctx context.Context, tenantID, userID uuid.UUID, authContext string, ttl time.Duration) (setCookie string, err error)
}

// NoopIssuer is a safe-by-default stub returned when no real issuer
// is configured (local dev, tests). Every Issue call returns
// ErrIssuerUnavailable so a misconfigured route fails loudly instead
// of silently minting an empty cookie.
type NoopIssuer struct{}

// Issue always returns ErrIssuerUnavailable — by design.
func (NoopIssuer) Issue(_ context.Context, _, _ uuid.UUID, _ string, _ time.Duration) (string, error) {
	return "", ErrIssuerUnavailable
}

// StaticIssuer is a test-only issuer that returns a fixed cookie
// string. Intended for handler tests that want to assert the
// Set-Cookie header is written without invoking auth-bff.
type StaticIssuer struct {
	Cookie string
	Err    error
}

// Issue returns the pre-configured Cookie / Err.
func (s StaticIssuer) Issue(_ context.Context, tenantID, userID uuid.UUID, _ string, ttl time.Duration) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	if s.Cookie != "" {
		return s.Cookie, nil
	}
	// Default — produces a shape tests can assert on. Not crypto-valid.
	return fmt.Sprintf("mk8_session=test-%s-%s; Max-Age=%d; HttpOnly; Secure; SameSite=Lax; Path=/",
		tenantID.String()[:8], userID.String()[:8], int(ttl.Seconds())), nil
}
