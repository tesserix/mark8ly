// Package notification sends transactional emails (and, eventually, push /
// SMS / webhooks). It is intentionally inlined into platform-api during the
// onboarding-only first slice rather than being a separate service —
// extraction is trivial later if there's a second consumer or worker shape.
//
// Design notes (clean-up vs old notification-service):
//
//   - Templates are EMBEDDED in the binary, not stored in the database.
//     Versioned with code, type-safe variables, tested in unit tests, no DB
//     hop, syntax errors caught at compile time. The DB-backed model is a
//     valid future addition (DBSender wrapping the embedded sender) but is
//     overkill pre-launch.
//
//   - Each template has a typed Vars struct. The Render functions take
//     concrete Vars, not map[string]any. Compiler enforces variable presence.
//
//   - Plain HTML + text/plain together. Real email clients want both.
//     Spam filters downgrade text-only emails. Accessibility tools prefer
//     plain. The Sender interface accepts both as a single Email struct.
//
//   - One Sender interface, three implementations: SendGrid (production),
//     LogSender (local dev — prints to stdout), NoopSender (tests).
//     The verification service depends on the interface, not on SendGrid.
package notification

import (
	"context"
	"errors"
)

// Email is one outbound message. Both HTML and text bodies are required so
// recipients on every email client get a usable rendering.
type Email struct {
	To       string
	From     string
	Subject  string
	HTMLBody string
	TextBody string
}

// Sender dispatches an Email. Implementations:
//   - SendGridSender   — production
//   - LogSender        — local dev (logs the OTP to stdout for testing)
//   - NoopSender       — unit tests
type Sender interface {
	Send(ctx context.Context, msg Email) error
}

// ErrNoRecipient is returned when an Email has no To address.
var ErrNoRecipient = errors.New("notification: missing recipient")

// validate ensures the Email has the minimum fields needed to ship.
// Implementations should call this before any provider call.
func validate(msg Email) error {
	if msg.To == "" {
		return ErrNoRecipient
	}
	if msg.Subject == "" {
		return errors.New("notification: missing subject")
	}
	if msg.HTMLBody == "" && msg.TextBody == "" {
		return errors.New("notification: missing body (need at least one of html_body or text_body)")
	}
	return nil
}
