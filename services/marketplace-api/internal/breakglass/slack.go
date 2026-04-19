package breakglass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// SlackChannel is the hard-coded channel every break-glass alert lands
// in. §12.4 names #security-alerts as the on-call escalation channel;
// overriding is deliberately not configurable — a quiet break-glass
// alert is an operational nightmare.
const SlackChannel = "#security-alerts"

// SlackClient posts incoming webhook payloads to Slack's Web API /
// Incoming Webhooks endpoint. Production wires the webhook URL from
// GCP Secret Manager; tests pass an httptest.Server URL.
type SlackClient struct {
	webhookURL string
	channel    string
	http       *http.Client
}

// NewSlackClient returns a SlackClient with a 5-second request timeout.
// A short timeout is deliberate: the login handler MUST emit audit +
// rotate credentials even if Slack is down — we can't let a slow
// network call starve the security post-use hook.
func NewSlackClient(webhookURL, channel string) *SlackClient {
	return &SlackClient{
		webhookURL: webhookURL,
		channel:    channel,
		http:       &http.Client{Timeout: 5 * time.Second},
	}
}

// PostLoginAlert posts a login alert. `success=true` for logins that
// minted a session, `false` for rejected attempts. Slack is expected
// to render the text as-is; no block-kit here so the message survives
// Slack's transport regardless of workspace config.
func (s *SlackClient) PostLoginAlert(ctx context.Context, tenantID uuid.UUID, success bool) error {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}
	text := fmt.Sprintf(":rotating_light: break-glass login %s — tenant=%s", status, tenantID.String())
	return s.post(ctx, map[string]any{
		"channel":  s.channel,
		"username": "mark8ly-security",
		"text":     text,
	})
}

// PostRotationAlert posts a rotation notice. Success events are
// informational; failures are SEV-2 (ops paging territory).
func (s *SlackClient) PostRotationAlert(ctx context.Context, tenantID uuid.UUID, success bool, reason string) error {
	status := ":white_check_mark: rotated"
	if !success {
		status = ":x: rotation FAILED — " + reason
	}
	text := fmt.Sprintf("break-glass %s — tenant=%s", status, tenantID.String())
	return s.post(ctx, map[string]any{
		"channel":  s.channel,
		"username": "mark8ly-security",
		"text":     text,
	})
}

func (s *SlackClient) post(ctx context.Context, payload map[string]any) error {
	if s.webhookURL == "" {
		// No webhook configured (local dev) — silent no-op.
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("slack: webhook returned %d", res.StatusCode)
	}
	return nil
}
