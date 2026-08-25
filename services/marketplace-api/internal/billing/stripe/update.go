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

// UpdateSubscriptionParams captures the inputs for a subscription update.
// The current subscription item ID is resolved internally via GetSubscription
// when — and only when — a PriceID is supplied.
type UpdateSubscriptionParams struct {
	SubscriptionID    string
	PriceID           string // optional: when empty, the price is left alone
	TrialEnd          *int64 // optional: Unix seconds; moves billing_cycle_anchor
	ProrationBehavior ProrationBehavior
	IdempotencyKey    string
	Metadata          map[string]string
}

// UpdateSubscription updates an existing Stripe Subscription.
//
// Items are sent ONLY when PriceID is set. This matters: before #358 the
// price swap was unconditional, so calling this to move a trial end would
// have re-priced the subscription silently. The two plan-change callers
// (subscription/planchange) always set PriceID and are unaffected.
//
// Setting TrialEnd also moves Stripe's billing_cycle_anchor to that value —
// Stripe's own documented behaviour, not ours — so it changes when the
// merchant is billed thereafter. Stripe bounds it at two years from the
// current anchor. TrialFromPlan is never set here; Stripe rejects it
// alongside trial_end.
func UpdateSubscription(ctx context.Context, c *Client, in UpdateSubscriptionParams) (*Subscription, error) {
	if in.PriceID == "" && in.TrialEnd == nil {
		return nil, errors.New("stripe: UpdateSubscription: price_id or trial_end required")
	}

	params := &sdk.SubscriptionUpdateParams{
		ProrationBehavior: sdk.String(string(in.ProrationBehavior)),
	}

	if in.PriceID != "" {
		current, err := GetSubscription(ctx, c, in.SubscriptionID)
		if err != nil {
			return nil, err
		}
		if len(current.Items.Data) == 0 {
			return nil, errors.New("stripe: subscription has no items, cannot update")
		}
		params.Items = []*sdk.SubscriptionUpdateItemParams{
			{
				ID:    sdk.String(current.Items.Data[0].ID),
				Price: sdk.String(in.PriceID),
			},
		}
	}

	if in.TrialEnd != nil {
		params.TrialEnd = sdk.Int64(*in.TrialEnd)
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

// UpdateTrialEndParams captures a trial-end move and nothing else.
type UpdateTrialEndParams struct {
	SubscriptionID string
	TrialEnd       int64 // Unix seconds — required
	IdempotencyKey string
	Metadata       map[string]string
}

// UpdateTrialEnd moves a subscription's trial end without touching its price.
//
// A narrow wrapper rather than a documented convention on UpdateSubscription:
// the extend path must be structurally incapable of acquiring a PriceID
// later, and a struct with no PriceID field is the only way to guarantee that
// against a future edit. proration_behavior is `none` because the anchor move
// this causes must not generate a proration invoice.
func UpdateTrialEnd(ctx context.Context, c *Client, in UpdateTrialEndParams) (*Subscription, error) {
	if in.TrialEnd <= 0 {
		return nil, errors.New("stripe: UpdateTrialEnd: trial_end required")
	}
	return UpdateSubscription(ctx, c, UpdateSubscriptionParams{
		SubscriptionID:    in.SubscriptionID,
		TrialEnd:          &in.TrialEnd,
		ProrationBehavior: ProrationNone,
		IdempotencyKey:    in.IdempotencyKey,
		Metadata:          in.Metadata,
	})
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
