package emailtemplates

// sendgrid_test_sender.go — minimum-viable SendGrid v3 dispatcher used
// by the /internal/templates/:key/test endpoint. Same wire shape as
// the per-package mailers (orderdoc.SendGridMailer, etc.) but without
// theming, attachments, or per-package shaping. Just renders a triple
// and POSTs it.
//
// Marked with custom_args { product: "mark8ly", kind: "template_test" }
// so any test-sends that flow through SendGrid show up clearly in
// engagement dashboards as test traffic, not real customer mail.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendGridTestSender implements TestSender by POSTing to SendGrid v3.
type SendGridTestSender struct {
	apiKey string
	from   string
	client *http.Client
}

// NewSendGridTestSender constructs a SendGridTestSender. apiKey may be
// empty in dev — SendTest then errors with a clear message rather
// than silently dropping the test.
func NewSendGridTestSender(apiKey, from string) *SendGridTestSender {
	return &SendGridTestSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// SendTest dispatches a rendered triple to the given recipient.
func (s *SendGridTestSender) SendTest(ctx context.Context, to string, r Rendered) error {
	if s.apiKey == "" {
		return fmt.Errorf("emailtemplates: SendGrid API key not configured (cannot test-send)")
	}
	if to == "" {
		return fmt.Errorf("emailtemplates: missing recipient")
	}
	if r.Subject == "" || (r.HTMLBody == "" && r.TextBody == "") {
		return fmt.Errorf("emailtemplates: rendered triple is incomplete")
	}

	falsePtr := false
	payload := sgTestRequest{
		Personalizations: []sgTestPersonalization{{To: []sgTestAddress{{Email: to}}}},
		From:             sgTestAddress{Email: s.from},
		Subject:          r.Subject,
		Content: []sgTestContent{
			{Type: "text/plain", Value: r.TextBody},
			{Type: "text/html", Value: r.HTMLBody},
		},
		CustomArgs: map[string]string{
			"product": "mark8ly",
			"kind":    "template_test",
		},
		TrackingSettings: &sgTestTracking{
			ClickTracking:        &sgTestEnable{Enable: &falsePtr, EnableText: &falsePtr},
			OpenTracking:         &sgTestEnable{Enable: &falsePtr},
			SubscriptionTracking: &sgTestEnable{Enable: &falsePtr},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("emailtemplates: marshal sendgrid request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("emailtemplates: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("emailtemplates: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("emailtemplates: sendgrid returned %d: %s", resp.StatusCode, string(body))
}

// SendGrid v3 wire shapes — local subset to avoid pulling in another
// package; these exactly mirror the shapes used by the per-package
// mailers in orderdoc / giftcard / campaign.
type sgTestRequest struct {
	Personalizations []sgTestPersonalization `json:"personalizations"`
	From             sgTestAddress           `json:"from"`
	Subject          string                  `json:"subject"`
	Content          []sgTestContent         `json:"content"`
	CustomArgs       map[string]string       `json:"custom_args,omitempty"`
	TrackingSettings *sgTestTracking         `json:"tracking_settings,omitempty"`
}

type sgTestPersonalization struct {
	To []sgTestAddress `json:"to"`
}

type sgTestAddress struct {
	Email string `json:"email"`
}

type sgTestContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgTestTracking struct {
	ClickTracking        *sgTestEnable `json:"click_tracking,omitempty"`
	OpenTracking         *sgTestEnable `json:"open_tracking,omitempty"`
	SubscriptionTracking *sgTestEnable `json:"subscription_tracking,omitempty"`
}

type sgTestEnable struct {
	Enable     *bool `json:"enable,omitempty"`
	EnableText *bool `json:"enable_text,omitempty"`
}
