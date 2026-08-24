// Package trial implements the deferred-charge card-add flow (§5.3).
// A merchant adds their card at onboarding time; we provision a Stripe
// subscription with trial_end = signup_date + 90d so Stripe defers the first
// invoice to day 90. No charge occurs at call time.
package trial

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
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

// Subscriber orchestrates the deferred-charge trial flow.
type Subscriber struct {
	db     *gorm.DB
	stripe StripeAPI
	clock  func() time.Time
}

// NewSubscriber constructs a Subscriber. If clock is nil, time.Now().UTC() is used.
func NewSubscriber(db *gorm.DB, s StripeAPI, clock func() time.Time) *Subscriber {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Subscriber{db: db, stripe: s, clock: clock}
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

// Subscribe creates a Stripe subscription with trial_end = signup_date + 90d.
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

	return &SubscribeResult{
		StripeSubscriptionID: sub.ID,
		TrialEndUnix:         trialEnd,
	}, nil
}
