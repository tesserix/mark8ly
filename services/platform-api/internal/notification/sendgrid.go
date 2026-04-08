package notification

import (
	"bytes"
	"context"
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

// Send dispatches a single Email via SendGrid.
func (s *SendGridSender) Send(ctx context.Context, msg Email) error {
	if err := validate(msg); err != nil {
		return err
	}
	if s.apiKey == "" {
		return fmt.Errorf("notification: SendGrid API key is not configured")
	}

	// Disable click + open tracking per-send. Transactional magic-link
	// emails carry single-use tokens; the SendGrid click tracker issues
	// a GET to record the click, which consumes the token BEFORE the
	// user can press the button. Disabling it account-wide is the
	// canonical fix, but belt-and-suspenders the setting here so a
	// future sender can't accidentally re-enable it globally.
	falsePtr := false
	body := sendgridRequest{
		Personalizations: []sendgridPersonalization{{
			To: []sendgridAddress{{Email: msg.To}},
		}},
		From:    sendgridAddress{Email: msg.From},
		Subject: msg.Subject,
		Content: []sendgridContent{
			{Type: "text/plain", Value: msg.TextBody},
			{Type: "text/html", Value: msg.HTMLBody},
		},
		TrackingSettings: &sendgridTrackingSettings{
			ClickTracking: &sendgridClickTracking{
				Enable:     &falsePtr,
				EnableText: &falsePtr,
			},
			OpenTracking: &sendgridOpenTracking{
				Enable: &falsePtr,
			},
			SubscriptionTracking: &sendgridSubscriptionTracking{
				Enable: &falsePtr,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("notification: marshal sendgrid request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("notification: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("notification: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("notification: sendgrid returned %d: %s", resp.StatusCode, string(respBody))
}

// SendGrid v3 API request shapes — minimum viable subset.
type sendgridRequest struct {
	Personalizations []sendgridPersonalization `json:"personalizations"`
	From             sendgridAddress           `json:"from"`
	Subject          string                    `json:"subject"`
	Content          []sendgridContent         `json:"content"`
	TrackingSettings *sendgridTrackingSettings `json:"tracking_settings,omitempty"`
}

type sendgridTrackingSettings struct {
	ClickTracking        *sendgridClickTracking        `json:"click_tracking,omitempty"`
	OpenTracking         *sendgridOpenTracking         `json:"open_tracking,omitempty"`
	SubscriptionTracking *sendgridSubscriptionTracking `json:"subscription_tracking,omitempty"`
}

type sendgridClickTracking struct {
	Enable     *bool `json:"enable,omitempty"`
	EnableText *bool `json:"enable_text,omitempty"`
}

type sendgridOpenTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sendgridSubscriptionTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sendgridPersonalization struct {
	To []sendgridAddress `json:"to"`
}

type sendgridAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sendgridContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
