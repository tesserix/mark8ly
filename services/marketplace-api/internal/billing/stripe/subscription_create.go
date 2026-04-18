package stripe

import (
	"context"
	"fmt"

	sdk "github.com/stripe/stripe-go/v82"
)

// CreateSubscriptionInput captures the inputs for a deferred-charge trial
// subscription. The caller fixes TrialEnd to signup_date + 90d so Stripe
// defers the first invoice to that moment — nothing is charged now.
type CreateSubscriptionInput struct {
	StoreID    string // used in the stable idempotency key
	Plan       string // "starter"|"studio"|"pro"
	Period     string // "monthly"|"annual"
	CustomerID string
	PriceID    string
	TrialEnd   int64 // Unix seconds — required; always signup_date + 90d for trial card-add
}

// CreateSubscription creates a Stripe Subscription with trial_end,
// proration_behavior=none, and a stable idempotency key. Stripe returns a
// "trialing" subscription whose first invoice fires at trial_end. No charge
// at call time.
//
// Callers persist the returned Subscription.ID onto
// store_subscriptions.stripe_subscription_id. The state transition
// (signup|trialing → active) happens asynchronously via the
// customer.subscription.created / invoice.paid webhook routed through
// statemachine.Transition.
func CreateSubscription(ctx context.Context, c *Client, in CreateSubscriptionInput) (*Subscription, error) {
	if in.CustomerID == "" || in.PriceID == "" {
		return nil, fmt.Errorf("stripe: CreateSubscription: customer + price required")
	}
	if in.TrialEnd <= 0 {
		return nil, fmt.Errorf("stripe: CreateSubscription: trial_end required (signup_date + 90d)")
	}

	params := &sdk.SubscriptionCreateParams{
		Customer: sdk.String(in.CustomerID),
		Items: []*sdk.SubscriptionCreateItemParams{
			{Price: sdk.String(in.PriceID)},
		},
		ProrationBehavior: sdk.String("none"),
		TrialEnd:          sdk.Int64(in.TrialEnd),
	}
	params.AddMetadata("mark8ly_store_id", in.StoreID)
	params.AddMetadata("mark8ly_plan", in.Plan)
	params.AddMetadata("mark8ly_period", in.Period)

	key := SubscriptionIdempotencyKey(in.StoreID, in.Plan, in.Period)
	params.SetIdempotencyKey(key)

	sdkSub, err := c.sdk.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapSubscription(sdkSub), nil
}
