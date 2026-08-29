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

// ApplyTrialRamp mutates the current-month budget row per spec §5.1, which
// documents the tiers as D1-3 = 500, D4-7 = 2000, D8+ = plan allowance
// (migration 000047's COMMENT on limit_set):
//   - day 4 (transition from D3): limit_set = GREATEST(limit_set, 2000),
//     remaining = GREATEST(remaining, 2000)
//   - day 8 (transition from D7): limit_set = plan_allowance,
//     remaining = GREATEST(remaining, plan_allowance)
//   - all other days: no-op
//
// limit_set is the TIER CEILING and is raised from its own previous value,
// never derived from remaining. Deriving it (`GREATEST(remaining, 2000)`)
// made the ceiling a function of how much the merchant had SPENT: a store
// that had burned its balance down to 100 got its ceiling cut to 2000, while
// an identical store that had spent nothing kept a higher one. Same plan,
// same day, different ceiling (#424).
//
// GREATEST(limit_set, 2000) rather than a flat 2000 so the step can only
// ever RAISE a ceiling. A row seeded at the full allowance before #424 fixed
// the seeding must not be cut to 2000 on day 4 — a merchant's ceiling going
// backwards mid-month is a worse outcome than an over-generous legacy row.
//
// Idempotency: each transition day is applied AT MOST ONCE per budget row,
// enforced by the `ramp_step_applied < N` guard in the WHERE clause. GREATEST
// alone is NOT sufficient — it is a floor, so a re-run after the merchant has
// spent budget would raise the balance back to the ceiling and refund consumed
// spend (#399). The guard is part of the same single atomic UPDATE, so
// concurrent runs on multiple pods still apply the step exactly once.
//
// Uses plangate.Limit to resolve the plan allowance; on plangate.Negotiated
// (Pro — contact sales) the function increments PlanRecomputeWarningTotal and
// leaves limit_set unchanged. Returning nil (not an error) keeps cron green —
// a warning metric is the operational signal.
func ApplyTrialRamp(ctx context.Context, db *gorm.DB, storeID uuid.UUID, day int, plan string) error {
	if !IsRampTransitionDay(day) {
		return nil
	}

	var applied bool
	switch day {
	case 4:
		// Applied at most once: the ramp_step_applied guard, not GREATEST, is
		// what makes this idempotent.
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set         = GREATEST(limit_set, 2000),
			    remaining         = GREATEST(remaining, 2000),
			    ramp_step_applied = 4
			WHERE store_id = $1
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
			  AND ramp_step_applied < 4`
		res := db.WithContext(ctx).Exec(sql, storeID)
		if res.Error != nil {
			return fmt.Errorf("ramp day-4: %w", res.Error)
		}
		applied = res.RowsAffected > 0
	case 8:
		allowance := plangate.Limit(subscription.SubscriptionPlan(plan), plangate.FeatureCampaignEmailsPerMonth)
		if allowance == plangate.Negotiated {
			PlanRecomputeWarningTotal.Inc()
			return nil // leave limit_set unchanged; ops sets it manually
		}
		const sql = `
			UPDATE campaign_email_budget
			SET limit_set         = $1,
			    remaining         = GREATEST(remaining, $1),
			    ramp_step_applied = 8
			WHERE store_id = $2
			  AND month    = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
			  AND ramp_step_applied < 8`
		res := db.WithContext(ctx).Exec(sql, allowance, storeID)
		if res.Error != nil {
			return fmt.Errorf("ramp day-8: %w", res.Error)
		}
		applied = res.RowsAffected > 0
	}
	if applied {
		TrialRampAppliedTotal.WithLabelValues(strconv.Itoa(day)).Inc()
	}
	return nil
}
