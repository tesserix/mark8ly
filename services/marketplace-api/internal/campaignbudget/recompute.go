package campaignbudget

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// RecomputeLimitForPlan recalculates limit_set for the current-month row
// based on the new plan. Intended to be called from P4's change-plan
// transaction: pass the caller's *gorm.DB (which may be mid-transaction),
// and this write joins that transaction. On rollback, the budget update
// reverts with the plan row write — state stays consistent.
//
// When plangate.Limit returns plangate.Negotiated (Pro — contact sales),
// this function is a no-op that increments PlanRecomputeWarningTotal and
// returns nil. Ops receives the metric signal and sets limit_set manually.
//
// On upgrade (new_limit > limit_set): remaining is raised to the new ceiling
// so the merchant immediately benefits. On downgrade (new_limit < remaining):
// remaining is clamped to new_limit — you can't keep quota you haven't paid for.
func RecomputeLimitForPlan(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, plan string) error {
	allowance := plangate.Limit(subscription.SubscriptionPlan(plan), plangate.FeatureCampaignEmailsPerMonth)
	if allowance == plangate.Negotiated {
		PlanRecomputeWarningTotal.Inc()
		return nil
	}
	if allowance == plangate.Disabled {
		return fmt.Errorf("recompute: plan %q has disabled campaign emails", plan)
	}

	// Single statement: set limit_set to new allowance, clamp remaining to
	// min(remaining, allowance) on downgrade, raise to allowance on upgrade.
	const sql = `
		UPDATE campaign_email_budget
		SET limit_set = $1,
		    remaining = CASE
		        WHEN remaining > $1 THEN $1
		        ELSE remaining
		    END
		WHERE store_id = $2
		  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date`
	if err := tx.WithContext(ctx).Exec(sql, allowance, storeID).Error; err != nil {
		return fmt.Errorf("recompute limit_set: %w", err)
	}
	return nil
}
