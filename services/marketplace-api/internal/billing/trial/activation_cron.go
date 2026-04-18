package trial

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ActivationSpec is the cron expression for the activation cron: 00:30 UTC daily,
// right after the expiry cron at 00:15 UTC.
const ActivationSpec = "30 0 * * *"

// ActivationCron increments the day-30 activation counter once per trialing store
// that hit day 30 today (using created_at as the signup_date proxy), has at least
// one product, and has not already been marked.
//
// Idempotency is guaranteed by the trial_activation_marked_at column: the UPDATE
// includes "AND trial_activation_marked_at IS NULL" so a second run for the same
// store always hits RowsAffected == 0 and skips the counter increment.
type ActivationCron struct {
	db      *gorm.DB
	counter prometheus.Counter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewActivationCron constructs an ActivationCron.
// Nil logger defaults to slog.Default(). Nil clock defaults to time.Now().UTC().
func NewActivationCron(
	db *gorm.DB,
	counter prometheus.Counter,
	logger *slog.Logger,
	clock func() time.Time,
) *ActivationCron {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &ActivationCron{db: db, counter: counter, logger: logger, clock: clock}
}

// Run selects all trialing stores whose created_at falls within the day-30
// window (today - 30d), have at least one product, and have not yet been marked.
// For each qualifying store it stamps trial_activation_marked_at and increments
// the Prometheus counter. Row-level errors are logged and skipped so one bad row
// never blocks the rest.
//
// Both store_subscriptions.store_id and products.store_id are native UUID columns
// in PostgreSQL so no cast is required in the EXISTS subquery.
func (c *ActivationCron) Run(ctx context.Context) error {
	now := c.clock().UTC()
	target := now.AddDate(0, 0, -30)
	dayStart := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := c.db.WithContext(ctx).Raw(`
		SELECT ss.store_id
		FROM store_subscriptions ss
		WHERE ss.status = ?
		  AND ss.created_at >= ? AND ss.created_at < ?
		  AND ss.trial_activation_marked_at IS NULL
		  AND EXISTS (SELECT 1 FROM products p WHERE p.store_id = ss.store_id LIMIT 1)`,
		subscription.StatusTrialing, dayStart, dayEnd,
	).Rows()
	if err != nil {
		return fmt.Errorf("trial activation: query: %w", err)
	}
	defer rows.Close()

	var marked int
	for rows.Next() {
		var storeID uuid.UUID
		if err := rows.Scan(&storeID); err != nil {
			c.logger.Warn("trial activation: scan failed; continuing", "err", err.Error())
			continue
		}
		res := c.db.WithContext(ctx).Exec(`
			UPDATE store_subscriptions
			SET trial_activation_marked_at = now(), updated_at = now()
			WHERE store_id = ? AND trial_activation_marked_at IS NULL`,
			storeID,
		)
		if res.Error != nil {
			c.logger.Warn("trial activation: mark failed; continuing",
				"store_id", storeID.String(), "err", res.Error.Error())
			continue
		}
		if res.RowsAffected == 1 {
			if c.counter != nil {
				c.counter.Inc()
			}
			marked++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("trial activation: rows iter: %w", err)
	}
	if marked > 0 {
		c.logger.Info("trial activation cron marked stores", "count", marked)
	}
	return nil
}
