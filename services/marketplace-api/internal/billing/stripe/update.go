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

// lookupKeyFor resolves the Stripe price lookup key for subscription-layer
// types by ASKING THE CATALOG for it (#459).
//
// It deliberately does not build the key itself. It used to: it called
// MustGetDescriptor only to validate the descriptor existed, discarded it,
// and rebuilt the string with its own fmt.Sprintf — leaving the format
// literal in two packages with nothing checking they agreed. A change to
// the catalog format would have left this path writing the old key,
// pointing subscription updates at prices that are stale or absent, and it
// would have rotted longest in the unattended downgrade cron.
//
// pricing.LookupKeyFor takes the currency that MustGetDescriptor could not,
// which was the stated reason for the second copy.
// TestLookupKeyFor_AgreesWithEveryCatalogDescriptor holds the invariant.
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
		key, ok := pricing.LookupKeyFor(p, per, pricing.TierDeveloped, currency)
		if !ok {
			return "", fmt.Errorf("stripe: no developed price for plan=%s period=%s", plan, period)
		}
		return key, nil
	case subscription.PriceTierPPP:
		key, ok := pricing.LookupKeyFor(p, per, pricing.TierPPP, currency)
		if !ok {
			return "", fmt.Errorf("stripe: no PPP price for plan=%s period=%s currency=%s", plan, period, currency)
		}
		return key, nil
	default:
		return "", fmt.Errorf("stripe: unknown price tier %q", tier)
	}
}
