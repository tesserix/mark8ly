// Package signup hosts signup-related operational code. Task 13 ships the
// anomaly cron here; the full signup-hardening handler lives elsewhere
// (deferred out of P5 — signup itself is a platform-api concern).
package signup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

const (
	// AnomalySpec is the cron expression: 05:00 UTC — 5h buffer for edge rows
	// landing just before midnight.
	AnomalySpec = "0 5 * * *"

	// AnomalyThreshold is the daily signup count above which an alert fires.
	AnomalyThreshold = 50
)

// SlackNotifier is the minimal interface the cron needs. Production wires a
// webhook-backed impl; tests inject a fake. NoOpSlack is a zero-value safe default.
type SlackNotifier interface {
	Send(ctx context.Context, text string) error
}

// NoOpSlack is a no-op SlackNotifier used when no real Slack client is wired.
type NoOpSlack struct{}

// Send satisfies SlackNotifier and does nothing.
func (NoOpSlack) Send(context.Context, string) error { return nil }

// AnomalyCron counts trialing signups created yesterday (UTC) and fires a Slack
// alert when the count exceeds AnomalyThreshold. It is idempotent: the
// signup_anomaly_log table's composite primary key (alert_date, signup_date)
// means a second same-day run inserts nothing (ON CONFLICT DO NOTHING) and
// therefore never double-alerts.
//
// Uses created_at as the signup_date proxy: StoreSubscription has no
// signup_date column; the row is inserted at signup time so created_at is
// equivalent.
type AnomalyCron struct {
	db      *gorm.DB
	slack   SlackNotifier
	counter prometheus.Counter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewAnomalyCron constructs an AnomalyCron.
// Nil slack defaults to NoOpSlack. Nil logger defaults to slog.Default().
// Nil clock defaults to time.Now().UTC().
func NewAnomalyCron(
	db *gorm.DB,
	s SlackNotifier,
	counter prometheus.Counter,
	logger *slog.Logger,
	clock func() time.Time,
) *AnomalyCron {
	if s == nil {
		s = NoOpSlack{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &AnomalyCron{db: db, slack: s, counter: counter, logger: logger, clock: clock}
}

// Run counts trialing signups created yesterday (UTC). When count > AnomalyThreshold
// it inserts an idempotency row into signup_anomaly_log and fires the Slack alert.
// A second same-day run finds RowsAffected == 0 and returns immediately.
func (c *AnomalyCron) Run(ctx context.Context) error {
	now := c.clock().UTC()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	yesterdayEnd := yesterdayStart.Add(24 * time.Hour)

	var count int64
	if err := c.db.WithContext(ctx).Table("store_subscriptions").
		Where("created_at >= ? AND created_at < ?", yesterdayStart, yesterdayEnd).
		Count(&count).Error; err != nil {
		return fmt.Errorf("signup anomaly: count: %w", err)
	}
	if count <= AnomalyThreshold {
		return nil
	}

	res := c.db.WithContext(ctx).Exec(`
		INSERT INTO signup_anomaly_log (alert_date, signup_date, count, created_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT DO NOTHING`,
		now.Format("2006-01-02"), yesterdayStart, count,
	)
	if res.Error != nil {
		return fmt.Errorf("signup anomaly: dedup insert: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Already alerted today — idempotent no-op.
		return nil
	}

	if c.counter != nil {
		c.counter.Inc()
	}
	msg := fmt.Sprintf("[mark8ly] %d signups on %s (threshold %d) — investigate",
		count, yesterdayStart.Format("2006-01-02"), AnomalyThreshold)
	if err := c.slack.Send(ctx, msg); err != nil {
		c.logger.Warn("signup anomaly: slack send failed; alert row persisted",
			"err", err.Error(), "count", count)
	}
	return nil
}
