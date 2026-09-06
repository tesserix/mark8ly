// Package trial implements the deferred-charge card-add flow (§5.3).
// A merchant adds their card at onboarding time; we provision a Stripe
// subscription with trial_end = the effective trial end (EndsAt) — normally
// signup_date + 90d, but a platform operator may have extended it — so
// Stripe defers the first invoice accordingly. No charge occurs at call time.
package trial

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TrialDays is the free-trial length mandated by §5.3.
const TrialDays = 90

var (
	// ErrSubscriptionAlreadyActive is returned when Subscribe is called on a
	// store whose subscription is already active. Callers should redirect to
	// the upgrade flow instead.
	ErrSubscriptionAlreadyActive = errors.New("trial: already active — use upgrade flow")

	// ErrMissingStripeCustomer is returned when the subscription row has no
	// Stripe customer ID, which means the onboarding card-collection step has
	// not completed yet.
	ErrMissingStripeCustomer = errors.New("trial: store has no Stripe customer")
)

// StripeAPI is the narrow interface Subscribe needs. The concrete satisfier is
// *billingstripe.Client used through StripeAdapter. Separating the interface
// from the concrete type keeps unit tests fast — no HTTP round-trips required.
type StripeAPI interface {
	CreateSubscription(ctx context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error)
	PriceIDFor(ctx context.Context, plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod, currency string, tier subscription.PriceTier) (string, error)
}

// StripeAdapter wraps *billingstripe.Client to satisfy StripeAPI.
// Construct one per billingstripe.Client in main.go and pass it to NewSubscriber.
type StripeAdapter struct{ C *billingstripe.Client }

// CreateSubscription delegates to the package-level billingstripe.CreateSubscription.
func (a *StripeAdapter) CreateSubscription(ctx context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	return billingstripe.CreateSubscription(ctx, a.C, in)
}

// PriceIDFor delegates to the package-level billingstripe.PriceIDFor.
func (a *StripeAdapter) PriceIDFor(ctx context.Context, plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod, currency string, tier subscription.PriceTier) (string, error) {
	return billingstripe.PriceIDFor(ctx, a.C, plan, period, currency, tier)
}

// TenantDiscountApplier offers a newly created subscription to the tenant's
// standing platform override (mark8ly#660). *tenantdiscount.Service satisfies
// it.
//
// Declared here rather than imported as a concrete type for the reason
// StripeAPI above is: it keeps the dependency pointing inward and lets the
// tests substitute a stub. There is no import cycle to avoid — tenantdiscount
// does not import this package — so the input and outcome types are the
// domain's own rather than re-spelled as strings.
type TenantDiscountApplier interface {
	ApplyToNewSubscription(ctx context.Context, in tenantdiscount.NewSubscriptionInput) (tenantdiscount.Outcome, error)
}

// Subscriber orchestrates the deferred-charge trial flow.
type Subscriber struct {
	db     *gorm.DB
	stripe StripeAPI
	clock  func() time.Time

	// discounts may be nil: without Stripe billing configured there is no
	// tenantdiscount.Service to wire, and a nil one means Subscribe behaves
	// exactly as it did before #660 T6.
	discounts TenantDiscountApplier
}

// NewSubscriber constructs a Subscriber. If clock is nil, time.Now().UTC() is used.
func NewSubscriber(db *gorm.DB, s StripeAPI, clock func() time.Time) *Subscriber {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Subscriber{db: db, stripe: s, clock: clock}
}

// WithTenantDiscount wires the tenant-override hook and returns the receiver
// so it can be chained onto NewSubscriber.
//
// A setter rather than a fourth NewSubscriber parameter: the discounter is
// optional (see Subscriber.discounts) and every existing caller and test
// constructs a Subscriber without one. It follows the same shape as
// brandingHandler.SetPlanResolver in main.go, where a dependency built later
// in the wiring is attached after construction.
func (s *Subscriber) WithTenantDiscount(d TenantDiscountApplier) *Subscriber {
	s.discounts = d
	return s
}

// SubscribeInput carries the caller-supplied parameters for a trial subscription.
type SubscribeInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	Plan     subscription.SubscriptionPlan
	Period   subscription.SubscriptionPeriod
	Currency string // ISO 4217, any case — normalised to lowercase by the handler
}

// SubscribeResult is the successful outcome of a Subscribe call.
type SubscribeResult struct {
	StripeSubscriptionID string
	TrialEndUnix         int64
}

// Subscribe creates a Stripe subscription with trial_end = the effective
// trial end (EndsAt) — normally signup_date + 90d, extended if an operator
// has set trial_ends_at.
//
// It does NOT flip subscription.status — the webhook owns that transition via
// statemachine.Transition. Keeping Subscribe a pure "provision Stripe + persist id"
// operation makes webhook-replay safety trivial: re-delivering the same event is
// idempotent because the state machine guards the transition.
//
// The advisory lock on storeID serialises concurrent calls for the same store so
// we never race two CreateSubscription calls against Stripe for the same store.
func (s *Subscriber) Subscribe(ctx context.Context, in SubscribeInput) (*SubscribeResult, error) {
	var out SubscribeResult
	err := subscription.WithAdvisoryLock(ctx, s.db, in.StoreID, func(tx *gorm.DB) error {
		result, err := s.subscribeInTx(ctx, tx, in)
		if err != nil {
			return err
		}
		out = *result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// subscribeInTx executes the subscribe logic inside an already-open transaction.
// It is a separate method so integration tests can inject a real GORM tx
// without going through the advisory-lock wrapper.
func (s *Subscriber) subscribeInTx(ctx context.Context, tx *gorm.DB, in SubscribeInput) (*SubscribeResult, error) {
	var row subscription.StoreSubscription
	if err := tx.Where("tenant_id = ? AND store_id = ?", in.TenantID, in.StoreID).
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("trial: load store: %w", err)
	}

	if row.Status == subscription.StatusActive {
		return nil, ErrSubscriptionAlreadyActive
	}
	if row.StripeCustomerID == "" {
		return nil, ErrMissingStripeCustomer
	}

	// trial_end is the EFFECTIVE end: an operator-extended trial must bill on
	// the extended date, not created_at + TrialDays (#353).
	trialEnd := EndsAt(row).Unix()

	priceID, err := s.stripe.PriceIDFor(ctx, in.Plan, in.Period, in.Currency, row.PriceTier)
	if err != nil {
		return nil, fmt.Errorf("trial: resolve price: %w", err)
	}

	sub, err := s.stripe.CreateSubscription(ctx, billingstripe.CreateSubscriptionInput{
		StoreID:    in.StoreID.String(),
		Plan:       string(in.Plan),
		Period:     string(in.Period),
		CustomerID: row.StripeCustomerID,
		PriceID:    priceID,
		TrialEnd:   trialEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("trial: create stripe subscription: %w", err)
	}

	if err := tx.Model(&subscription.StoreSubscription{}).
		Where("tenant_id = ? AND store_id = ?", in.TenantID, in.StoreID).
		Update("stripe_subscription_id", sub.ID).Error; err != nil {
		return nil, fmt.Errorf("trial: persist subscription id: %w", err)
	}

	// #660 T6 — the tenant may hold a standing platform override granted
	// before this store had a Stripe subscription to carry it. Applying it
	// here is what makes the grant cover stores created after the operator
	// pressed the button.
	s.applyTenantDiscount(ctx, in, sub.ID)

	return &SubscribeResult{
		StripeSubscriptionID: sub.ID,
		TrialEndUnix:         trialEnd,
	}, nil
}

// applyTenantDiscount offers the new subscription to the tenant's standing
// override, and RETURNS NOTHING.
//
// That is the contract, not an oversight. A discount that cannot be applied
// costs the tenant a discount an operator can re-apply by hand; a discount
// that blocks this call costs the merchant their subscription. The first is
// recoverable and the second is not, so the failure is logged and dropped.
//
// It also touches no transaction: ApplyToNewSubscription works entirely on the
// tenantdiscount service's own handle. A failed statement poisons a Postgres
// transaction, so writing on the caller's tx would turn exactly the failure
// this function is built to swallow back into a failed subscription.
func (s *Subscriber) applyTenantDiscount(ctx context.Context, in SubscribeInput, stripeSubID string) {
	if s.discounts == nil {
		return
	}
	outcome, err := s.discounts.ApplyToNewSubscription(ctx, tenantdiscount.NewSubscriptionInput{
		TenantID:             in.TenantID,
		StoreID:              in.StoreID,
		StripeSubscriptionID: stripeSubID,
	})
	if err != nil {
		slog.Error("trial: could not apply the tenant's standing platform override to a new subscription",
			"tenant_id", in.TenantID.String(),
			"store_id", in.StoreID.String(),
			"stripe_subscription_id", stripeSubID,
			"err", err)
		return
	}
	if outcome == tenantdiscount.OutcomeApplied {
		slog.Info("trial: applied the tenant's standing platform override to a new subscription",
			"tenant_id", in.TenantID.String(),
			"store_id", in.StoreID.String(),
			"stripe_subscription_id", stripeSubID)
	}
}
