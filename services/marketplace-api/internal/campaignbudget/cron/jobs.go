// Package cron registers the trial-ramp and monthly-reset jobs with
// robfig/cron/v3. Both jobs are pure idempotent functions of (db, now()) —
// safe to re-run, safe to run on multiple pods concurrently.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	robcron "github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
)

// TrialRampSpec is the cron expression for the trial-ramp job: 00:00 UTC daily.
const TrialRampSpec = "CRON_TZ=UTC 0 0 * * *"

// MonthlyResetSpec is the cron expression for the monthly-reset job:
// 00:05 UTC on the 1st of each month. The 5-minute offset after midnight
// ensures the monthly reset runs after the daily trial ramp on day-1 boundaries.
const MonthlyResetSpec = "CRON_TZ=UTC 5 0 1 * *"

// RegisterTrialRampJob schedules the trial-ramp cron to run at 00:00 UTC daily.
func RegisterTrialRampJob(c *robcron.Cron, db *gorm.DB) (robcron.EntryID, error) {
	return c.AddFunc(TrialRampSpec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := RunTrialRampOnce(ctx, db, time.Now()); err != nil {
			slog.Error("trial ramp cron failed", "err", err)
		}
	})
}

// RegisterMonthlyResetJob schedules the monthly-reset cron for 00:05 UTC on
// the 1st of each month.
func RegisterMonthlyResetJob(c *robcron.Cron, db *gorm.DB) (robcron.EntryID, error) {
	return c.AddFunc(MonthlyResetSpec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := campaignbudget.MonthlyReset(ctx, db); err != nil {
			slog.Error("monthly reset cron failed", "err", err)
		}
	})
}

// RunTrialRampOnce is the exported, testable body of the trial-ramp cron.
// Iterates all trialing subscriptions and applies ApplyTrialRamp using
// ComputeRampDay(created_at, now). Non-transition days are no-ops.
//
// Uses created_at as the signup_date proxy — store_subscriptions has no
// dedicated signup_date column; the row is inserted at signup time so
// created_at is an accurate stand-in (same approach as the activation cron).
func RunTrialRampOnce(ctx context.Context, db *gorm.DB, now time.Time) error {
	type rampRow struct {
		StoreID   string
		Plan      string
		CreatedAt time.Time
	}

	rows, err := db.WithContext(ctx).Raw(`
		SELECT store_id, plan, created_at
		FROM store_subscriptions
		WHERE status IN ('signup', 'trialing')
	`).Rows()
	if err != nil {
		return fmt.Errorf("trial ramp: query subscriptions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r rampRow
		if err := rows.Scan(&r.StoreID, &r.Plan, &r.CreatedAt); err != nil {
			slog.Warn("trial ramp: scan row", "err", err)
			continue
		}
		storeID, err := parseUUID(r.StoreID)
		if err != nil {
			continue
		}
		day := campaignbudget.ComputeRampDay(r.CreatedAt, now)
		if !campaignbudget.IsRampTransitionDay(day) {
			continue
		}
		if err := campaignbudget.ApplyTrialRamp(ctx, db, storeID, day, r.Plan); err != nil {
			slog.Warn("trial ramp: apply failed", "store_id", r.StoreID, "err", err)
		}
	}
	return rows.Err()
}
