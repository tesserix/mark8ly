package stripe

import (
	"context"
	"encoding/json"
	"net/url"
)

type SubscriptionItem struct {
	Price Price `json:"price"`
}

type Subscription struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	Currency           string `json:"currency"`
	CurrentPeriodStart int64  `json:"current_period_start"`
	CurrentPeriodEnd   int64  `json:"current_period_end"`
	CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
	Customer           string `json:"customer"`
	Items              struct {
		Data []SubscriptionItem `json:"data"`
	} `json:"items"`
}

// GetSubscription fetches by Stripe subscription ID.
func GetSubscription(ctx context.Context, c *Client, id string) (*Subscription, error) {
	body, err := c.Get(ctx, "/v1/subscriptions/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var s Subscription
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
