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
// returns nil if and only if a provider accepted the message. Every other
// outcome returns a classified error. Callers map that to a
// billing_emails_skipped_total{template,reason} increment; they must never
// increment a *_sent_total counter for it.

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
)

// Additional reason labels, complementing those in recipient.go.
const (
	ReasonRenderFailed    = "render_failed"
	ReasonTransportFailed = "transport_failed"
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
	default:
		return ReasonUnknown
	}
}

type templateClient struct {
	loader *emailtemplates.Loader
	sender Sender
	from   string
	logger *slog.Logger
}

// NewTemplateClient returns the production Client. loader and sender are
// required; a nil logger falls back to slog.Default().
func NewTemplateClient(loader *emailtemplates.Loader, sender Sender, from string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &templateClient{loader: loader, sender: sender, from: from, logger: logger}
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
		return fmt.Errorf("email: render %s: %w: %v", template, ErrRender, err)
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
		return fmt.Errorf("email: send %s: %w: %v", template, ErrTransport, err)
	}
	return nil
}
