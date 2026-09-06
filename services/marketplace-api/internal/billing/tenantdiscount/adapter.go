package tenantdiscount

import (
	"context"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// StripeAdapter wraps *billingstripe.Client to satisfy StripeDiscounts.
// Construct one per client in main.go and pass it to NewService, mirroring
// trial.StripeAdapter (trial/subscribe.go:46).
//
// The three helpers it delegates to are package-level functions taking the
// client, not methods on it, so an adapter is the only way to express them as
// an interface — which is what keeps the fan-out testable without HTTP.
type StripeAdapter struct{ C *billingstripe.Client }

func (a *StripeAdapter) SubscriptionHasDiscount(ctx context.Context, subID, couponID string) (bool, error) {
	return billingstripe.SubscriptionHasDiscount(ctx, a.C, subID, couponID)
}

func (a *StripeAdapter) AddSubscriptionDiscount(ctx context.Context, subID, couponID string) error {
	return billingstripe.AddSubscriptionDiscount(ctx, a.C, subID, couponID)
}

func (a *StripeAdapter) RemoveSubscriptionDiscount(ctx context.Context, subID, couponID string) error {
	return billingstripe.RemoveSubscriptionDiscount(ctx, a.C, subID, couponID)
}
