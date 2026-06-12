// sender.go — shared provider transport for ALL outbound email.
//
// Every product mailer (campaign, ticket, giftcard, orderdoc, shipping
// label) renders its own envelope and hands the result to a Sender; the
// adapters in this package translate the provider-agnostic Message into
// each provider's wire format. Production wiring is SendGrid (primary)
// → Resend (fallback) via FallbackSender so a SendGrid outage degrades
// to a provider switch instead of dropped transactional mail. Mirrors
// platform-api/internal/notification so both services fail over the
// same way.
package email

import (
	"context"
	"fmt"
	"log/slog"
)

// Message is one outbound email, provider-agnostic. The product mailers
// own rendering (subjects, templates, theming); Message is purely the
// finished envelope plus attribution metadata.
type Message struct {
	From     string // sender address — required
	FromName string // optional display name (e.g. "Mark8ly Support")
	To       string // recipient address — required
	ToName   string // optional recipient display name

	Subject  string
	HTMLBody string // either body may be empty, but not both
	TextBody string

	// CustomArgs carries attribution metadata (product / kind / tenant_id /
	// campaign_id) — see the notification-service Wave 1.5 webhook
	// receiver. SendGrid echoes them back as custom_args on every
	// engagement event (delivered/open/click/bounce); Resend mirrors them
	// as tags. Always emit `product` so cross-product surfaces stay
	// distinguishable.
	CustomArgs map[string]string

	// Attachments ride along on the outgoing email (order PDFs, shipping
	// labels). Content is raw bytes — each adapter base64-encodes per its
	// own wire format, so callers never deal with encoding.
	Attachments []Attachment
}

// Attachment is one file attached to a Message.
type Attachment struct {
	Filename    string
	ContentType string // e.g. "application/pdf"
	Content     []byte // raw bytes — adapters encode on the wire
}

// Sender delivers a single Message.
//
// Implementations:
//   - SendGridSender — primary provider, SendGrid v3 HTTP API
//   - ResendSender   — fallback provider, Resend HTTP API
//   - FallbackSender — chains primary → fallback per message
//   - LogSender      — dev / no-keys mode, logs instead of sending
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// validate rejects Messages no provider would accept. Centralised so
// FallbackSender can fail fast instead of burning both providers' quota
// on a malformed message.
func validate(msg Message) error {
	if msg.From == "" {
		return fmt.Errorf("email: missing from address")
	}
	if msg.To == "" {
		return fmt.Errorf("email: missing to address")
	}
	if msg.Subject == "" {
		return fmt.Errorf("email: missing subject")
	}
	if msg.HTMLBody == "" && msg.TextBody == "" {
		return fmt.Errorf("email: missing body")
	}
	return nil
}

// Provider construction, ordering and the config-driven chain live in
// providers.go.

// LogSender logs the would-be send instead of delivering. Wired when no
// provider API key is configured (local dev, CI) so the full dispatch
// path — rendering, attribution, attachments — stays exercised without
// a provider account.
type LogSender struct {
	Logger *slog.Logger
}

// Send logs the message at info level and returns nil without any I/O.
func (s *LogSender) Send(_ context.Context, msg Message) error {
	if s.Logger == nil {
		return nil
	}
	s.Logger.Info("email: send (log-only)",
		"to", msg.To,
		"subject", msg.Subject,
		"custom_args", msg.CustomArgs,
		"html_bytes", len(msg.HTMLBody),
		"text_bytes", len(msg.TextBody),
		"attachments", len(msg.Attachments))
	return nil
}

// Name identifies the log-only transport in fallback-chain logs.
func (s *LogSender) Name() string { return "log" }
