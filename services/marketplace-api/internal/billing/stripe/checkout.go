package stripe

import (
	"context"
	"strings"
	"time"

	sdk "github.com/stripe/stripe-go/v82"
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
	key := CheckoutIdempotencyKey(in.StoreID, in.Plan, in.Period, in.Now.Unix())

	params := &sdk.CheckoutSessionCreateParams{}
	params.Context = ctx
	params.IdempotencyKey = sdk.String(key)
	params.Mode = sdk.String("subscription")
	params.Customer = sdk.String(in.CustomerID)
	params.Currency = sdk.String(strings.ToLower(in.Currency))
	params.SuccessURL = sdk.String(in.SuccessURL)
	params.CancelURL = sdk.String(in.CancelURL)
	if in.Locale != "" {
		params.Locale = sdk.String(in.Locale)
	}
	params.LineItems = []*sdk.CheckoutSessionCreateLineItemParams{
		{Price: sdk.String(in.PriceID), Quantity: sdk.Int64(1)},
	}
	params.Metadata = map[string]string{
		"store_id":  in.StoreID,
		"tenant_id": in.TenantID,
		"plan":      in.Plan,
		"period":    in.Period,
	}
	params.SubscriptionData = &sdk.CheckoutSessionCreateSubscriptionDataParams{
		Metadata: map[string]string{
			"store_id":  in.StoreID,
			"tenant_id": in.TenantID,
		},
	}

	sess, err := c.sdk.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return &CheckoutSession{
		ID:             sess.ID,
		URL:            sess.URL,
		IdempotencyKey: key,
	}, nil
}
