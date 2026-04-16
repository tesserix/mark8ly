// Package autologin issues a session cookie for a user who just completed
// onboarding, without forcing them through a separate sign-in step.
//
// The flow:
//
//   1. apps/onboarding completes a session via platform-api → tenant + user exist
//   2. Frontend redirects to /auth/auto-login with a tenant_id and id_token
//   3. auto-login handler:
//      a. Verifies the id_token via GIP (multi-tenant aware)
//      b. CheckMembership against OpenFGA — IS THE TUPLE THERE YET?
//      c. If yes: mint session cookie, return success
//      d. If no:  retry CheckMembership with backoff up to ~2 seconds
//      e. After retry budget: return 503, frontend can retry the call
//
// Step (b)+(d) is the auth-bug #2 fix on the auth-bff side. Even if the
// outbox drainer hasn't shipped the FGA write yet, the autologin call won't
// hand back a session before the tuple is visible. The user will see a
// 1-2 second delay at worst, never a "tenant not found" error.
package autologin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mark8ly/auth-bff/internal/audit"
	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/usersessions"
)

// MFAStatusChecker is the narrow interface autologin needs from the
// MFA service: "is this user enrolled and enabled?" Kept as an
// interface so the autologin package doesn't take a hard dependency
// on the usermfa types.
type MFAStatusChecker interface {
	IsEnabled(ctx context.Context, userID string) (bool, error)
}

// Service is the autologin business logic.
type Service struct {
	gip      gip.Verifier
	fga      authz.Client
	sessions *session.Manager
	registry *usersessions.Repository
	mfa      MFAStatusChecker
	audit    *audit.Client // optional — nil/no-op when marketplace-api is not wired
	logger   *slog.Logger
	policy   RetryPolicy
}

// RetryPolicy controls the FGA-check retry loop. Defaults are conservative.
type RetryPolicy struct {
	// MaxAttempts is the total number of CheckMembership calls (including
	// the first). 0 → 8 (default).
	MaxAttempts int
	// InitialBackoff is the wait before the second attempt. 0 → 50ms.
	InitialBackoff time.Duration
	// MaxBackoff caps the per-attempt wait. 0 → 500ms.
	MaxBackoff time.Duration
}

// Config holds Service dependencies.
type Config struct {
	GIP      gip.Verifier
	FGA      authz.Client
	Sessions *session.Manager
	// Registry is the side-table of active sessions. Optional — when
	// nil, the autologin path still mints JWT cookies normally and the
	// admin "Active sessions" UI just stays empty.
	Registry *usersessions.Repository
	// MFA is the TOTP enrolment checker. Optional — when nil, every
	// login skips the challenge step (pre-MFA behaviour). When wired,
	// AutoLogin short-circuits to MintPending + MFARequired whenever
	// the user has a verified enrolment on file.
	MFA MFAStatusChecker
	// Audit posts cross-service audit events to marketplace-api. Optional.
	Audit  *audit.Client
	Logger *slog.Logger
	Policy RetryPolicy
}

// NewService constructs a Service. Defaults are applied to Policy.
func NewService(cfg Config) *Service {
	p := cfg.Policy
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 8
	}
	if p.InitialBackoff == 0 {
		p.InitialBackoff = 50 * time.Millisecond
	}
	if p.MaxBackoff == 0 {
		p.MaxBackoff = 500 * time.Millisecond
	}
	return &Service{
		gip:      cfg.GIP,
		fga:      cfg.FGA,
		sessions: cfg.Sessions,
		registry: cfg.Registry,
		mfa:      cfg.MFA,
		audit:    cfg.Audit,
		logger:   cfg.Logger,
		policy:   p,
	}
}

// Request is the input to AutoLogin.
type Request struct {
	IDToken          string // Firebase/GIP ID token from the frontend
	ExpectedTenantID string // The GIP tenant pool we expect (e.g. MP-Internal-...)
	WorkspaceTenant  string // The Mark8ly tenant UUID the user is logging into
	// Client metadata captured for the sessions UI. Best-effort — when
	// any field is empty, the row shows a sensible placeholder ("Browser").
	Device    string
	IPAddress string
	UserAgent string
}

// Result is what AutoLogin returns on success.
type Result struct {
	UID      string
	Email    string
	TenantID string
	// MFARequired is true when the user has MFA enabled and the session
	// cookie was NOT minted. Instead a short-lived pending cookie was
	// written; the caller must complete POST /auth/mfa-challenge before
	// any authenticated request will succeed.
	MFARequired bool
}

// Errors. Each maps to a specific HTTP response in the handler.
var (
	ErrTokenInvalid    = errors.New("autologin: token invalid")
	ErrTenantMismatch  = errors.New("autologin: token tenant pool does not match")
	ErrNotMember       = errors.New("autologin: user is not a member of the tenant")
	ErrFGAUnreachable  = errors.New("autologin: openfga is unreachable")
	ErrSessionMintFail = errors.New("autologin: failed to mint session")
)

// AutoLogin verifies the ID token, confirms tenant membership (with retry
// to close the auth-bug #2 race window), mints a session cookie onto the
// response, and returns the result.
func (s *Service) AutoLogin(ctx context.Context, w http.ResponseWriter, req Request) (*Result, error) {
	if req.IDToken == "" || req.ExpectedTenantID == "" || req.WorkspaceTenant == "" {
		return nil, ErrTokenInvalid
	}

	// Step 1: verify the ID token (GIP signature, expiry, audience, tenant claim).
	tok, err := s.gip.VerifyToken(ctx, req.IDToken, req.ExpectedTenantID)
	if err != nil {
		switch {
		case errors.Is(err, gip.ErrTenantMismatch):
			return nil, ErrTenantMismatch
		default:
			return nil, fmt.Errorf("%w: %s", ErrTokenInvalid, err)
		}
	}

	// Step 2: check FGA membership with retry. THE BUG-FIX LOOP.
	if err := s.checkMembershipWithRetry(ctx, tok.UID, req.WorkspaceTenant); err != nil {
		return nil, err
	}

	// Step 2b: gate on MFA enrolment. Users with a verified TOTP
	// enrolment must complete a second factor before they see a real
	// session cookie. We write a short-lived pending cookie carrying
	// just enough context to finish the challenge without re-verifying
	// the GIP token, and flag MFARequired on the result so the handler
	// can respond with the right HTTP shape.
	if s.mfa != nil {
		enabled, err := s.mfa.IsEnabled(ctx, tok.UID)
		if err != nil && s.logger != nil {
			s.logger.Warn("autologin: mfa status check failed — treating as disabled",
				"err", err, "user_id", tok.UID)
		}
		if enabled {
			if err := s.sessions.MintPending(w, session.Pending{
				UID:      tok.UID,
				Email:    tok.Email,
				TenantID: req.WorkspaceTenant,
			}); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrSessionMintFail, err)
			}
			return &Result{
				UID:         tok.UID,
				Email:       tok.Email,
				TenantID:    req.WorkspaceTenant,
				MFARequired: true,
			}, nil
		}
	}

	// Step 3: mint the session cookie.
	if err := s.sessions.Mint(w, session.Session{
		UID:      tok.UID,
		Email:    tok.Email,
		TenantID: req.WorkspaceTenant,
	}); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSessionMintFail, err)
	}

	// Step 4 (best-effort): record the new session in the registry for
	// the admin "Active sessions" UI. A DB failure here must not break
	// login — the user is already authenticated and the cookie is set.
	if s.registry != nil {
		device := req.Device
		if device == "" {
			device = "Browser"
		}
		if _, err := s.registry.Create(ctx, usersessions.CreateParams{
			UserID:    tok.UID,
			TenantID:  req.WorkspaceTenant,
			Device:    device,
			IPAddress: req.IPAddress,
			UserAgent: req.UserAgent,
		}); err != nil && s.logger != nil {
			s.logger.Warn("usersessions: create failed", "err", err, "user_id", tok.UID)
		}
	}

	// Step 5 (best-effort): emit cross-service audit event. Async, never
	// blocks login — a slow or down marketplace-api must not delay the
	// happy path.
	s.audit.EmitAsync(audit.Event{
		TenantID:    req.WorkspaceTenant,
		Action:      "user.signed_in",
		ResourceType: "user",
		ResourceID:  tok.UID,
		ActorType:   "user",
		ActorUserID: tok.UID,
		ActorEmail:  tok.Email,
		IPAddress:   req.IPAddress,
		UserAgent:   req.UserAgent,
		Metadata:    map[string]any{"device": req.Device, "method": "auto_login"},
	})

	return &Result{
		UID:      tok.UID,
		Email:    tok.Email,
		TenantID: req.WorkspaceTenant,
	}, nil
}

// checkMembershipWithRetry is the auth-bug #2 fix on the auth-bff side.
//
// The outbox drainer in platform-api ships the FGA tuple a moment after the
// onboarding completion DB tx commits. There's a tiny window (typically
// 0-200ms) between commit and tuple visibility. Without retry, the autologin
// call races the drainer and loses ~10% of the time, causing "tenant not
// found" right after onboarding.
//
// With retry: the call waits up to MaxAttempts × backoff for the tuple to
// appear. Total budget is ~2 seconds in the default policy. If the tuple
// is still missing after that, we return ErrNotMember (the drainer is
// genuinely failing — admin should investigate).
func (s *Service) checkMembershipWithRetry(ctx context.Context, userID, tenantID string) error {
	backoff := s.policy.InitialBackoff
	var lastErr error

	for attempt := 1; attempt <= s.policy.MaxAttempts; attempt++ {
		ok, err := s.fga.CheckMembership(ctx, userID, tenantID)
		if err != nil {
			lastErr = err
			// Network/FGA error: retry with backoff (transient).
		} else if ok {
			// Membership tuple is visible — we're done.
			return nil
		}
		// Either ok=false (tuple not yet visible) or err != nil (transient).
		// Either way: back off and try again.

		if attempt == s.policy.MaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > s.policy.MaxBackoff {
			backoff = s.policy.MaxBackoff
		}
	}

	if lastErr != nil {
		return fmt.Errorf("%w: %s", ErrFGAUnreachable, lastErr)
	}
	return ErrNotMember
}
