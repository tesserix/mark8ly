// Package shipping — labelmailer.go: delivers the shipping-label PDF to a
// nominated email address. Two implementations:
//
//   - SendGridLabelMailer: production, attaches the PDF and posts to
//     SendGrid v3 /mail/send.
//   - LogLabelMailer: dev / CI fallback that records the intended send
//     without actually emailing anything.
//
// Both satisfy the LabelMailer interface in the admin handler package;
// we define the contract there to keep this package free of handler
// imports.
package shipping

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// LabelEmailPayload is the transport-agnostic payload handed to a mailer.
// Kept separate from the handler package's LabelEmailInput so both can
// evolve independently — the handler maps one to the other.
type LabelEmailPayload struct {
	Recipient      string
	StoreName      string
	OrderNumber    string
	Carrier        string
	TrackingNumber string
	PDF            []byte
	ContentType    string
	Filename       string
}

// LogLabelMailer records the intended email instead of sending it.
// Used when SENDGRID_API_KEY is unset (local dev, integration tests)
// so the admin flow can be exercised end-to-end without SMTP access.
type LogLabelMailer struct {
	Logger *slog.Logger
}

// SendLabel records the would-be dispatch on the package logger.
func (m *LogLabelMailer) SendLabel(_ context.Context, in LabelEmailPayload) error {
	if m.Logger != nil {
		m.Logger.Info("shipping: label email (log-only)",
			"recipient", in.Recipient,
			"order_number", in.OrderNumber,
			"carrier", in.Carrier,
			"tracking_number", in.TrackingNumber,
			"pdf_bytes", len(in.PDF))
	}
	return nil
}

// SendGridLabelMailer posts the label email + PDF attachment to SendGrid.
type SendGridLabelMailer struct {
	apiKey string
	from   string
	client *http.Client
	logger *slog.Logger
}

// NewSendGridLabelMailer constructs a SendGrid-backed label mailer.
func NewSendGridLabelMailer(apiKey, from string, logger *slog.Logger) *SendGridLabelMailer {
	return &SendGridLabelMailer{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
		logger: logger,
	}
}

// SendLabel renders a short plain-text + HTML envelope and attaches the
// PDF. Mirrors the orderdoc mailer's request shape (no click / open
// tracking so the recipient sees the attachment cleanly).
func (m *SendGridLabelMailer) SendLabel(ctx context.Context, in LabelEmailPayload) error {
	if m.apiKey == "" {
		return fmt.Errorf("shipping: SendGrid API key not configured")
	}
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

	falsePtr := false
	payload := sgLabelRequest{
		Personalizations: []sgLabelPersonalization{{To: []sgLabelAddress{{Email: in.Recipient}}}},
		From:             sgLabelAddress{Email: m.from},
		Subject:          subject,
		Content: []sgLabelContent{
			{Type: "text/plain", Value: textBody},
			{Type: "text/html", Value: htmlBody},
		},
		Attachments: []sgLabelAttachment{{
			Content:     base64.StdEncoding.EncodeToString(in.PDF),
			Type:        ct,
			Filename:    filename,
			Disposition: "attachment",
		}},
		TrackingSettings: &sgLabelTrackingSettings{
			ClickTracking:        &sgLabelTrackingFlag{Enable: &falsePtr, EnableText: &falsePtr},
			OpenTracking:         &sgLabelTrackingFlag{Enable: &falsePtr},
			SubscriptionTracking: &sgLabelTrackingFlag{Enable: &falsePtr},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("shipping: marshal sendgrid request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("shipping: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("shipping: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("shipping: sendgrid returned %d: %s", resp.StatusCode, string(body))
}

// -- SendGrid wire types (subset reused from orderdoc mailer, renamed
// to sidestep a cross-package type collision while keeping this file
// self-contained). --

type sgLabelRequest struct {
	Personalizations []sgLabelPersonalization `json:"personalizations"`
	From             sgLabelAddress           `json:"from"`
	Subject          string                   `json:"subject"`
	Content          []sgLabelContent         `json:"content"`
	Attachments      []sgLabelAttachment      `json:"attachments,omitempty"`
	TrackingSettings *sgLabelTrackingSettings `json:"tracking_settings,omitempty"`
}

type sgLabelPersonalization struct {
	To []sgLabelAddress `json:"to"`
}

type sgLabelAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgLabelContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgLabelAttachment struct {
	Content     string `json:"content"` // base64
	Type        string `json:"type"`
	Filename    string `json:"filename"`
	Disposition string `json:"disposition"`
}

type sgLabelTrackingSettings struct {
	ClickTracking        *sgLabelTrackingFlag `json:"click_tracking,omitempty"`
	OpenTracking         *sgLabelTrackingFlag `json:"open_tracking,omitempty"`
	SubscriptionTracking *sgLabelTrackingFlag `json:"subscription_tracking,omitempty"`
}

type sgLabelTrackingFlag struct {
	Enable     *bool `json:"enable,omitempty"`
	EnableText *bool `json:"enable_text,omitempty"`
}
