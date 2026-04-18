package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPPagerDuty posts incident events to a configured webhook URL. When URL
// is empty the Trigger call becomes a no-op — useful in dev/test.
type HTTPPagerDuty struct {
	URL    string
	Client *http.Client
}

// Trigger sends a PagerDuty event with the given summary. A 4xx/5xx response
// is returned as an error. If URL is empty, Trigger is a no-op.
func (p *HTTPPagerDuty) Trigger(ctx context.Context, summary string) error {
	if p.URL == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"summary":  summary,
		"severity": "error",
		"source":   "marketplace-api",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("pagerduty: status %d", resp.StatusCode)
	}
	return nil
}
