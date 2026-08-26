package email

// template_client.go — the production implementation of Client.
//
// Before this landed, Client had exactly one implementation (NoOpClient),
// so no merchant had ever received a dunning notice, trial reminder,
// payment-action reminder, win-back promo or trial-billed confirmation
// (#381). This adapter renders a template key through the shared
// emailtemplates registry — the same one orderdoc and giftcard use, so
// operators can reword billing copy without a deploy — and hands the
// finished envelope to the shared SendGrid→Resend Sender chain.
//
// Contract, and the reason the delivery counters can be trusted: Send
// returns nil if and only if a real provider accepted the message. Every
// other outcome — including the log-only dev transport, which delivers
// nothing — returns a classified error. Callers map that to a
// billing_emails_skipped_total{template,reason} increment; they must never
// increment a *_sent_total counter for it.
//
// The log-only case is called out explicitly because it is the pre-#381
// failure in a new costume: NewFromConfig falls back to LogSender when
// neither SENDGRID_API_KEY nor RESEND_API_KEY is set, and LogSender.Send
// returns nil. Reporting that as success would put dashboards back to
// showing delivery while nothing is sent, one missing secret away. Local
// dev still works — the send is logged exactly as before — the metric just
// stops lying: the caller counts ErrNoProvider as a `no_provider` skip.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// fromName is the display name on every billing email.
const fromName = "Mark8ly Billing"

// Sentinels so callers can classify a failure without string matching.
var (
	// ErrRender means the template could not be rendered — an unknown key,
	// or a published DB override with broken syntax.
	ErrRender = errors.New("template render failed")
	// ErrTransport means every configured provider refused the message.
	ErrTransport = errors.New("transport failed")
	// ErrNoProvider means no real provider is configured, so the message
	// was only logged. Nothing was delivered.
	ErrNoProvider = errors.New("no email provider configured")
)

// Additional reason labels, complementing those in recipient.go.
const (
	ReasonRenderFailed    = "render_failed"
	ReasonTransportFailed = "transport_failed"
	ReasonNoProvider      = "no_provider"
	ReasonUnknown         = "unknown"
)

// SkipReason maps a Send error onto a stable metric label.
func SkipReason(err error) string {
	if reason, ok := UndeliverableReason(err); ok {
		return reason
	}
	switch {
	case errors.Is(err, ErrRender):
		return ReasonRenderFailed
	case errors.Is(err, ErrTransport):
		return ReasonTransportFailed
	case errors.Is(err, ErrNoProvider):
		return ReasonNoProvider
	default:
		return ReasonUnknown
	}
}

type templateClient struct {
	loader *emailtemplates.Loader
	sender Sender
	from   string
	logger *slog.Logger
	// logOnly is true when sender delivers nothing (no provider key set).
	// Send then reports ErrNoProvider instead of nil, so no caller can
	// increment a *_sent_total for a message that never left the process.
	logOnly bool
}

// isLogOnly reports whether s is the dev transport that logs instead of
// delivering. Matched both by concrete type and by the Named contract, so
// any future log-only adapter naming itself "log" is caught too.
func isLogOnly(s Sender) bool {
	if _, ok := s.(*LogSender); ok {
		return true
	}
	if n, ok := s.(Named); ok && n.Name() == "log" {
		return true
	}
	return false
}

// NewTemplateClient returns the production Client. loader and sender are
// required; a nil logger falls back to slog.Default().
func NewTemplateClient(loader *emailtemplates.Loader, sender Sender, from string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &templateClient{
		loader:  loader,
		sender:  sender,
		from:    from,
		logger:  logger,
		logOnly: isLogOnly(sender),
	}
}

// Send renders `template` with `data` and delivers it to `to`.
//
// `to` is an email address. It used to be a store UUID at four call sites
// and a Stripe customer ID at a fifth; callers now resolve the address from
// store_subscriptions.email before calling.
func (c *templateClient) Send(ctx context.Context, template TemplateID, to string, data map[string]any) error {
	if err := ValidateRecipient(to); err != nil {
		c.logger.Warn("billing email: undeliverable recipient; not sending",
			"template", string(template), "reason", SkipReason(err))
		return err
	}

	rendered, err := c.loader.Render(ctx, string(template), data)
	if err != nil {
		c.logger.Error("billing email: render failed",
			"template", string(template), "err", err.Error())
		return fmt.Errorf("email: render %s: %w: %w", template, ErrRender, err)
	}

	msg := Message{
		From:     c.from,
		FromName: fromName,
		To:       strings.TrimSpace(to),
		Subject:  rendered.Subject,
		HTMLBody: rendered.HTMLBody,
		TextBody: rendered.TextBody,
		// Wave 1.5 attribution — the same shape the five working mailers
		// emit, so the notification-service webhook receiver groups these
		// without parsing subjects, and #348's send log picks them up free.
		CustomArgs: map[string]string{
			"product": "mark8ly",
			"kind":    string(template),
		},
	}
	if tenantID, ok := data["tenant_id"].(string); ok && tenantID != "" {
		msg.CustomArgs["tenant_id"] = tenantID
	}

	if err := c.sender.Send(ctx, msg); err != nil {
		c.logger.Error("billing email: transport failed",
			"template", string(template), "err", err.Error())
		return fmt.Errorf("email: send %s: %w: %w", template, ErrTransport, err)
	}

	// The message was logged, not delivered. Say so, rather than let the
	// caller record a delivery.
	if c.logOnly {
		c.logger.Warn("billing email: logged only — no provider configured",
			"template", string(template))
		return fmt.Errorf("email: send %s: %w", template, ErrNoProvider)
	}
	return nil
}
