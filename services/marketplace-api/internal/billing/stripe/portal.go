package stripe

import (
	"context"
	"time"

	sdk "github.com/stripe/stripe-go/v82"
)

type PortalSession struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	IdempotencyKey string `json:"-"`
}

type PortalInput struct {
	StoreID    string
	CustomerID string
	ReturnURL  string
	Now        time.Time
}

func CreatePortalSession(ctx context.Context, c *Client, in PortalInput) (*PortalSession, error) {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	key := PortalIdempotencyKey(in.StoreID, in.Now.Unix())

	params := &sdk.BillingPortalSessionCreateParams{}
	params.Context = ctx
	params.IdempotencyKey = sdk.String(key)
	params.Customer = sdk.String(in.CustomerID)
	params.ReturnURL = sdk.String(in.ReturnURL)

	ps, err := c.sdk.V1BillingPortalSessions.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return &PortalSession{
		ID:             ps.ID,
		URL:            ps.URL,
		IdempotencyKey: key,
	}, nil
}
