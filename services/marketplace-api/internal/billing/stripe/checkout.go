package stripe

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

type CheckoutSession struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	IdempotencyKey string `json:"-"` // echoed back for logging
}

type CheckoutInput struct {
	StoreID    string
	TenantID   string
	CustomerID string
	PriceID    string
	Currency   string // lowercase ISO 4217
	Plan       string
	Period     string
	SuccessURL string
	CancelURL  string
	Locale     string
	Now        time.Time // injected for testability
}

func CreateCheckoutSession(ctx context.Context, c *Client, in CheckoutInput) (*CheckoutSession, error) {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	v := url.Values{}
	v.Set("mode", "subscription")
	v.Set("customer", in.CustomerID)
	v.Set("line_items[0][price]", in.PriceID)
	v.Set("line_items[0][quantity]", "1")
	v.Set("currency", strings.ToLower(in.Currency))
	v.Set("success_url", in.SuccessURL)
	v.Set("cancel_url", in.CancelURL)
	if in.Locale != "" {
		v.Set("locale", in.Locale)
	}
	v.Set("metadata[store_id]", in.StoreID)
	v.Set("metadata[tenant_id]", in.TenantID)
	v.Set("metadata[plan]", in.Plan)
	v.Set("metadata[period]", in.Period)
	v.Set("subscription_data[metadata][store_id]", in.StoreID)
	v.Set("subscription_data[metadata][tenant_id]", in.TenantID)

	key := CheckoutIdempotencyKey(in.StoreID, in.Plan, in.Period, in.Now.Unix())
	body, err := c.PostForm(ctx, "/v1/checkout/sessions", key, v)
	if err != nil {
		return nil, err
	}
	var sess CheckoutSession
	if err := json.Unmarshal(body, &sess); err != nil {
		return nil, err
	}
	sess.IdempotencyKey = key
	return &sess, nil
}
