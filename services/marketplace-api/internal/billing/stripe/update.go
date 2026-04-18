package stripe

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/stripe/stripe-go/v82"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ProrationBehavior mirrors Stripe's proration_behavior enum for subscription updates.
type ProrationBehavior string

const (
	ProrationAlwaysInvoice    ProrationBehavior = "always_invoice"
	ProrationCreateProrations ProrationBehavior = "create_prorations"
	ProrationNone             ProrationBehavior = "none"
)

// UpdateSubscriptionParams captures the inputs for a plan/period change.
// The current subscription item ID is resolved internally via GetSubscription.
type UpdateSubscriptionParams struct {
	SubscriptionID    string
	PriceID           string
	ProrationBehavior ProrationBehavior
	IdempotencyKey    string
	Metadata          map[string]string
}

// UpdateSubscription swaps the price on an existing Stripe Subscription.
// Expects the subscription to have exactly one item (our catalog invariant).
// It fetches the current subscription first to discover the subscription item ID
// that Stripe requires when updating items.
func UpdateSubscription(ctx context.Context, c *Client, in UpdateSubscriptionParams) (*Subscription, error) {
	current, err := GetSubscription(ctx, c, in.SubscriptionID)
	if err != nil {
		return nil, err
	}
	if len(current.Items.Data) == 0 {
		return nil, errors.New("stripe: subscription has no items, cannot update")
	}

	itemID := current.Items.Data[0].ID

	params := &sdk.SubscriptionUpdateParams{
		ProrationBehavior: sdk.String(string(in.ProrationBehavior)),
		Items: []*sdk.SubscriptionUpdateItemParams{
			{
				ID:    sdk.String(itemID),
				Price: sdk.String(in.PriceID),
			},
		},
	}
	for k, v := range in.Metadata {
		params.AddMetadata(k, v)
	}
	if in.IdempotencyKey != "" {
		params.SetIdempotencyKey(in.IdempotencyKey)
	}

	sdkSub, err := c.sdk.V1Subscriptions.Update(ctx, in.SubscriptionID, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapSubscription(sdkSub), nil
}

// PriceIDFor resolves (plan, period, currency, tier) to a Stripe Price ID via
// the catalog lookup key. For TierDeveloped, a single Price object covers all
// developed-market currencies via currency_options, so currency is not encoded
// in its lookup key. For TierPPP, each currency has its own Price object and
// the lookup key encodes the currency.
func PriceIDFor(
	ctx context.Context,
	c *Client,
	plan subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
	currency string,
	tier subscription.PriceTier,
) (string, error) {
	lookupKey, err := lookupKeyFor(plan, period, currency, tier)
	if err != nil {
		return "", err
	}
	p, err := FindPriceByLookupKey(ctx, c, lookupKey)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

// lookupKeyFor derives the Stripe price lookup key from subscription-layer types.
// It mirrors the catalog key format without importing the full descriptor (which
// would require a currency argument that MustGetDescriptor does not accept for PPP).
func lookupKeyFor(
	plan subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
	currency string,
	tier subscription.PriceTier,
) (string, error) {
	p := pricing.Plan(plan)
	per := pricing.Period(period)

	switch tier {
	case subscription.PriceTierDeveloped:
		// Validate (plan, period) exist in the catalog.
		pricing.MustGetDescriptor(p, per, pricing.TierDeveloped)
		return fmt.Sprintf("mark8ly_%s_%s_developed_v1", p, per), nil
	case subscription.PriceTierPPP:
		// Validate the (plan, period, currency) combination exists in the PPP catalog.
		if _, ok := pricing.LookupPPPOption(p, per, currency); !ok {
			return "", fmt.Errorf("stripe: no PPP price for plan=%s period=%s currency=%s", plan, period, currency)
		}
		return fmt.Sprintf("mark8ly_%s_%s_ppp_%s_v1", p, per, currency), nil
	default:
		return "", fmt.Errorf("stripe: unknown price tier %q", tier)
	}
}
