package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendGridSender sends emails via the SendGrid v3 HTTP API.
//
// We use a thin HTTP client instead of the official SendGrid SDK to avoid a
// large dependency for what is fundamentally a single POST request. The
// official SDK pulls in ~30 transitive packages for ~5 lines of business logic.
type SendGridSender struct {
	apiKey string
	client *http.Client
}

// NewSendGridSender constructs a SendGridSender. apiKey must be set; an
// empty key fails-fast on Send rather than silently dropping mail.
func NewSendGridSender(apiKey string) *SendGridSender {
	return &SendGridSender{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Send dispatches a single Message via SendGrid.
func (s *SendGridSender) Send(ctx context.Context, msg Message) error {
	if err := validate(msg); err != nil {
		return err
	}
	if s.apiKey == "" {
		return fmt.Errorf("email: SendGrid API key is not configured")
	}

	raw, err := json.Marshal(buildSendGridRequest(msg))
	if err != nil {
		return fmt.Errorf("email: marshal sendgrid request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("email: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("email: sendgrid returned %d: %s", resp.StatusCode, string(respBody))
}

// buildSendGridRequest renders a Message into SendGrid's wire shape.
// Split out of Send so the envelope — sender identity, reply-to and the
// custom_args attribution the notification-service webhook receiver
// joins on — can be asserted without an HTTP round trip.
func buildSendGridRequest(msg Message) sgRequest {
	// SendGrid rejects content entries with empty values and requires
	// text/plain to precede text/html, so only the parts a mailer
	// actually rendered are included (the ticket notifier is HTML-only).
	var content []sgContent
	if msg.TextBody != "" {
		content = append(content, sgContent{Type: "text/plain", Value: msg.TextBody})
	}
	if msg.HTMLBody != "" {
		content = append(content, sgContent{Type: "text/html", Value: msg.HTMLBody})
	}

	// Attachment content MUST be base64-encoded; SendGrid rejects raw
	// bytes with a 400.
	var attachments []sgAttachment
	for _, a := range msg.Attachments {
		attachments = append(attachments, sgAttachment{
			Content:     base64.StdEncoding.EncodeToString(a.Content),
			Filename:    a.Filename,
			Type:        a.ContentType,
			Disposition: "attachment",
		})
	}

	// Disable click + open tracking per-send. Transactional magic-link
	// emails carry single-use tokens; the SendGrid click tracker issues
	// a GET to record the click, which consumes the token BEFORE the
	// user can press the button. Disabling it account-wide is the
	// canonical fix, but belt-and-suspenders the setting here so a
	// future sender can't accidentally re-enable it globally.
	falsePtr := false
	body := sgRequest{
		Personalizations: []sgPersonalization{{
			To: []sgAddress{{Email: msg.To, Name: msg.ToName}},
		}},
		From: sgAddress{Email: msg.From, Name: msg.FromName},
		// Reply-To is a pointer so an unset one is omitted entirely;
		// SendGrid rejects `"reply_to": {}` with a 400 rather than
		// ignoring it.
		ReplyTo:     replyToAddress(msg),
		Subject:     msg.Subject,
		Content:     content,
		Attachments: attachments,
		// Per-tenant attribution metadata — see notification-service
		// Wave 1.5 webhook receiver. SendGrid echoes custom_args back on
		// every engagement event (delivered/open/click/bounce) so we can
		// join them to a tenant without parsing recipient strings.
		CustomArgs: msg.CustomArgs,
		TrackingSettings: &sgTrackingSettings{
			ClickTracking: &sgClickTracking{
				Enable:     &falsePtr,
				EnableText: &falsePtr,
			},
			OpenTracking: &sgOpenTracking{
				Enable: &falsePtr,
			},
			SubscriptionTracking: &sgSubscriptionTracking{
				Enable: &falsePtr,
			},
		},
	}
	return body
}

// replyToAddress renders Message.ReplyTo as SendGrid's reply_to object,
// or nil when unset. The display name is deliberately NOT repeated here:
// the merchant's contact address is their own mailbox, and labelling it
// with the store name would let a store name reach a second header.
func replyToAddress(msg Message) *sgAddress {
	if msg.ReplyTo == "" {
		return nil
	}
	return &sgAddress{Email: msg.ReplyTo}
}

// SendGrid v3 API request shapes — minimum viable subset.
type sgRequest struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	ReplyTo          *sgAddress          `json:"reply_to,omitempty"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	Attachments      []sgAttachment      `json:"attachments,omitempty"`
	CustomArgs       map[string]string   `json:"custom_args,omitempty"`
	TrackingSettings *sgTrackingSettings `json:"tracking_settings,omitempty"`
}

// sgAttachment is SendGrid's attachment wire shape. Content MUST be
// base64-encoded; SendGrid rejects raw bytes with a 400.
type sgAttachment struct {
	Content     string `json:"content"`
	Filename    string `json:"filename"`
	Type        string `json:"type,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

type sgTrackingSettings struct {
	ClickTracking        *sgClickTracking        `json:"click_tracking,omitempty"`
	OpenTracking         *sgOpenTracking         `json:"open_tracking,omitempty"`
	SubscriptionTracking *sgSubscriptionTracking `json:"subscription_tracking,omitempty"`
}

type sgClickTracking struct {
	Enable     *bool `json:"enable,omitempty"`
	EnableText *bool `json:"enable_text,omitempty"`
}

type sgOpenTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sgSubscriptionTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Name identifies the provider in fallback-chain logs.
func (s *SendGridSender) Name() string { return ProviderSendGrid }
