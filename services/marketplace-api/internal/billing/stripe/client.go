package stripe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultAPIBase = "https://api.stripe.com"

// Client wraps the Stripe REST API with idempotency, sanitized errors, and a fixed timeout.
// No body is ever logged.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	now     func() time.Time
}

// New constructs a Client with the default 30s timeout and production API base.
func New(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultAPIBase,
		http:    &http.Client{Timeout: 30 * time.Second},
		now:     time.Now,
	}
}

// SetBaseURLForTesting overrides the Stripe API base for tests (httptest.Server URL).
func (c *Client) SetBaseURLForTesting(u string) { c.baseURL = u }

// PostForm executes a POST with x-www-form-urlencoded body. idempotencyKey MUST be non-empty;
// callers generate it deterministically per §17.8.
func (c *Client) PostForm(ctx context.Context, path, idempotencyKey string, values url.Values) ([]byte, error) {
	if idempotencyKey == "" {
		return nil, errors.New("stripe: idempotency key required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("Stripe-Version", "2026-01-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, ParseAPIError(resp.StatusCode, string(body), resp.Header.Get("Request-Id"))
	}
	return body, nil
}

// Get performs a read-only GET.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Stripe-Version", "2026-01-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, ParseAPIError(resp.StatusCode, string(body), resp.Header.Get("Request-Id"))
	}
	return body, nil
}

// --- Idempotency-Key generators (§17.8) -------------------------------------

func CustomerIdempotencyKey(storeID string) string {
	return "customer:" + storeID
}

func CheckoutIdempotencyKey(storeID, plan, period string, unixTs int64) string {
	dayBucket := unixTs / 86400
	return fmt.Sprintf("checkout:%s:%s:%s:%d", storeID, plan, period, dayBucket)
}

func SubscriptionIdempotencyKey(storeID, plan, period string) string {
	return fmt.Sprintf("subscription:%s:%s:%s", storeID, plan, period)
}

// PortalIdempotencyKey: §17.8 Council finding #1 — 5-min bucket matches the
// Stripe portal URL lifetime. Larger buckets returned expired URLs.
func PortalIdempotencyKey(storeID string, unixTs int64) string {
	return fmt.Sprintf("portal:%s:%d", storeID, unixTs/300)
}

func RefundIdempotencyKey(invoiceID string) string {
	return "refund:" + invoiceID
}
