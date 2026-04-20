// Package planchange orchestrates §4.4 + §4.5 plan and period changes.
// rules.go holds the pure-function decision helpers; this file wires them
// into a DB-backed orchestrator with Stripe and audit integration.
package planchange

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// StripeClient is the consumer-side interface the orchestrator depends on.
// It allows unit tests to substitute a fake without a real HTTP server.
// billingstripe.Client satisfies this via StripeClientAdapter.
type StripeClient interface {
	UpdateSubscription(ctx context.Context, in billingstripe.UpdateSubscriptionParams) (*billingstripe.Subscription, error)
	CreateSubscription(ctx context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error)
	PriceIDFor(ctx context.Context, plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod, currency string, tier subscription.PriceTier) (string, error)
}

// StripeClientAdapter wraps *billingstripe.Client to satisfy StripeClient.
// The billing/stripe package exposes package-level functions rather than
// methods; this adapter bridges the gap without modifying that package.
type StripeClientAdapter struct{ C *billingstripe.Client }

func (a *StripeClientAdapter) UpdateSubscription(ctx context.Context, in billingstripe.UpdateSubscriptionParams) (*billingstripe.Subscription, error) {
	return billingstripe.UpdateSubscription(ctx, a.C, in)
}

func (a *StripeClientAdapter) CreateSubscription(ctx context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	return billingstripe.CreateSubscription(ctx, a.C, in)
}

func (a *StripeClientAdapter) PriceIDFor(ctx context.Context, plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod, currency string, tier subscription.PriceTier) (string, error) {
	return billingstripe.PriceIDFor(ctx, a.C, plan, period, currency, tier)
}

// Sentinel errors returned by Execute.
var (
	ErrSubscriptionReadOnly = errors.New("planchange: subscription is in read-only status")
	ErrCurrencyLocked       = errors.New("planchange: currency cannot change mid-term")
	ErrNoChange             = errors.New("planchange: target plan + period identical to current")
	ErrStoreCountOverQuota  = errors.New("planchange: target plan does not permit current store count")
	ErrInvalidTargetPlan    = errors.New("planchange: target plan not merchant-selectable")
)

// Result describes what the orchestrator did.
type Result string

const (
	ResultUpgradeCommitted      Result = "upgrade_committed"
	ResultDowngradeScheduled    Result = "downgrade_scheduled"
	ResultPeriodSwitchCommitted Result = "period_switch_committed"
)

// Input carries the change request parameters.
type Input struct {
	TenantID          uuid.UUID
	StoreID           uuid.UUID
	TargetPlan        subscription.SubscriptionPlan
	TargetPeriod      subscription.SubscriptionPeriod
	RequestedCurrency string
	Actor             string
	Reason            string
	GinCtx            *gin.Context
	Now               time.Time
}

// Output carries the result of a successful Execute call.
type Output struct {
	Result        Result
	EffectiveAt   time.Time
	StripeUpdated bool
}

// Clock abstracts wall-clock access for testing.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// BudgetRecomputer is implemented by campaignbudget.Service. The interface
// lives here so the planchange package does not import campaignbudget directly
// (avoiding a circular dependency). P9 wires the concrete implementation in
// main.go via Deps.BudgetRecomputer.
//
// RecomputeLimitForPlan is called inside the advisory-locked transaction that
// commits a plan upgrade so the budget limit_set and plan row always commit
// atomically. A nil BudgetRecomputer is a no-op — existing P4 tests remain
// green without any change.
type BudgetRecomputer interface {
	RecomputeLimitForPlan(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, plan string) error
}

// Deps groups Orchestrator dependencies.
type Deps struct {
	DB               *gorm.DB
	Stripe           StripeClient
	Emitter          *audit.Emitter
	SubscriptionRepo subscription.Repository
	StoreRepo        stores.Repository
	Clock            Clock
	// BudgetRecomputer is optional (P9). When non-nil, RecomputeLimitForPlan
	// is called inside the upgrade/downgrade-commit transaction so that plan
	// and budget rows always commit atomically.
	BudgetRecomputer BudgetRecomputer
}

// Orchestrator runs the plan-change workflow under an advisory lock.
type Orchestrator struct{ deps Deps }

// NewOrchestrator constructs an Orchestrator. A nil Clock defaults to
// the real wall clock.
func NewOrchestrator(d Deps) *Orchestrator {
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	return &Orchestrator{deps: d}
}

// merchantSelectablePlans enumerates plans users can request via the change-plan
// endpoint. Marketplace is hidden-internal; trial upgrades happen via checkout.
var merchantSelectablePlans = map[subscription.SubscriptionPlan]bool{
	subscription.PlanStarter: true,
	subscription.PlanStudio:  true,
	subscription.PlanPro:     true,
}

// Execute validates the request, acquires an advisory lock, and delegates to
// executeUpgrade or executeDowngradeSchedule depending on direction.
func (o *Orchestrator) Execute(ctx context.Context, in Input) (Output, error) {
	// Validate target plan is merchant-selectable.
	if !merchantSelectablePlans[in.TargetPlan] {
		return Output{}, ErrInvalidTargetPlan
	}

	if in.Now.IsZero() {
		in.Now = o.deps.Clock.Now()
	}

	var out Output
	err := subscription.WithAdvisoryLock(ctx, o.deps.DB, in.StoreID, func(tx *gorm.DB) error {
		sub, err := o.deps.SubscriptionRepo.GetByStoreID(ctx, tx, in.TenantID, in.StoreID)
		if err != nil {
			return fmt.Errorf("planchange: load subscription: %w", err)
		}

		// Reject read-only statuses.
		switch sub.Status {
		case subscription.StatusExpired,
			subscription.StatusStoreClosed,
			subscription.StatusPendingHardDelete,
			subscription.StatusHardDeleted:
			return ErrSubscriptionReadOnly
		}

		// Currency lock: if the caller specified a currency, it must match the
		// stored billing currency (case-insensitive). Changing currency mid-term
		// would break Stripe's subscription continuity.
		if in.RequestedCurrency != "" && sub.BillingCurrency != nil &&
			!strings.EqualFold(in.RequestedCurrency, *sub.BillingCurrency) {
			return ErrCurrencyLocked
		}

		dir := Classify(sub.Plan, sub.SubscriptionPeriod, in.TargetPlan, in.TargetPeriod)
		if dir == DirectionNoChange {
			return ErrNoChange
		}

		if dir == DirectionUpgrade {
			o2, err := o.executeUpgrade(ctx, tx, in, sub)
			if err != nil {
				return err
			}
			out = o2
			return nil
		}

		// Downgrade or period downgrade.
		o2, err := o.executeDowngradeSchedule(ctx, tx, in, sub)
		if err != nil {
			return err
		}
		out = o2
		return nil
	})
	if err != nil {
		return Output{}, err
	}
	return out, nil
}

// executeInitialSubscription creates the first Stripe subscription for a row
// that was lazily bootstrapped (stripe_customer_id set, stripe_subscription_id
// still NULL). Uses the 90-day trial flow per §4.6 so nothing is charged until
// trial_end. The row stays in signup state until the customer.subscription.*
// webhook fires statemachine.Transition(signup → trialing).
func (o *Orchestrator) executeInitialSubscription(ctx context.Context, tx *gorm.DB, in Input, sub *subscription.StoreSubscription) (Output, error) {
	if sub.StripeCustomerID == "" {
		return Output{}, fmt.Errorf("planchange: subscription has no stripe_customer_id — run bootstrap first")
	}

	currency := ""
	if sub.BillingCurrency != nil {
		currency = *sub.BillingCurrency
	}
	priceID, err := o.deps.Stripe.PriceIDFor(ctx, in.TargetPlan, in.TargetPeriod, currency, sub.PriceTier)
	if err != nil {
		return Output{}, fmt.Errorf("planchange: resolve price id: %w", err)
	}

	// trial_end = signup_date + 90d. CreatedAt is the signup timestamp.
	trialEnd := sub.CreatedAt.Add(90 * 24 * time.Hour).Unix()

	stripeSub, err := o.deps.Stripe.CreateSubscription(ctx, billingstripe.CreateSubscriptionInput{
		StoreID:    in.StoreID.String(),
		Plan:       string(in.TargetPlan),
		Period:     string(in.TargetPeriod),
		CustomerID: sub.StripeCustomerID,
		PriceID:    priceID,
		TrialEnd:   trialEnd,
	})
	if err != nil {
		return Output{}, fmt.Errorf("planchange: stripe create subscription: %w", err)
	}

	// Persist stripe_subscription_id + plan + period onto the row. Status
	// stays signup until the webhook fires the transition — the state
	// machine owns that.
	if err := tx.Model(&subscription.StoreSubscription{}).
		Where("tenant_id = ? AND store_id = ?", in.TenantID, in.StoreID).
		Updates(map[string]any{
			"stripe_subscription_id": stripeSub.ID,
			"plan":                   in.TargetPlan,
			"subscription_period":    in.TargetPeriod,
			"last_plan_change_at":    in.Now,
			"last_plan_change_reason": "initial_selection",
		}).Error; err != nil {
		return Output{}, fmt.Errorf("planchange: persist initial subscription: %w", err)
	}

	if o.deps.Emitter != nil && in.GinCtx != nil {
		o.deps.Emitter.Emit(in.GinCtx, audit.Event{
			Action:       "subscription.initial_selected",
			ResourceType: "subscription",
			Metadata: map[string]any{
				"plan":                   string(in.TargetPlan),
				"period":                 string(in.TargetPeriod),
				"stripe_subscription_id": stripeSub.ID,
			},
		})
	}

	return Output{
		Result:        ResultUpgradeCommitted,
		EffectiveAt:   in.Now,
		StripeUpdated: true,
	}, nil
}

// executeUpgrade handles immediate plan upgrades and period upgrades
// (monthly → annual). Called from within the advisory lock.
func (o *Orchestrator) executeUpgrade(ctx context.Context, tx *gorm.DB, in Input, sub *subscription.StoreSubscription) (Output, error) {
	if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
		return o.executeInitialSubscription(ctx, tx, in, sub)
	}

	// Determine billing currency to use.
	currency := ""
	if sub.BillingCurrency != nil {
		currency = *sub.BillingCurrency
	}

	// Resolve the Stripe price ID for the target plan/period.
	priceID, err := o.deps.Stripe.PriceIDFor(ctx, in.TargetPlan, in.TargetPeriod, currency, sub.PriceTier)
	if err != nil {
		return Output{}, fmt.Errorf("planchange: resolve price id: %w", err)
	}

	// Idempotency key scoped to (store, target plan, target period, 5-min bucket).
	// The 5-min window matches the advisory lock TTL — duplicate requests within
	// the same minute get deduplicated by Stripe rather than creating double charges.
	idempotencyKey := fmt.Sprintf("plan-change:%s:%s:%s:%d",
		in.StoreID, in.TargetPlan, in.TargetPeriod,
		in.Now.Truncate(5*time.Minute).Unix(),
	)

	stripeSub, err := o.deps.Stripe.UpdateSubscription(ctx, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    *sub.StripeSubscriptionID,
		PriceID:           priceID,
		ProrationBehavior: billingstripe.ProrationAlwaysInvoice,
		IdempotencyKey:    idempotencyKey,
		Metadata: map[string]string{
			"tenant_id":     in.TenantID.String(),
			"store_id":      in.StoreID.String(),
			"target_plan":   string(in.TargetPlan),
			"target_period": string(in.TargetPeriod),
		},
	})
	if err != nil {
		return Output{}, fmt.Errorf("planchange: stripe update subscription: %w", err)
	}

	// Determine action label: upgrade_committed when plan changes, period_switch_committed
	// when only the billing period changes within the same plan tier.
	action := "upgrade_committed"
	var result Result = ResultUpgradeCommitted
	if sub.Plan == in.TargetPlan {
		action = "period_switch_committed"
		result = ResultPeriodSwitchCommitted
	}

	if err := o.deps.SubscriptionRepo.CommitUpgrade(ctx, tx, in.TenantID, in.StoreID,
		in.TargetPlan, in.TargetPeriod, action); err != nil {
		return Output{}, fmt.Errorf("planchange: commit upgrade: %w", err)
	}

	// P9 — recompute campaign email budget limit inside the same transaction
	// so plan row and budget row always commit atomically. Nil-safe: existing
	// callers that don't inject BudgetRecomputer are unaffected.
	if o.deps.BudgetRecomputer != nil {
		if err := o.deps.BudgetRecomputer.RecomputeLimitForPlan(ctx, tx, in.StoreID, string(in.TargetPlan)); err != nil {
			return Output{}, fmt.Errorf("planchange: recompute budget limit: %w", err)
		}
	}

	// BillingCurrency is stored upper-cased to satisfy the CHAR(3) NOT NULL
	// constraint with canonical ISO-4217 form (e.g. "USD" not "usd").
	auditCurrency := strings.ToUpper(currency)
	if auditCurrency == "" {
		auditCurrency = "USD" // fallback for subscriptions without explicit currency
	}

	// TODO: backfill proration fields from invoice webhook — ProrationCents and
	// StripeInvoiceID are set to zero/"" here and populated when
	// invoice.payment_succeeded arrives from the Stripe webhook handler.
	stripeSubID := ""
	if stripeSub != nil {
		stripeSubID = stripeSub.ID
	}

	if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
		TenantID:             in.TenantID,
		StoreID:              in.StoreID,
		StripeSubscriptionID: stripeSubID,
		StripeInvoiceID:      "",
		FromPlan:             sub.Plan,
		ToPlan:               in.TargetPlan,
		FromPeriod:           sub.SubscriptionPeriod,
		ToPeriod:             in.TargetPeriod,
		Action:               action,
		BillingCurrency:      auditCurrency,
		ProrationCents:       0,
		Actor:                in.Actor,
		Reason:               in.Reason,
		EffectiveAt:          in.Now,
	}); err != nil {
		return Output{}, fmt.Errorf("planchange: write audit row: %w", err)
	}

	// Emit to audit log (non-blocking; nil emitter is safe).
	if o.deps.Emitter != nil {
		o.deps.Emitter.EmitPlanChange(in.GinCtx, audit.PlanChange{
			TenantID:    in.TenantID,
			StoreID:     in.StoreID,
			FromPlan:    string(sub.Plan),
			ToPlan:      string(in.TargetPlan),
			FromPeriod:  string(sub.SubscriptionPeriod),
			ToPeriod:    string(in.TargetPeriod),
			Subaction:   action,
			Actor:       in.Actor,
			Reason:      in.Reason,
			EffectiveAt: in.Now,
		})
	}

	return Output{
		Result:        result,
		EffectiveAt:   in.Now,
		StripeUpdated: true,
	}, nil
}

