// Package autologin issues a session cookie for a user who just completed
// onboarding, without forcing them through a separate sign-in step.
//
// The flow:
//
//  1. apps/onboarding completes a session via platform-api → tenant + user exist
//  2. Frontend redirects to /auth/auto-login with a tenant_id and id_token
//  3. auto-login handler:
//     a. Verifies the id_token via GIP (multi-tenant aware)
//     b. CheckMembership against OpenFGA — IS THE TUPLE THERE YET?
//     c. If yes: mint session cookie, return success
//     d. If no:  retry CheckMembership with backoff up to ~2 seconds
//     e. After retry budget: return 503, frontend can retry the call
//
// Step (b)+(d) is the auth-bug #2 fix on the auth-bff side. Even if the
// outbox drainer hasn't shipped the FGA write yet, the autologin call won't
// hand back a session before the tuple is visible. The user will see a
// 1-2 second delay at worst, never a "tenant not found" error.
// GIP-708 SPLIT: this file is full of the string "gip" but must NOT be deleted
// wholesale. completeLogin is SHARED — Zitadel's CompleteForProvider calls it,
// so removing the file takes out the live Zitadel login gauntlet. Only
// AutoLogin and the gip field are GIP-only.
// See docs/auth/2026-09-07-gip-removal-audit.md.
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
	"github.com/mark8ly/auth-bff/internal/deviceguard"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/usersessions"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
)

// MFAStatusChecker is the narrow interface autologin needs from the
// MFA service: "is this user enrolled and enabled?" Kept as an
// interface so the autologin package doesn't take a hard dependency
// on the usermfa types.
type MFAStatusChecker interface {
	IsEnabled(ctx context.Context, userID string) (bool, error)
}

// DeviceEvaluator reports whether a login came from a device the
// account has not used before, alerting the user when it has not.
type DeviceEvaluator interface {
	Evaluate(ctx context.Context, l deviceguard.Login) (bool, error)
}

// ChallengeIssuer mails a one-time sign-in code to the address.
type ChallengeIssuer interface {
	IssueChallenge(ctx context.Context, email, ip string) error
}

// Service is the autologin business logic.
type Service struct {
	gip      gip.Verifier
	fga      authz.Client
	sessions *session.Manager
	registry *usersessions.Repository
	mfa      MFAStatusChecker
	devices  DeviceEvaluator
	emailOTP ChallengeIssuer
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
	// Devices raises new-device security alerts. Optional — when nil,
	// logins are not checked against device history.
	Devices DeviceEvaluator
	// EmailOTP gates unrecognised devices behind a mailed code. Optional
	// — when nil, a new device is alerted about but not challenged,
	// which is the pre-feature behaviour.
	EmailOTP ChallengeIssuer
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
		devices:  cfg.Devices,
		emailOTP: cfg.EmailOTP,
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
	// Country is an ISO-3166 alpha-2 code resolved at the edge, or "".
	Country string
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
	// EmailOTPRequired is true when the login came from an unrecognised
	// device and a code was mailed instead of a session being minted.
	// The caller must complete POST /auth/otp/verify.
	EmailOTPRequired bool
}

// Errors. Each maps to a specific HTTP response in the handler.
var (
	ErrTokenInvalid    = errors.New("autologin: token invalid")
	ErrTenantMismatch  = errors.New("autologin: token tenant pool does not match")
	ErrNotMember       = errors.New("autologin: user is not a member of the tenant")
	ErrFGAUnreachable  = errors.New("autologin: openfga is unreachable")
	ErrSessionMintFail = errors.New("autologin: failed to mint session")
	// ErrChallengeSendFail means the device was unrecognised but the code
	// could not be delivered. Deliberately fatal to the login: falling
	// through to a session would let anyone who can disrupt the mail path
	// walk straight past the new-device gate.
	ErrChallengeSendFail = errors.New("autologin: failed to send sign-in code")
)

// Identity is the outcome of authenticating a user, independent of which
// provider did it. Everything downstream of this type — membership, MFA,
// device and OTP gating, session minting — is provider-agnostic and shared
// between the GIP and Zitadel paths.
type Identity struct {
	UID      string
	Email    string
	TenantID string
	// Provider labels the audit event's "method" field with who actually
	// authenticated this login. Empty means GIP/AutoLogin's own default
	// ("auto_login") applies; CompleteForProvider sets this explicitly so a
	// Zitadel login is not misattributed to GIP in the audit trail.
	Provider string
}

// auditMethod returns the audit event's "method" label for id, defaulting to
// "auto_login" (the GIP path's historical value) when no provider is set.
func auditMethod(id Identity) string {
	if id.Provider == "" {
		return "auto_login"
	}
	return id.Provider
}

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

	return s.completeLogin(ctx, w, Identity{
		UID:      tok.UID,
		Email:    tok.Email,
		TenantID: tok.TenantID,
	}, req)
}

// CompleteForProvider is the exported entry point for a provider other than
// GIP — currently Zitadel — that has already established a verified identity
// by its own means (password + sufficiency checks) and just needs to run the
// shared gauntlet: FGA membership, the MFA gate, deviceguard, the email-OTP
// step-up, session minting.
//
// It is a thin wrapper around completeLogin so that gauntlet has exactly one
// implementation regardless of which provider authenticated the user, and it
// maps zitadellogin.LoginContext onto Request field-for-field so UserAgent,
// IPAddress, Device and Country reach completeLogin exactly as they do on the
// GIP path. Passing these empty is not an option: deviceguard fingerprints
// the user agent, and Fingerprint("") is a constant every user would share —
// silently collapsing new-device detection for every Zitadel login.
func (s *Service) CompleteForProvider(ctx context.Context, w http.ResponseWriter, lc zitadellogin.LoginContext) (zitadellogin.CompleteResult, error) {
	res, err := s.completeLogin(ctx, w, Identity{
		UID:      lc.UID,
		Email:    lc.Email,
		TenantID: lc.TenantID,
		Provider: "zitadel",
	}, Request{
		WorkspaceTenant: lc.TenantID,
		Device:          lc.Device,
		IPAddress:       lc.IPAddress,
		UserAgent:       lc.UserAgent,
		Country:         lc.Country,
	})
	if err != nil {
		return zitadellogin.CompleteResult{}, err
	}
	return zitadellogin.CompleteResult{
		MFARequired:      res.MFARequired,
		EmailOTPRequired: res.EmailOTPRequired,
	}, nil
}

// completeLogin runs every gate that stands between a verified identity and a
// minted session: FGA membership (with the outbox-race retry), the MFA gate,
// new-device evaluation, the email-OTP step-up, then the session cookie,
// registry row and audit event.
//
// It is deliberately provider-agnostic. A Zitadel login that has satisfied its
// own credential and factor checks arrives here with the same Identity a
// verified GIP token produces, and is subject to exactly the same gates — so
// the two providers cannot drift apart in what they enforce after login.
func (s *Service) completeLogin(ctx context.Context, w http.ResponseWriter, id Identity, req Request) (*Result, error) {
	// Step 2: check FGA membership with retry. THE BUG-FIX LOOP.
	if err := s.checkMembershipWithRetry(ctx, id.UID, req.WorkspaceTenant); err != nil {
		return nil, err
	}

	// Step 2b: gate on MFA enrolment. Users with a verified TOTP
	// enrolment must complete a second factor before they see a real
	// session cookie. We write a short-lived pending cookie carrying
	// just enough context to finish the challenge without re-verifying
	// the GIP token, and flag MFARequired on the result so the handler
	// can respond with the right HTTP shape.
	if s.mfa != nil {
		enabled, err := s.mfa.IsEnabled(ctx, id.UID)
		if err != nil && s.logger != nil {
			s.logger.Warn("autologin: mfa status check failed — treating as disabled",
				"err", err, "user_id", id.UID)
		}
		if enabled {
			if err := s.sessions.MintPending(w, session.Pending{
				UID:      id.UID,
				Email:    id.Email,
				TenantID: req.WorkspaceTenant,
			}); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrSessionMintFail, err)
			}
			return &Result{
				UID:         id.UID,
				Email:       id.Email,
				TenantID:    req.WorkspaceTenant,
				MFARequired: true,
			}, nil
		}
	}

	device := req.Device
	if device == "" {
		device = "Browser"
	}
	fingerprint := deviceguard.Fingerprint(req.UserAgent)

	// Step 2c: device history. Evaluated BEFORE the session row is
	// written, otherwise the row we are about to insert would itself
	// make this device look familiar and every alert would be
	// suppressed. Evaluate also dispatches the new-device alert, which
	// is why it runs even when the login is about to be challenged —
	// the attempt is what the account holder needs to hear about.
	newDevice := false
	if s.devices != nil {
		isNew, err := s.devices.Evaluate(ctx, deviceguard.Login{
			UserID:      id.UID,
			Email:       id.Email,
			Fingerprint: fingerprint,
			Device:      device,
			IPAddress:   req.IPAddress,
			Country:     req.Country,
			At:          time.Now().UTC(),
		})
		if err != nil && s.logger != nil {
			s.logger.Warn("deviceguard: evaluate failed", "err", err, "user_id", id.UID)
		}
		newDevice = isNew
	}

	// Step 2d: an unrecognised device must prove control of the account's
	// email before it gets a session. This is what makes signing in on a
	// second device safe rather than merely possible.
	if newDevice && s.emailOTP != nil {
		if err := s.emailOTP.IssueChallenge(ctx, id.Email, req.IPAddress); err != nil {
			if s.logger != nil {
				s.logger.Error("autologin: challenge send failed",
					"err", err, "user_id", id.UID, "tenant_id", req.WorkspaceTenant)
			}
			return nil, fmt.Errorf("%w: %w", ErrChallengeSendFail, err)
		}
		if err := s.sessions.MintPending(w, session.Pending{
			UID:         id.UID,
			Email:       id.Email,
			TenantID:    req.WorkspaceTenant,
			Fingerprint: fingerprint,
			Device:      device,
			IPAddress:   req.IPAddress,
		}); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSessionMintFail, err)
		}
		return &Result{
			UID:              id.UID,
			Email:            id.Email,
			TenantID:         req.WorkspaceTenant,
			EmailOTPRequired: true,
		}, nil
	}

	// Step 3: mint the session cookie.
	if err := s.sessions.Mint(w, session.Session{
		UID:      id.UID,
		Email:    id.Email,
		TenantID: req.WorkspaceTenant,
	}); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionMintFail, err)
	}

	// Step 4 (best-effort): record the new session in the registry for
	// the admin "Active sessions" UI. A DB failure here must not break
	// login — the user is already authenticated and the cookie is set.
	if s.registry != nil {
		if _, err := s.registry.Create(ctx, usersessions.CreateParams{
			UserID:      id.UID,
			TenantID:    req.WorkspaceTenant,
			Device:      device,
			IPAddress:   req.IPAddress,
			UserAgent:   req.UserAgent,
			Fingerprint: fingerprint,
		}); err != nil && s.logger != nil {
			s.logger.Warn("usersessions: create failed", "err", err, "user_id", id.UID)
		}
	}

	// Step 5 (best-effort): emit cross-service audit event. Async, never
	// blocks login — a slow or down marketplace-api must not delay the
	// happy path.
	s.audit.EmitAsync(audit.Event{
		TenantID:     req.WorkspaceTenant,
		Action:       "user.signed_in",
		ResourceType: "user",
		ResourceID:   id.UID,
		ActorType:    "user",
		ActorUserID:  id.UID,
		ActorEmail:   id.Email,
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
		Metadata:     map[string]any{"device": req.Device, "method": auditMethod(id)},
	})

	return &Result{
		UID:      id.UID,
		Email:    id.Email,
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
	// An unwired client is an outage, not a crash — report it as such
	// rather than dereferencing nil into a recovered panic and a 500.
	if s.fga == nil {
		return fmt.Errorf("%w: authz client not configured", ErrFGAUnreachable)
	}

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
