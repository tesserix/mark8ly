package stripe

import (
	"context"

	sdk "github.com/stripe/stripe-go/v82"
)

type SubscriptionItem struct {
	ID    string `json:"id"`
	Price Price  `json:"price"`
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
	s, err := c.sdk.V1Subscriptions.Retrieve(ctx, id, nil)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapSubscription(s), nil
}

// mapSubscription maps the SDK Subscription to our public Subscription struct.
// CurrentPeriodStart/End moved from the top-level Subscription to each
// SubscriptionItem in SDK v82; we derive them from the first item when present.
func mapSubscription(s *sdk.Subscription) *Subscription {
	var customerID string
	if s.Customer != nil {
		customerID = s.Customer.ID
	}

	out := &Subscription{
		ID:                s.ID,
		Status:            string(s.Status),
		Currency:          string(s.Currency),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd,
		Customer:          customerID,
	}

	if s.Items != nil {
		for _, item := range s.Items.Data {
			if item == nil {
				continue
			}
			// CurrentPeriodStart/End are on SubscriptionItem in SDK v82,
			// not on the parent Subscription. Take the first item's values.
			if out.CurrentPeriodStart == 0 {
				out.CurrentPeriodStart = item.CurrentPeriodStart
				out.CurrentPeriodEnd = item.CurrentPeriodEnd
			}
			si := SubscriptionItem{ID: item.ID}
			if item.Price != nil {
				si.Price = *mapPrice(item.Price)
			}
			out.Items.Data = append(out.Items.Data, si)
		}
	}

	return out
}
