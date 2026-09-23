package stripe

import (
	"context"
	"errors"

	sdk "github.com/stripe/stripe-go/v82"
)

// CancelAtPeriodEndParams captures a cancellation schedule and nothing else.
type CancelAtPeriodEndParams struct {
	SubscriptionID string
	IdempotencyKey string
	Metadata       map[string]string
}

// CancelAtPeriodEnd schedules a subscription to stop at the end of the period
// the merchant has already paid for, and returns the subscription as Stripe
// now holds it.
//
// This is the write that was missing entirely: cancellation was recorded
// locally and Stripe was never told, so a merchant lost access at the next
// FinalizeCron tick and kept being charged. Everything the merchant is told
// about when their access ends comes from the CurrentPeriodEnd on the
// returned object — Stripe owns that date, not us.
//
// It does NOT delete the subscription. A merchant who cancels keeps what they
// paid for until the period ends, which is also what makes the save offer
// possible: ResumeSubscription reverses this exactly while the period is
// still running.
//
// A narrow wrapper rather than a flag on UpdateSubscription, for the reason
// UpdateTrialEnd is one: a struct with no PriceID and no TrialEnd field
// cannot grow into a re-price or an anchor move in a later edit. It sends no
// items, so no proration can arise and proration_behavior is left unset.
func CancelAtPeriodEnd(ctx context.Context, c *Client, in CancelAtPeriodEndParams) (*Subscription, error) {
	return setCancelAtPeriodEnd(ctx, c, in.SubscriptionID, true, in.IdempotencyKey, in.Metadata)
}

// ResumeSubscriptionParams captures a cancellation reversal and nothing else.
type ResumeSubscriptionParams struct {
	SubscriptionID string
	IdempotencyKey string
	Metadata       map[string]string
}

// ResumeSubscription clears a scheduled cancellation, so billing continues.
//
// This is the save offer's other half. Accepting the offer flips the local
// row back to active; without this the subscription would still stop at
// Stripe on the date the merchant was just told it would not — the worst
// shape of the bug, because they un-cancelled in reliance on that sentence.
//
// Only valid while the period is still running: once Stripe has actually
// ended the subscription there is nothing to reverse, and Stripe rejects it.
func ResumeSubscription(ctx context.Context, c *Client, in ResumeSubscriptionParams) (*Subscription, error) {
	return setCancelAtPeriodEnd(ctx, c, in.SubscriptionID, false, in.IdempotencyKey, in.Metadata)
}

func setCancelAtPeriodEnd(ctx context.Context, c *Client, subscriptionID string, cancel bool, idempotencyKey string, metadata map[string]string) (*Subscription, error) {
	if subscriptionID == "" {
		return nil, errors.New("stripe: setCancelAtPeriodEnd: subscription_id required")
	}

	params := &sdk.SubscriptionUpdateParams{
		CancelAtPeriodEnd: sdk.Bool(cancel),
	}
	for k, v := range metadata {
		params.AddMetadata(k, v)
	}
	if idempotencyKey != "" {
		params.SetIdempotencyKey(idempotencyKey)
	}

	sdkSub, err := c.sdk.V1Subscriptions.Update(ctx, subscriptionID, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapSubscription(sdkSub), nil
}
