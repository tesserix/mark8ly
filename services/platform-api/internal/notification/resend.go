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

// ResendSender sends emails via the Resend HTTP API (https://resend.com).
//
// Same thin-HTTP-client philosophy as SendGridSender: the whole integration
// is one POST request, so the official SDK is not worth the dependency.
// Resend is wired as the FALLBACK provider behind SendGrid — see
// FallbackSender — so transactional mail keeps flowing when SendGrid is
// down, rate-limiting, or rejecting the account.
type ResendSender struct {
	apiKey string
	client *http.Client
}

// NewResendSender constructs a ResendSender. apiKey must be set; an empty
// key fails-fast on Send rather than silently dropping mail.
func NewResendSender(apiKey string) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Send dispatches a single Email via Resend.
func (s *ResendSender) Send(ctx context.Context, msg Email) error {
	if err := validate(msg); err != nil {
		return err
	}
	if s.apiKey == "" {
		return fmt.Errorf("notification: Resend API key is not configured")
	}

	// Resend has no per-send tracking toggle (click/open tracking is off
	// unless enabled on the domain), so magic-link tokens are safe by
	// default. Attribution mirrors the SendGrid custom_args via tags —
	// Resend echoes tags back on webhook events. Tag names/values only
	// allow ASCII letters, numbers, underscores and dashes; tenant IDs
	// are UUIDs, so they pass as-is.
	tags := []resendTag{{Name: "product", Value: "mark8ly"}}
	if msg.TenantID != "" {
		tags = append(tags, resendTag{Name: "tenant_id", Value: msg.TenantID})
	}

	body := resendRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTMLBody,
		Text:    msg.TextBody,
		Tags:    tags,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("notification: marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.resend.com/emails", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("notification: build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("notification: resend POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("notification: resend returned %d: %s", resp.StatusCode, string(respBody))
}

// Resend API request shape — minimum viable subset.
type resendRequest struct {
	From    string      `json:"from"`
	To      []string    `json:"to"`
	Subject string      `json:"subject"`
	HTML    string      `json:"html,omitempty"`
	Text    string      `json:"text,omitempty"`
	Tags    []resendTag `json:"tags,omitempty"`
}

type resendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
