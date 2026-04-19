package campaignbudget

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ComputeRampDay returns the 1-indexed trial day for (signupDate, now).
// Day boundaries are UTC midnight — the cron runs at 00:00 UTC so a store
// that signed up at 2026-04-01T12:00Z is on day 1 until 2026-04-02T00:00Z
// then day 2 until 2026-04-03T00:00Z, etc.
//
// The function is pure and deterministic. Clock skew before signup clamps
// to day 1 rather than returning 0 or negative — the cron must never mistake
// pre-signup clock skew for "time to apply the plan allowance".
func ComputeRampDay(signupDate, now time.Time) int {
	s := time.Date(signupDate.UTC().Year(), signupDate.UTC().Month(), signupDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	diffDays := int(n.Sub(s).Hours() / 24)
	if diffDays < 0 {
		return 1
	}
	return diffDays + 1
}

// IsRampTransitionDay reports whether day is one of the two transition days
// where the cron must mutate limit_set (day 3→4 or day 7→8). On all other
// days the cron short-circuits for this store.
func IsRampTransitionDay(day int) bool {
	return day == 4 || day == 8
}

// ApplyTrialRamp mutates the current-month budget row per spec §5.1:
//   - day 4 (transition from D3): limit_set = GREATEST(remaining, 2000),
//     remaining = limit_set
//   - day 8 (transition from D7): limit_set = plan_allowance,
//     remaining = GREATEST(remaining, plan_allowance)
//   - all other days: no-op
//
// Idempotency: re-running on the same transition day with a smaller remaining
// uses GREATEST semantics so consumed balance is never re-inflated. The UPDATE
// is a single atomic statement; concurrent runs produce the same result.
//
// Uses plangate.Limit to resolve the plan allowance; on plangate.Negotiated
// (Pro — contact sales) the function increments PlanRecomputeWarningTotal and
// leaves limit_set unchanged. Returning nil (not an error) keeps cron green —
// a warning metric is the operational signal.
func ApplyTrialRamp(ctx context.Context, db *gorm.DB, storeID uuid.UUID, day int, plan string) error {
	if !IsRampTransitionDay(day) {
		return nil
	}

	switch day {
	case 4:
		// Single SQL does GREATEST(remaining, 2000) atomically.
		// Idempotent: if remaining already >= 2000, no change.
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set  = GREATEST(remaining, 2000),
			    remaining  = GREATEST(remaining, 2000)
			WHERE store_id = $1
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date`
		if err := db.WithContext(ctx).Exec(sql, storeID).Error; err != nil {
			return fmt.Errorf("ramp day-4: %w", err)
		}
	case 8:
		allowance := plangate.Limit(subscription.SubscriptionPlan(plan), plangate.FeatureCampaignEmailsPerMonth)
		if allowance == plangate.Negotiated {
			PlanRecomputeWarningTotal.Inc()
			return nil // leave limit_set unchanged; ops sets it manually
		}
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set = $1,
			    remaining = GREATEST(remaining, $1)
			WHERE store_id = $2
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date`
		if err := db.WithContext(ctx).Exec(sql, allowance, storeID).Error; err != nil {
			return fmt.Errorf("ramp day-8: %w", err)
		}
	}
	TrialRampAppliedTotal.WithLabelValues(strconv.Itoa(day)).Inc()
	return nil
}
