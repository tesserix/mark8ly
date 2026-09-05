package campaign

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/email"
)

// EmailDispatcher sends pre-rendered campaign emails through the shared
// provider transport (SendGrid primary, Resend fallback — see
// internal/email). Templating + theming happens in the send worker (so
// the store branding is loaded once per campaign, not once per
// recipient) — this dispatcher is a thin transport adapter.
type EmailDispatcher struct {
	sender email.Sender
	from   string
	logger *slog.Logger
}

// NewEmailDispatcher constructs a Dispatcher on top of the shared email
// transport. main builds the Sender once via email.NewFromConfig, so
// dev (log-only) vs prod (provider-backed) is decided in one place
// instead of per dispatcher.
func NewEmailDispatcher(sender email.Sender, from string, logger *slog.Logger) *EmailDispatcher {
	return &EmailDispatcher{
		sender: sender,
		from:   from,
		logger: logger,
	}
}

// Send hands one pre-rendered email to the provider transport.
func (d *EmailDispatcher) Send(ctx context.Context, msg OutboundEmail) error {
	if msg.Recipient == "" {
		return fmt.Errorf("campaign: missing recipient")
	}
	if msg.Subject == "" {
		return fmt.Errorf("campaign: missing subject")
	}
	if msg.HTMLBody == "" && msg.TextBody == "" {
		return fmt.Errorf("campaign: missing body")
	}

	// Per-tenant attribution metadata — see notification-service Wave 1.5
	// webhook receiver. Echoed back on every engagement event so the
	// ingester can attribute opens / clicks / bounces to the right tenant
	// and campaign in tesserix-home dashboards.
	customArgs := map[string]string{"product": "mark8ly", "kind": "campaign"}
	if msg.TenantID != "" {
		customArgs["tenant_id"] = msg.TenantID
	}
	if msg.CampaignID != "" {
		customArgs["campaign_id"] = msg.CampaignID
	}

	out := email.Message{
		To:         msg.Recipient,
		Subject:    msg.Subject,
		HTMLBody:   msg.HTMLBody,
		TextBody:   msg.TextBody,
		CustomArgs: customArgs,
	}
	// Customer-facing marketing sent BY the store, so it carries the
	// store identity — never the platform's.
	email.StoreIdentity(d.from, msg.Sender).Apply(&out)

	return d.sender.Send(ctx, out)
}
