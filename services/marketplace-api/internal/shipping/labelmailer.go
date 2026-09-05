// Package shipping — labelmailer.go: delivers the shipping-label PDF to a
// nominated email address via EmailLabelMailer, which rides the shared
// internal/email transport (SendGrid primary, Resend fallback; log-only
// sender in dev / CI so the admin flow can be exercised end-to-end
// without a provider account).
//
// It satisfies the LabelMailer interface in the admin handler package;
// we define the contract there to keep this package free of handler
// imports.
package shipping

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/email"
)

// LabelEmailPayload is the transport-agnostic payload handed to a mailer.
// Kept separate from the handler package's LabelEmailInput so both can
// evolve independently — the handler maps one to the other.
type LabelEmailPayload struct {
	Recipient string
	// TenantID, when non-empty, is forwarded to the email provider as a
	// custom_arg so notification-service can attribute open/click/bounce
	// events back to the right tenant in tesserix-home dashboards.
	TenantID  string
	StoreName string
	// StoreSlug and StoreContactEmail carry the store's sender identity
	// (#718). This mail goes to an address the MERCHANT nominates (a
	// fulfiller, a warehouse), not to a shopper — but it is still the
	// store's own mail, not the platform speaking to the merchant, so it
	// wears the store identity.
	StoreSlug         string
	StoreContactEmail string
	OrderNumber       string
	Carrier           string
	TrackingNumber    string
	PDF               []byte
	ContentType       string
	Filename          string
}

// EmailLabelMailer sends the label email + PDF attachment through the
// shared provider transport.
type EmailLabelMailer struct {
	sender email.Sender
	from   string
	logger *slog.Logger
}

// NewEmailLabelMailer constructs a label mailer on top of the shared
// transport.
func NewEmailLabelMailer(sender email.Sender, from string, logger *slog.Logger) *EmailLabelMailer {
	return &EmailLabelMailer{
		sender: sender,
		from:   from,
		logger: logger,
	}
}

// SendLabel renders a short plain-text + HTML envelope and attaches the
// PDF. Mirrors the orderdoc mailer's envelope shape (the shared
// transport disables click / open tracking so the recipient sees the
// attachment cleanly).
func (m *EmailLabelMailer) SendLabel(ctx context.Context, in LabelEmailPayload) error {
	if in.Recipient == "" {
		return fmt.Errorf("shipping: missing recipient")
	}
	if len(in.PDF) == 0 {
		return fmt.Errorf("shipping: empty PDF payload")
	}
	storeLabel := in.StoreName
	if storeLabel == "" {
		storeLabel = "Mark8ly"
	}
	subject := fmt.Sprintf("Shipping label for order %s", in.OrderNumber)
	textBody := fmt.Sprintf(
		"Hi,\n\nAttached is the shipping label for order %s.\nCarrier: %s\nTracking: %s\n\nSent from %s.",
		in.OrderNumber, in.Carrier, in.TrackingNumber, storeLabel)
	htmlBody := fmt.Sprintf(
		`<p>Hi,</p><p>Attached is the shipping label for order <strong>%s</strong>.</p>`+
			`<p>Carrier: <strong>%s</strong><br/>Tracking: <strong>%s</strong></p>`+
			`<p style="color:#7A766E;font-size:12px">Sent from %s.</p>`,
		in.OrderNumber, in.Carrier, in.TrackingNumber, storeLabel)

	ct := in.ContentType
	if ct == "" {
		ct = "application/pdf"
	}
	filename := in.Filename
	if filename == "" {
		filename = "shipping-label.pdf"
	}

	customArgs := map[string]string{"product": "mark8ly", "kind": "shipping_label"}
	if in.TenantID != "" {
		customArgs["tenant_id"] = in.TenantID
	}

	msg := email.Message{
		To:         in.Recipient,
		Subject:    subject,
		HTMLBody:   htmlBody,
		TextBody:   textBody,
		CustomArgs: customArgs,
		Attachments: []email.Attachment{{
			Filename:    filename,
			ContentType: ct,
			Content:     in.PDF,
		}},
	}
	email.StoreIdentity(m.from, email.StoreSender{
		Name:         in.StoreName,
		Slug:         in.StoreSlug,
		ContactEmail: in.StoreContactEmail,
	}).Apply(&msg)

	if err := m.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("shipping: send label email: %w", err)
	}
	return nil
}
