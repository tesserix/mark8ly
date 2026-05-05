package dunning

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TrialRemindersSpec is the cron expression for the trial reminder cron —
// runs at 09:30 UTC daily, offset from the SCA reminder cron (09:15) so the
// two never serialise on the same audit-log scan.
const TrialRemindersSpec = "30 9 * * *"

// trialReminderTarget pairs a stable offset_key (used for idempotency and
// telemetry) with the days-before-expiry the reminder fires on, the email
// template, and the payment-method state it applies to.
type trialReminderTarget struct {
	OffsetKey   string
	DaysBefore  int
	Template    email.TemplateID
	HasPM       bool // when true, only fires for rows with has_default_payment_method=true
	Description string
}

// trialReminderTargets encodes the spec:
//
//   - No payment method on file: nudge the merchant 5 times during the final
//     15 days, escalating in tone, asking them to add a card / pick a plan.
//   - Has payment method on file: a single heads-up the day before Stripe
//     auto-bills the chosen plan.
//
// Order is purely cosmetic; each target's idempotency slot is independent.
var trialReminderTargets = []trialReminderTarget{
	{"no_pm_t_minus_15", 15, email.TemplateTrialNoPMT15, false, "trial ends in 15 days; add a card"},
	{"no_pm_t_minus_10", 10, email.TemplateTrialNoPMT10, false, "trial ends in 10 days; add a card"},
	{"no_pm_t_minus_7", 7, email.TemplateTrialNoPMT7, false, "trial ends in 7 days; add a card"},
	{"no_pm_t_minus_3", 3, email.TemplateTrialNoPMT3, false, "trial ends in 3 days; add a card"},
	{"no_pm_t_minus_1", 1, email.TemplateTrialNoPMT1, false, "trial ends tomorrow; final nudge"},
	{"has_pm_t_minus_1", 1, email.TemplateTrialHasPMT1, true, "trial ends tomorrow; plan auto-starts"},
}

// SendTrialReminders is a daily cron that emails merchants approaching the
// 90-day trial boundary. Cadence depends on whether the merchant has a
// default payment method on file (mirrored onto store_subscriptions by the
// customer.updated webhook handler — see migration 087):
//
//   - Without a card → T-15, T-10, T-7, T-3, T-1 nudges asking them to add
//     billing details or pick a plan.
//   - With a card    → T-1 heads-up before Stripe auto-bills the chosen plan.
//
// Idempotency is guaranteed via the trial_reminders table
// (INSERT … ON CONFLICT DO NOTHING) keyed by (subscription_id, offset_key).
// Multi-pod safety: only the first inserter sends; later attempts no-op.
//
// Multi-tenant: every query is keyed by subscription_id (which carries tenant_id
// and store_id transitively). Reminder rows persist tenant_id/store_id alongside
// for cheap per-tenant audits without needing a join.
type SendTrialReminders struct {
	db      *gorm.DB
	emailCl email.Client
	logger  *slog.Logger
	clock   func() time.Time
	counter CounterVecIncrementer
}

// NewSendTrialReminders constructs a SendTrialReminders cron. db and emailCl
// are required; logger, counter and clock default safely.
func NewSendTrialReminders(db *gorm.DB, em email.Client, logger *slog.Logger, counter CounterVecIncrementer, clock func() time.Time) *SendTrialReminders {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &SendTrialReminders{
		db:      db,
		emailCl: em,
		logger:  logger,
		counter: counter,
		clock:   clock,
	}
}

// Run executes one pass through every reminder target. Per-offset failures
// are logged and skipped so one bad offset never blocks the rest.
func (s *SendTrialReminders) Run(ctx context.Context) error {
	now := s.clock().UTC()
	for _, t := range trialReminderTargets {
		if err := s.runForOffset(ctx, now, t); err != nil {
			s.logger.Error("trial reminders: offset batch failed",
				"offset", t.OffsetKey, "err", err.Error())
			// Continue to next offset rather than aborting all.
		}
	}
	return nil
}

func (s *SendTrialReminders) runForOffset(ctx context.Context, now time.Time, t trialReminderTarget) error {
	// Target subscriptions whose trial expiry (created_at + TrialDays) is
	// exactly DaysBefore days from now — equivalently, those created
	// (TrialDays - DaysBefore) days ago.
	dayOffset := trial.TrialDays - t.DaysBefore
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -dayOffset)
	dayEnd := dayStart.AddDate(0, 0, 1)

	var rows []subscription.StoreSubscription
	err := s.db.WithContext(ctx).
		Where("status IN ?", []subscription.SubscriptionStatus{
			subscription.StatusSignup,
			subscription.StatusTrialing,
		}).
		Where("has_default_payment_method = ?", t.HasPM).
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Find(&rows).Error
	if err != nil {
		return err
	}

	for i := range rows {
		if err := s.processOne(ctx, &rows[i], t, now); err != nil {
			s.logger.Error("trial reminders: row failed; continuing",
				"store_id", rows[i].StoreID.String(),
				"tenant_id", rows[i].TenantID.String(),
				"offset", t.OffsetKey,
				"err", err.Error())
		}
	}
	return nil
}

// processOne claims the (subscription_id, offset_key) idempotency slot. If
// another pod or prior tick already inserted the row (RowsAffected==0), we
// skip without sending. Email-send failures intentionally do NOT delete the
// idempotency row — that would risk a double-send on the next tick.
func (s *SendTrialReminders) processOne(ctx context.Context, row *subscription.StoreSubscription, t trialReminderTarget, now time.Time) error {
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		row.ID, row.TenantID, row.StoreID, t.OffsetKey, now,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Slot already claimed — another pod, or a prior cron tick.
		return nil
	}

	// TODO: trial reminder email recipient — the StoreSubscription row does
	// not yet carry an email/store_name pair. Mirroring the placeholder used
	// by trial.ExpiryCron and SendPaymentActionReminders, we pass StoreID as
	// the recipient string for now; the real recipient is resolved by the
	// email adapter via tenant lookup. Revisit when the columns land.
	if err := s.emailCl.Send(ctx, t.Template, row.StoreID.String(), map[string]any{
		"store_id":           row.StoreID.String(),
		"tenant_id":          row.TenantID.String(),
		"offset":             t.OffsetKey,
		"days_remaining":     t.DaysBefore,
		"has_payment_method": t.HasPM,
		"plan":               string(row.Plan),
	}); err != nil {
		s.logger.Warn("trial reminder email failed",
			"store_id", row.StoreID.String(),
			"offset", t.OffsetKey,
			"err", err.Error())
		// Do not delete the idempotency row — preserves at-most-once semantics.
		return nil
	}

	if s.counter != nil {
		s.counter.WithDay(t.OffsetKey).Inc()
	}
	return nil
}
