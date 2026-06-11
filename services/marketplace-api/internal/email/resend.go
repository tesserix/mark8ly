package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// Send dispatches a single Message via Resend.
func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	if err := validate(msg); err != nil {
		return err
	}
	if s.apiKey == "" {
		return fmt.Errorf("email: Resend API key is not configured")
	}

	// Resend has no per-send tracking toggle (click/open tracking is off
	// unless enabled on the domain), so magic-link tokens are safe by
	// default. Attribution mirrors the SendGrid custom_args via tags —
	// Resend echoes tags back on webhook events. Tag names/values only
	// allow ASCII letters, numbers, underscores and dashes; our keys
	// (product/kind/tenant_id/campaign_id) and UUID values all pass.
	// Tags that don't are skipped rather than failing the send — losing
	// one attribution dimension beats losing the email. Keys are sorted
	// so the payload is deterministic.
	var tags []resendTag
	keys := make([]string, 0, len(msg.CustomArgs))
	for k := range msg.CustomArgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := msg.CustomArgs[k]
		if !resendTagSafe(k) || !resendTagSafe(v) {
			continue
		}
		tags = append(tags, resendTag{Name: k, Value: v})
	}

	// Resend takes the display name inline RFC-5322 style rather than as
	// a separate field.
	from := msg.From
	if msg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", msg.FromName, msg.From)
	}

	var attachments []resendAttachment
	for _, a := range msg.Attachments {
		attachments = append(attachments, resendAttachment{
			Content:     base64.StdEncoding.EncodeToString(a.Content),
			Filename:    a.Filename,
			ContentType: a.ContentType,
		})
	}

	body := resendRequest{
		From:        from,
		To:          []string{msg.To},
		Subject:     msg.Subject,
		HTML:        msg.HTMLBody,
		Text:        msg.TextBody,
		Tags:        tags,
		Attachments: attachments,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("email: marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.resend.com/emails", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("email: build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email: resend POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("email: resend returned %d: %s", resp.StatusCode, string(respBody))
}

// resendTagSafe reports whether s is a valid Resend tag name or value:
// non-empty ASCII letters, numbers, underscores and dashes only.
func resendTagSafe(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// Resend API request shape — minimum viable subset.
type resendRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html,omitempty"`
	Text        string             `json:"text,omitempty"`
	Tags        []resendTag        `json:"tags,omitempty"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

type resendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// resendAttachment mirrors Resend's attachment shape; content is a
// base64 string (Resend also accepts byte arrays, but base64 keeps the
// payload shape identical to SendGrid's).
type resendAttachment struct {
	Content     string `json:"content"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
}
