// Package auth owns the platform-api side of the merchant password
// flows. Sign-in itself is handled by auth-bff against the identity
// provider; this package exists to move the password-reset email out of
// the provider's own default handler (plain text + an exposed vendor
// action URL) and into our branded flow.
//
// The flow is:
//  1. Admin app POSTs to /internal/auth/password-reset/request with
//     an email.
//  2. Service asks the configured PasswordResetProvider to mint a reset
//     code WITHOUT the provider sending its own email.
//  3. Service renders the password_reset HTML/text template with a
//     link to {admin}/reset-password?oobCode=... and dispatches via
//     the shared SendGrid sender.
//  4. User clicks the link, lands in the admin app, types a new
//     password, and the admin POSTs to /internal/auth/password-reset/
//     confirm with the oob code and new password.
//  5. Service hands the code and new password back to the provider. The
//     code itself is proof-of-possession, so no admin token is required
//     for confirm.
//
// The provider is *zitadeladmin.Client (#791 retired the GIP one). The
// interface below is the whole contract; this package never names a
// vendor.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark8ly/platform-api/internal/idperr"
	"github.com/mark8ly/platform-api/internal/notification"
)

// PasswordResetProvider is the two-call contract the password-reset flow
// needs from an identity provider: mint a reset code, then redeem one.
// Defined as an interface — rather than naming a concrete client — so the
// provider can be swapped without this package changing. It was, in #791:
// the GIP client that originally satisfied it is gone and
// *zitadeladmin.Client satisfies it unchanged.
//
// Construct the interface value ONLY when a real, non-nil client exists —
// never assign a possibly-nil concrete pointer straight into Config.Admin.
// Doing so would produce a non-nil interface value wrapping a nil pointer
// (the "typed nil" trap documented in cmd/server/account_wiring.go): every
// method below unconditionally dereferences the client, so a caller's
// `!= nil` guard would be defeated and the eventual call would panic.
// cmd/server/provider_wiring.go's selectAccountProviders is the single
// place that performs that guard; cmd/server/main.go only calls
// auth.NewService when it returned a genuine, non-nil provider.
//
// Implementations classify their upstream failures into internal/idperr's
// sentinels, which handler.go maps to HTTP statuses.
type PasswordResetProvider interface {
	SendPasswordResetOobCode(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, oobCode, newPassword string) error
}

// Config bundles the dependencies and tunables for Service.
type Config struct {
	Admin  PasswordResetProvider
	Sender notification.Sender
	// Loader is the DB-backed template loader (embedded fallback). May
	// be nil during boot races; nil falls through to embedded rendering.
	Loader       *notification.Loader
	EmailFrom    string
	SupportEmail string
	// AdminResetBaseURL is the origin users land on after clicking
	// the reset email (e.g. https://admin.mark8ly.com). The oob code
	// is appended as a query string by buildResetURL.
	AdminResetBaseURL string
	Logger            *slog.Logger
}

// Service orchestrates the password-reset flow.
type Service struct {
	cfg Config
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

// RequestPasswordReset asks GIP for an oobCode and emails a branded
// reset link. Returns nil for both "email sent" and "email not found"
// so callers do not leak account existence; the caller should always
// return 204 to the browser regardless of the underlying outcome.
// Internal upstream failures still return an error so the caller can
// fail-closed with a 502.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return fmt.Errorf("auth: email is required")
	}

	oobCode, err := s.cfg.Admin.SendPasswordResetOobCode(ctx, email)
	if err != nil {
		// Suppress enumeration: if the account doesn't exist, pretend
		// success. Everything else is a real outage.
		if errors.Is(err, idperr.ErrUserNotFound) {
			if s.cfg.Logger != nil {
				s.cfg.Logger.Info("auth: password reset requested for unknown email",
					"email", email)
			}
			return nil
		}
		return err
	}

	resetURL := buildResetURL(s.cfg.AdminResetBaseURL, oobCode)
	vars := notification.PasswordResetVars{
		ResetURL:     resetURL,
		ExpiresIn:    "1 hour",
		SupportEmail: s.cfg.SupportEmail,
	}
	// Try DB-loaded template first (operator-edited); fall back to
	// embedded if no Loader is wired or DB is unreachable. Password
	// reset is cross-tenant — no tenant_id to forward.
	var msg notification.Email
	if s.cfg.Loader != nil {
		msg, err = s.cfg.Loader.Render(ctx, "password_reset", email, s.cfg.EmailFrom, "", vars)
	} else {
		msg, err = notification.RenderPasswordReset(email, s.cfg.EmailFrom, vars)
	}
	if err != nil {
		return fmt.Errorf("auth: render reset email: %w", err)
	}

	if err := s.cfg.Sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("auth: send reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset validates the oob code and sets a new password.
// The service layer enforces only a minimal length check; GIP itself
// does the real policy enforcement (length, strength, rate limits).
func (s *Service) ConfirmPasswordReset(ctx context.Context, oobCode, newPassword string) error {
	oobCode = strings.TrimSpace(oobCode)
	newPassword = strings.TrimSpace(newPassword)
	if oobCode == "" {
		return fmt.Errorf("auth: oob_code is required")
	}
	if len(newPassword) < 8 {
		return idperr.ErrWeakPassword
	}
	return s.cfg.Admin.ResetPassword(ctx, oobCode, newPassword)
}

// buildResetURL assembles the reset URL the user lands on. Uses a
// simple query string instead of a fragment so the admin middleware
// can forward search params to the client page component without
// extra plumbing.
func buildResetURL(base, oobCode string) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s/reset-password%soobCode=%s", strings.TrimRight(base, "/"), sep, oobCode)
}
