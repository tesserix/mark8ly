package cancel

import (
	"context"
	"errors"
	"time"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// ErrStripeUnavailable is returned when a subscription that Stripe is billing
// cannot be reached or updated. The local row is left alone: a merchant who
// is told "cancelled" while Stripe keeps charging is the defect this whole
// change exists to remove, so a Stripe failure must fail the request instead.
var ErrStripeUnavailable = errors.New("cancel: stripe subscription could not be updated")

// ErrStripeNotWired is returned when the row carries a Stripe subscription id
// but no Stripe client is configured. Same reasoning as ErrStripeUnavailable:
// the only safe answer is to refuse, because the money is real and this
// process cannot stop it.
var ErrStripeNotWired = errors.New("cancel: stripe billing is not configured for a subscription Stripe is billing")

// StripeState is what the cancellation flow needs back from Stripe. Stripe is
// the authority on the period end — the date the merchant is told their
// access runs to — so it is read from the response rather than assumed from
// the local row, which may never have been populated.
type StripeState struct {
	CancelAtPeriodEnd bool
	CurrentPeriodEnd  time.Time // zero when Stripe reports none
}

// StripeCanceller is the narrow slice of the Stripe client this flow needs.
// Declared here so the dependency is optional and stubbable, the same shape
// as PromoApplier above it and trial.StripeAPI next door.
type StripeCanceller interface {
	CancelAtPeriodEnd(ctx context.Context, subscriptionID string) (StripeState, error)
	Resume(ctx context.Context, subscriptionID string) (StripeState, error)
}

// WithStripe returns a copy of the Service that schedules and reverses
// cancellations at Stripe. Passing a nil canceller leaves the Service
// unchanged — which is correct only for stores Stripe is not billing; see
// requireStripe.
func (s *Service) WithStripe(sc StripeCanceller) *Service {
	if s == nil || sc == nil {
		return s
	}
	cp := *s
	cp.stripe = sc
	return &cp
}

// StripeAdapter wraps *billingstripe.Client to satisfy StripeCanceller.
// Construct one in main.go and hand it to WithStripe.
type StripeAdapter struct{ C *billingstripe.Client }

// CancelAtPeriodEnd delegates to billingstripe.CancelAtPeriodEnd.
func (a *StripeAdapter) CancelAtPeriodEnd(ctx context.Context, subscriptionID string) (StripeState, error) {
	sub, err := billingstripe.CancelAtPeriodEnd(ctx, a.C, billingstripe.CancelAtPeriodEndParams{
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		return StripeState{}, err
	}
	return toStripeState(sub), nil
}

// Resume delegates to billingstripe.ResumeSubscription.
func (a *StripeAdapter) Resume(ctx context.Context, subscriptionID string) (StripeState, error) {
	sub, err := billingstripe.ResumeSubscription(ctx, a.C, billingstripe.ResumeSubscriptionParams{
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		return StripeState{}, err
	}
	return toStripeState(sub), nil
}

func toStripeState(sub *billingstripe.Subscription) StripeState {
	out := StripeState{CancelAtPeriodEnd: sub.CancelAtPeriodEnd}
	if sub.CurrentPeriodEnd > 0 {
		out.CurrentPeriodEnd = time.Unix(sub.CurrentPeriodEnd, 0).UTC()
	}
	return out
}
