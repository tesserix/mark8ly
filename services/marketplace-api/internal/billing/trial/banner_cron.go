package trial

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// BannerSpec is the cron expression for the banner cron: 09:00 UTC daily.
// Early enough for start-of-day in most timezones.
const BannerSpec = "0 9 * * *"

type bannerTarget struct {
	day   int
	state string
}

// bannerTargets defines the three thresholds at which trial banner state advances.
var bannerTargets = []bannerTarget{
	{60, "day_60"},
	{75, "day_75"},
	{85, "day_85"},
}

// BannerCron scans trialing stores at days 60, 75, and 85 of their trial and
// advances the trial_banner_state column so the storefront/admin can surface
// the appropriate urgency banner.
//
// This cron drives the in-app banner only. Trial email is owned by
// internal/subscription/dunning/trial_reminders.go, which sends the
// T-15/T-10/T-7/T-3/T-1 ladder.
type BannerCron struct {
	db     *gorm.DB
	logger *slog.Logger
	clock  func() time.Time
}

// NewBannerCron constructs a BannerCron. If clock is nil, time.Now().UTC() is
// used. If logger is nil, slog.Default() is used.
func NewBannerCron(db *gorm.DB, logger *slog.Logger, clock func() time.Time) *BannerCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BannerCron{db: db, logger: logger, clock: clock}
}

// Run executes one pass over all three banner thresholds. Safe to re-run the
// same UTC day — the CAS on trial_banner_state ensures each row flips at most
// once per threshold.
func (c *BannerCron) Run(ctx context.Context) error {
	for _, t := range bannerTargets {
		if err := c.processDay(ctx, t); err != nil {
			return fmt.Errorf("banner cron day %d: %w", t.day, err)
		}
	}
	return nil
}

func (c *BannerCron) processDay(ctx context.Context, t bannerTarget) error {
	now := c.clock().UTC()
	// signup_date proxy: created_at. Target day boundary = [created_at_day + N, +1).
	targetDay := now.AddDate(0, 0, -t.day)
	dayStart := time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusTrialing).
		Where("stripe_subscription_id IS NULL").
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Where("trial_banner_state IS DISTINCT FROM ?", t.state).
		Find(&rows).Error
	if err != nil {
		return err
	}
	for i := range rows {
		if err := c.processOne(ctx, &rows[i], t); err != nil {
			c.logger.Error("banner cron: row failed; continuing",
				"store_id", rows[i].StoreID.String(), "day", t.day, "err", err)
		}
	}
	return nil
}

func (c *BannerCron) processOne(ctx context.Context, row *subscription.StoreSubscription, t bannerTarget) error {
	return subscription.WithAdvisoryLock(ctx, c.db, row.StoreID, func(tx *gorm.DB) error {
		// CAS on trial_banner_state — exactly-one-writer across pods.
		res := tx.Exec(`
			UPDATE store_subscriptions
			SET trial_banner_state = ?, trial_banner_state_set_at = now(), updated_at = now()
			WHERE store_id = ?
			  AND status = 'trialing'
			  AND stripe_subscription_id IS NULL
			  AND (trial_banner_state IS NULL OR trial_banner_state <> ?)`,
			t.state, row.StoreID, t.state,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // another pod won the race
		}
		// No email is sent here — dunning/trial_reminders.go owns trial mail.
		// Log the advance so ops can see banner state changes in structured logs.
		c.logger.Info("trial banner advanced",
			"store_id", row.StoreID.String(),
			"tenant_id", row.TenantID.String(),
			"banner_state", t.state,
			"day", t.day)
		return nil
	})
}
