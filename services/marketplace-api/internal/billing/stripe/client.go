package stripe

import (
	"fmt"
	"time"

	sdk "github.com/stripe/stripe-go/v82"
)

// Client wraps the Stripe SDK client with idempotency helpers and test-URL override.
// No request or response bodies are ever logged; callers must use SanitizeForLog.
type Client struct {
	apiKey string
	sdk    *sdk.Client
	now    func() time.Time
}

// New constructs a Client using the official Stripe SDK backend.
func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		sdk:    sdk.NewClient(apiKey),
		now:    time.Now,
	}
}

// SetBaseURLForTesting redirects all SDK HTTP calls to u (e.g. an httptest.Server URL).
// Both the API and Uploads backends are redirected so that every SDK call hits the stub.
func (c *Client) SetBaseURLForTesting(u string) {
	cfg := &sdk.BackendConfig{URL: sdk.String(u)}
	backends := &sdk.Backends{
		API:         sdk.GetBackendWithConfig(sdk.APIBackend, cfg),
		Connect:     sdk.GetBackendWithConfig(sdk.ConnectBackend, cfg),
		Uploads:     sdk.GetBackendWithConfig(sdk.UploadsBackend, cfg),
		MeterEvents: sdk.GetBackendWithConfig(sdk.MeterEventsBackend, cfg),
	}
	c.sdk = sdk.NewClient(c.apiKey, sdk.WithBackends(backends))
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
