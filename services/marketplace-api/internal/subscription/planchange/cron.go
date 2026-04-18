package planchange

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// DowngradeBlockNotifier delivers the §4.5.1 "we kept you on the higher plan"
// email. Injected so tests can count invocations without hitting SendGrid.
// Production wiring in Task 14 supplies a SendGrid-backed implementation (or
// a no-op until the email template lands).
type DowngradeBlockNotifier interface {
	NotifyDowngradeBlocked(
		ctx context.Context,
		tenantID, storeID uuid.UUID,
		reason string,
		details map[string]any,
	) error
}

// CronDeps groups every dependency the recheck cron needs. Match the fields
// the orchestrator uses so main.go can construct one set and feed both.
type CronDeps struct {
	DB               *gorm.DB
	SubscriptionRepo subscription.Repository
	StoreRepo        stores.Repository
	Stripe           StripeClient
	Emitter          *audit.Emitter
	Clock            Clock
	Notifier         DowngradeBlockNotifier
	Interval         time.Duration // defaults to 1h
	Logger           *slog.Logger
}

// DowngradeRecheckCron runs on a configurable interval (default 1h), picks up
// every subscription row whose pending_downgrade_effective_at has passed, and
// either commits the plan swap or blocks + emails the merchant when the
// store count would exceed the target plan's cap (§4.5.1).
type DowngradeRecheckCron struct {
	deps CronDeps
	sch  *cron.Cron
}

// NewDowngradeRecheckCron constructs a cron. Zero-value Clock, Interval, and
// Logger fields are filled with safe defaults so callers need only set the
// fields they care about.
func NewDowngradeRecheckCron(d CronDeps) *DowngradeRecheckCron {
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	if d.Interval == 0 {
		d.Interval = time.Hour
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &DowngradeRecheckCron{deps: d}
}

// Start schedules RunOnce at deps.Interval using robfig/cron. Mirror the
// pattern from internal/billing/dispatch/cron.go.
func (c *DowngradeRecheckCron) Start(ctx context.Context) error {
	c.sch = cron.New()
	_, err := c.sch.AddFunc(
		fmt.Sprintf("@every %s", c.deps.Interval.String()),
		func() { _ = c.RunOnce(ctx) },
	)
	if err != nil {
		return err
	}
	c.sch.Start()
	return nil
}

// Stop halts the underlying cron scheduler. Safe to call on a nil scheduler.
func (c *DowngradeRecheckCron) Stop() {
	if c.sch != nil {
		c.sch.Stop()
	}
}

// RunOnce executes a single pass: it loads all ready-to-commit pending-downgrade
// rows and processes each one under its own advisory lock. Row-level failures
// are logged and skipped so a single bad row does not abort the whole pass.
func (c *DowngradeRecheckCron) RunOnce(ctx context.Context) error {
	rows, err := c.deps.SubscriptionRepo.FindPendingDowngradesReady(ctx, c.deps.DB, c.deps.Clock.Now())
	if err != nil {
		return err
	}
	for i := range rows {
		if err := c.processOne(ctx, &rows[i]); err != nil {
			c.deps.Logger.Error("downgrade_recheck: row failed; will retry next tick",
				"store_id", rows[i].StoreID.String(),
				"tenant_id", rows[i].TenantID.String(),
				"err", err.Error(),
			)
		}
	}
	return nil
}

// processOne acquires a per-store advisory lock and delegates to ProcessOneInLock.
// Splitting the methods enables unit tests to call ProcessOneInLock directly with
// a nil tx, bypassing the PostgreSQL advisory-lock call entirely.
func (c *DowngradeRecheckCron) processOne(ctx context.Context, sub *subscription.StoreSubscription) error {
	return subscription.WithAdvisoryLock(ctx, c.deps.DB, sub.StoreID, func(tx *gorm.DB) error {
		return c.ProcessOneInLock(ctx, tx, sub)
	})
}

// ProcessOneInLock performs the re-read-under-lock + commit-or-block logic. It
// is exported so that the external planchange_test package can call it directly
// with a nil tx and in-memory fakes, bypassing the PostgreSQL advisory-lock call.
func (c *DowngradeRecheckCron) ProcessOneInLock(ctx context.Context, tx *gorm.DB, sub *subscription.StoreSubscription) error {
	// Re-read inside the lock so we act on the freshest state and avoid
	// processing a row that was already committed by a concurrent worker.
	fresh, err := c.deps.SubscriptionRepo.GetByStoreID(ctx, tx, sub.TenantID, sub.StoreID)
	if err != nil {
		return err
	}

	// Another worker may have already committed or cleared the pending downgrade.
	if fresh.PendingDowngradePlan == nil {
		return nil
	}

	// Guard against clock skew or a row returned too early by FindPendingDowngradesReady.
	if fresh.PendingDowngradeEffectiveAt == nil ||
		fresh.PendingDowngradeEffectiveAt.After(c.deps.Clock.Now()) {
		return nil
	}

	target := *fresh.PendingDowngradePlan
	period := *fresh.PendingDowngradePeriod

	// §4.5.1: Re-check store count at execution time. A merchant may have
	// added stores between scheduling and committing the downgrade.
	if RequiresStoreCountCheck(fresh.Plan, target) {
		count, err := c.deps.StoreRepo.CountActiveOrSoftDeletedRestorableTx(ctx, tx, fresh.TenantID)
		if err != nil {
			return fmt.Errorf("downgrade_recheck: count stores: %w", err)
		}
		limit := plangate.Limit(target, plangate.FeatureStores)
		if limit != plangate.Unlimited && count > limit {
			return c.blockAndNotify(ctx, tx, fresh, target, period, count, limit)
		}
	}

	return c.commitDowngrade(ctx, tx, fresh, target, period)
}

// commitDowngrade performs the Stripe swap → repo CommitDowngrade → audit row
// → emitter sequence inside the already-held advisory lock.
func (c *DowngradeRecheckCron) commitDowngrade(
	ctx context.Context,
	tx *gorm.DB,
	fresh *subscription.StoreSubscription,
	target subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
) error {
	currency := ""
	if fresh.BillingCurrency != nil {
		currency = *fresh.BillingCurrency
	}

	priceID, err := c.deps.Stripe.PriceIDFor(ctx, target, period, currency, fresh.PriceTier)
	if err != nil {
		return fmt.Errorf("downgrade_recheck: resolve price id: %w", err)
	}

	// Idempotency key scoped to (store, target plan, target period, 5-min bucket)
	// to survive retries within the same window without double-charging.
	idempotencyKey := fmt.Sprintf("plan-change-cron:%s:%s:%s:%d",
		fresh.StoreID, target, period,
		c.deps.Clock.Now().Truncate(5*time.Minute).Unix(),
	)

	stripeSubID := ""
	if fresh.StripeSubscriptionID != nil {
		stripeSubID = *fresh.StripeSubscriptionID
	}

	if _, err := c.deps.Stripe.UpdateSubscription(ctx, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    stripeSubID,
		PriceID:           priceID,
		ProrationBehavior: billingstripe.ProrationNone, // already paid for the period
		IdempotencyKey:    idempotencyKey,
		Metadata: map[string]string{
			"store_id":       fresh.StoreID.String(),
			"tenant_id":      fresh.TenantID.String(),
			"mark8ly_action": "plan_change_downgrade_commit",
		},
	}); err != nil {
		return fmt.Errorf("downgrade_recheck: stripe update subscription: %w", err)
	}

	// CommitDowngrade signature: (ctx, db, tenantID, storeID, newPlan, newPeriod) — no reason arg.
	if err := c.deps.SubscriptionRepo.CommitDowngrade(ctx, tx,
		fresh.TenantID, fresh.StoreID, target, period); err != nil {
		return fmt.Errorf("downgrade_recheck: commit downgrade: %w", err)
	}

	auditCurrency := buildAuditCurrency(fresh)
	now := c.deps.Clock.Now()

	if tx != nil {
		if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
			TenantID:             fresh.TenantID,
			StoreID:              fresh.StoreID,
			StripeSubscriptionID: stripeSubID,
			FromPlan:             fresh.Plan,
			ToPlan:               target,
			FromPeriod:           fresh.SubscriptionPeriod,
			ToPeriod:             period,
			Action:               "downgrade_committed",
			BillingCurrency:      auditCurrency,
			Actor:                "system:cron:downgrade_recheck",
			Reason:               "scheduled_execution",
			EffectiveAt:          now,
		}); err != nil {
			return fmt.Errorf("downgrade_recheck: write audit row: %w", err)
		}
	}

	if c.deps.Emitter != nil {
		c.deps.Emitter.EmitPlanChange(nil, audit.PlanChange{
			TenantID:    fresh.TenantID,
			StoreID:     fresh.StoreID,
			FromPlan:    string(fresh.Plan),
			ToPlan:      string(target),
			FromPeriod:  string(fresh.SubscriptionPeriod),
			ToPeriod:    string(period),
			Subaction:   "downgrade_committed",
			Actor:       "system:cron:downgrade_recheck",
			Reason:      "scheduled_execution",
			EffectiveAt: now,
		})
	}
	return nil
}

// blockAndNotify clears the pending downgrade, writes a blocked audit row,
// emits to the audit pipeline, and fires the merchant email notifier.
func (c *DowngradeRecheckCron) blockAndNotify(
	ctx context.Context,
	tx *gorm.DB,
	fresh *subscription.StoreSubscription,
	target subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
	count, limit int,
) error {
	if err := c.deps.SubscriptionRepo.ClearPendingDowngrade(ctx, tx, fresh.TenantID, fresh.StoreID); err != nil {
		return fmt.Errorf("downgrade_recheck: clear pending downgrade: %w", err)
	}

	reason := fmt.Sprintf("store_count=%d target_cap=%d", count, limit)
	auditCurrency := buildAuditCurrency(fresh)
	now := c.deps.Clock.Now()

	stripeSubID := ""
	if fresh.StripeSubscriptionID != nil {
		stripeSubID = *fresh.StripeSubscriptionID
	}

	if tx != nil {
		if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
			TenantID:             fresh.TenantID,
			StoreID:              fresh.StoreID,
			StripeSubscriptionID: stripeSubID,
			FromPlan:             fresh.Plan,
			ToPlan:               target,
			FromPeriod:           fresh.SubscriptionPeriod,
			ToPeriod:             period,
			Action:               "downgrade_blocked_over_quota",
			BillingCurrency:      auditCurrency,
			Actor:                "system:cron:downgrade_recheck",
			Reason:               reason,
			EffectiveAt:          now,
		}); err != nil {
			return fmt.Errorf("downgrade_recheck: write blocked audit row: %w", err)
		}
	}

	if c.deps.Emitter != nil {
		c.deps.Emitter.EmitPlanChange(nil, audit.PlanChange{
			TenantID:    fresh.TenantID,
			StoreID:     fresh.StoreID,
			FromPlan:    string(fresh.Plan),
			ToPlan:      string(target),
			FromPeriod:  string(fresh.SubscriptionPeriod),
			ToPeriod:    string(period),
			Subaction:   "downgrade_blocked_over_quota",
			Actor:       "system:cron:downgrade_recheck",
			Reason:      reason,
			EffectiveAt: now,
		})
	}

	if c.deps.Notifier != nil {
		if err := c.deps.Notifier.NotifyDowngradeBlocked(ctx, fresh.TenantID, fresh.StoreID,
			"store_count_over_quota",
			map[string]any{
				"store_count":  count,
				"target_cap":   limit,
				"target_plan":  string(target),
				"current_plan": string(fresh.Plan),
			}); err != nil {
			c.deps.Logger.Warn("downgrade_recheck: notifier failed",
				"store_id", fresh.StoreID.String(),
				"err", err.Error(),
			)
		}
	}
	return nil
}

