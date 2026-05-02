package campaign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// SendGridDispatcher sends pre-rendered campaign emails via the SendGrid
// v3 HTTP API. Templating + theming happens in the send worker (so the
// store branding is loaded once per campaign, not once per recipient) —
// this dispatcher is a thin transport adapter.
//
// Thin HTTP client instead of the official SDK — same rationale as
// platform-api/internal/notification/sendgrid.go: a single POST is not
// worth the transitive dependency weight.
type SendGridDispatcher struct {
	apiKey string
	from   string
	client *http.Client
	logger *slog.Logger
}

// NewSendGridDispatcher constructs a SendGrid-backed Dispatcher. If
// apiKey is empty, Send errors on every call — callers should fall back
// to LogDispatcher when the key is missing.
func NewSendGridDispatcher(apiKey, from string, logger *slog.Logger) *SendGridDispatcher {
	return &SendGridDispatcher{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
		logger: logger,
	}
}

// Send POSTs one pre-rendered email to SendGrid v3.
func (d *SendGridDispatcher) Send(ctx context.Context, msg OutboundEmail) error {
	if d.apiKey == "" {
		return fmt.Errorf("campaign: SendGrid API key not configured")
	}
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

	// Disable tracking — keeps behavior identical to platform-api's
	// notification sender so merchants don't see inconsistent click
	// tracking across transactional and campaign mail.
	falsePtr := false
	payload := sgRequest{
		Personalizations: []sgPersonalization{{To: []sgAddress{{Email: msg.Recipient}}}},
		From:             sgAddress{Email: d.from},
		Subject:          msg.Subject,
		Content: []sgContent{
			{Type: "text/plain", Value: msg.TextBody},
			{Type: "text/html", Value: msg.HTMLBody},
		},
		CustomArgs: customArgs,
		TrackingSettings: &sgTrackingSettings{
			ClickTracking:        &sgClickTracking{Enable: &falsePtr, EnableText: &falsePtr},
			OpenTracking:         &sgOpenTracking{Enable: &falsePtr},
			SubscriptionTracking: &sgSubscriptionTracking{Enable: &falsePtr},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("campaign: marshal sendgrid request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("campaign: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("campaign: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("campaign: sendgrid returned %d: %s", resp.StatusCode, string(respBody))
}

// --- SendGrid v3 API wire types (minimum viable subset) ---

type sgRequest struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	CustomArgs       map[string]string   `json:"custom_args,omitempty"`
	TrackingSettings *sgTrackingSettings `json:"tracking_settings,omitempty"`
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
