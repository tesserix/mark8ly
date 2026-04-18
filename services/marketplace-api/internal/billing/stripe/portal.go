package stripe

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
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
	v := url.Values{}
	v.Set("customer", in.CustomerID)
	v.Set("return_url", in.ReturnURL)

	key := PortalIdempotencyKey(in.StoreID, in.Now.Unix())
	body, err := c.PostForm(ctx, "/v1/billing_portal/sessions", key, v)
	if err != nil {
		return nil, err
	}
	var ps PortalSession
	if err := json.Unmarshal(body, &ps); err != nil {
		return nil, err
	}
	ps.IdempotencyKey = key
	return &ps, nil
}
