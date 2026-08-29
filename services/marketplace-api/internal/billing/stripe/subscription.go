package stripe

import (
	"context"
	"errors"

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
	// TrialEnd and BillingCycleAnchor are Unix seconds, 0 when Stripe
	// reports none. Projected explicitly rather than embedding the SDK
	// object: a passthrough leaks every field Stripe adds upstream.
	// BillingCycleAnchor is needed because SubscriptionUpdateParams.TrialEnd
	// is bounded at two years FROM THE ANCHOR, not from now.
	TrialEnd           int64 `json:"trial_end"`
	BillingCycleAnchor int64 `json:"billing_cycle_anchor"`
	// Metadata carries the keys CreateSubscription stamps on the object
	// (mark8ly_store_id / _plan / _period). Reconciliation uses
	// mark8ly_store_id to attribute a Stripe subscription back to a local
	// store when the local row never recorded its id.
	Metadata map[string]string `json:"metadata"`
	Items    struct {
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
		ID:                 s.ID,
		Status:             string(s.Status),
		Currency:           string(s.Currency),
		CancelAtPeriodEnd:  s.CancelAtPeriodEnd,
		Customer:           customerID,
		TrialEnd:           s.TrialEnd,
		BillingCycleAnchor: s.BillingCycleAnchor,
		Metadata:           s.Metadata,
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

// ListSubscriptionsByCustomer returns every subscription Stripe currently
// holds for customerID that is not canceled (Stripe's default list filter).
//
// Used by reconciliation to find subscriptions that exist at Stripe but that
// no local row points at — the orphan left behind when a subscription is
// created and the transaction that would have persisted its id then rolls
// back. Canceled subscriptions are deliberately excluded: they bill nothing,
// so they are not the divergence worth alerting on.
func ListSubscriptionsByCustomer(ctx context.Context, c *Client, customerID string) ([]*Subscription, error) {
	if customerID == "" {
		return nil, errors.New("stripe: ListSubscriptionsByCustomer: customer required")
	}

	params := &sdk.SubscriptionListParams{Customer: sdk.String(customerID)}
	params.Context = ctx
	params.Limit = sdk.Int64(100)

	var out []*Subscription
	for s, err := range c.sdk.V1Subscriptions.List(ctx, params) {
		if err != nil {
			return nil, toAPIError(err)
		}
		out = append(out, mapSubscription(s))
	}
	return out, nil
}
