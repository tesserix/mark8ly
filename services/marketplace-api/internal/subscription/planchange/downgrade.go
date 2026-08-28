package planchange

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// executeDowngradeSchedule parks a pending downgrade at period-end after
// running the store-count preflight guard (§4.5.1). Called from within the
// advisory lock transaction tx.
//
// Store-count check: we call stores.NewRepository(tx) on each invocation.
// gormRepository holds only a *gorm.DB handle so construction is allocation-
// trivial. Using tx ensures the count is consistent with any uncommitted rows
// in the same advisory-lock transaction.
func (o *Orchestrator) executeDowngradeSchedule(
	ctx context.Context, tx *gorm.DB, in Input, sub *subscription.StoreSubscription,
) (Output, *PlanChangeAuditRow, error) {
	// Store-count preflight — only required for transitions that reduce the
	// store limit (currently only Studio→Starter per §4.5.1).
	if RequiresStoreCountCheck(sub.Plan, in.TargetPlan) {
		storeLimit := plangate.Limit(in.TargetPlan, plangate.FeatureStores)

		// Count using the tx-scoped repo so the read is inside the advisory lock.
		storeRepo := stores.NewRepository(tx)
		count, err := storeRepo.CountActiveOrSoftDeletedRestorableTx(ctx, tx, in.TenantID)
		if err != nil {
			return Output{}, nil, fmt.Errorf("planchange: count stores for preflight: %w", err)
		}

		// storeLimit == plangate.Unlimited (-1) means no cap; skip the check.
		if storeLimit != plangate.Unlimited && count > storeLimit {
			// The audit row must OUTLIVE this transaction: we are about to
			// return ErrStoreCountOverQuota, and WithAdvisoryLock rolls the tx
			// back on any non-nil return — which would discard a row written on
			// tx here (#397). Hand it to Execute, which writes it on the pooled
			// handle after the lock is released.
			auditCurrency := buildAuditCurrency(sub)
			blocked := &PlanChangeAuditRow{
				TenantID:        in.TenantID,
				StoreID:         in.StoreID,
				FromPlan:        sub.Plan,
				ToPlan:          in.TargetPlan,
				FromPeriod:      sub.SubscriptionPeriod,
				ToPeriod:        in.TargetPeriod,
				Action:          "downgrade_blocked_over_quota",
				BillingCurrency: auditCurrency,
				Actor:           in.Actor,
				Reason:          in.Reason,
				EffectiveAt:     in.Now,
			}

			if o.deps.Emitter != nil {
				o.deps.Emitter.EmitPlanChange(in.GinCtx, audit.PlanChange{
					TenantID:    in.TenantID,
					StoreID:     in.StoreID,
					FromPlan:    string(sub.Plan),
					ToPlan:      string(in.TargetPlan),
					FromPeriod:  string(sub.SubscriptionPeriod),
					ToPeriod:    string(in.TargetPeriod),
					Subaction:   "downgrade_blocked_over_quota",
					Actor:       in.Actor,
					Reason:      in.Reason,
					EffectiveAt: in.Now,
				})
			}

			return Output{}, blocked, ErrStoreCountOverQuota
		}
	}

	// Determine effective_at: downgrades take effect at current period-end.
	var periodEnd = in.Now // fallback if no period end is recorded
	if sub.CurrentPeriodEnd != nil {
		periodEnd = *sub.CurrentPeriodEnd
	}
	effective, _ := EffectiveAt(DirectionDowngrade, in.Now, periodEnd)

	if err := o.deps.SubscriptionRepo.SetPendingDowngrade(
		ctx, tx,
		in.TenantID, in.StoreID,
		in.TargetPlan, in.TargetPeriod,
		effective, in.Reason,
	); err != nil {
		return Output{}, nil, fmt.Errorf("planchange: set pending downgrade: %w", err)
	}

	auditCurrency := buildAuditCurrency(sub)

	if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
		TenantID:        in.TenantID,
		StoreID:         in.StoreID,
		FromPlan:        sub.Plan,
		ToPlan:          in.TargetPlan,
		FromPeriod:      sub.SubscriptionPeriod,
		ToPeriod:        in.TargetPeriod,
		Action:          "downgrade_scheduled",
		BillingCurrency: auditCurrency,
		Actor:           in.Actor,
		Reason:          in.Reason,
		EffectiveAt:     effective,
	}); err != nil {
		return Output{}, nil, fmt.Errorf("planchange: write downgrade audit row: %w", err)
	}

	if o.deps.Emitter != nil {
		o.deps.Emitter.EmitPlanChange(in.GinCtx, audit.PlanChange{
			TenantID:    in.TenantID,
			StoreID:     in.StoreID,
			FromPlan:    string(sub.Plan),
			ToPlan:      string(in.TargetPlan),
			FromPeriod:  string(sub.SubscriptionPeriod),
			ToPeriod:    string(in.TargetPeriod),
			Subaction:   "downgrade_scheduled",
			Actor:       in.Actor,
			Reason:      in.Reason,
			EffectiveAt: effective,
		})
	}

	return Output{
		Result:        ResultDowngradeScheduled,
		EffectiveAt:   effective,
		StripeUpdated: false,
	}, nil, nil
}

// buildAuditCurrency returns the upper-cased billing currency for audit rows.
// Upper-case is required to satisfy the CHAR(3) NOT NULL constraint with
// canonical ISO-4217 form (e.g. "USD" not "usd"). Falls back to "USD" when
// no currency is recorded on the subscription.
func buildAuditCurrency(sub *subscription.StoreSubscription) string {
	if sub.BillingCurrency != nil && *sub.BillingCurrency != "" {
		return strings.ToUpper(*sub.BillingCurrency)
	}
	return "USD"
}
