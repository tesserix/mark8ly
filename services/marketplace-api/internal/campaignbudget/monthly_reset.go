package campaignbudget

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// MonthlyReset seeds one campaign_email_budget row for each active subscription
// for the current UTC month. Safe to re-run on the same day and across multiple
// pods — ON CONFLICT DO NOTHING ensures only one winner per (store_id, month).
//
// Active statuses that get a row: signup, trialing, active, past_due,
// payment_action_required, cancel_scheduled. Statuses excluded (merchant cannot
// send campaigns): expired, store_closed, pending_hard_delete, hard_deleted.
//
// limit_set + remaining start at the plan allowance, EXCEPT trial, which
// starts at the §5.1 D1-3 tier of 500 and is raised by the trial-ramp cron
// on day 4 (to 2000) and day 8 (to the 5000 trial allowance). Seeding trial
// at the full allowance defeated the ramp entirely: the day-4 step could
// never raise a ceiling that already exceeded it, so a brand-new store had
// its whole month's quota on day one (#424).
//
// Each ramp step applies at most once, guarded by ramp_step_applied (#399).
// Pro subscriptions (plangate.Negotiated) are
// intentionally excluded — ops sets limit_set manually via direct DB update.
func MonthlyReset(ctx context.Context, db *gorm.DB) error {
	// Use a CTE to inline plan allowances without needing a DB function.
	// 'pro' is intentionally omitted — Negotiated ceiling, ops sets manually.
	// 'marketplace' is omitted — internal plan, not subject to campaign gates.
	const sqlWithCTE = `
		WITH allowance(plan, amount) AS (VALUES
			('trial',                      500),
			('starter',                  15000),
			('studio',                   50000)
			-- 'pro' intentionally omitted: Negotiated, operator sets manually
		)
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		SELECT
			ss.store_id,
			date_trunc('month', (now() AT TIME ZONE 'utc'))::date,
			a.amount,
			a.amount
		FROM store_subscriptions ss
		JOIN allowance a ON a.plan = ss.plan
		WHERE ss.status IN (
			'signup', 'trialing', 'active', 'past_due',
			'payment_action_required', 'cancel_scheduled'
		)
		ON CONFLICT (store_id, month) DO NOTHING`

	if err := db.WithContext(ctx).Exec(sqlWithCTE).Error; err != nil {
		return fmt.Errorf("monthly reset: %w", err)
	}
	return nil
}
