package dunning

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// PaymentActionRemindersSpec is the cron expression for the SCA reminder
// cron — runs at 09:15 UTC daily.
const PaymentActionRemindersSpec = "15 9 * * *"

// paymentActionTarget pairs an offset label with how many days before
// expiry the reminder is sent.
type paymentActionTarget struct {
	OffsetKey  string
	DaysBefore int
}

var paymentActionTargets = []paymentActionTarget{
	{"t_minus_14", 14},
	{"t_minus_7", 7},
	{"t_minus_1", 1},
}

// SendPaymentActionReminders is a daily cron that sends T-14, T-7, and T-1
// reminder emails to merchants whose subscription is in payment_action_required
// status. Idempotency is guaranteed via the payment_action_reminders table
// (INSERT … ON CONFLICT DO NOTHING): only the first pod to insert a row for a
// given (subscription_id, offset_key) pair sends the email.
type SendPaymentActionReminders struct {
	db      *gorm.DB
	emailCl email.Client
	logger  *slog.Logger
	clock   func() time.Time
	counter counterVecIncrementer
}

// NewSendPaymentActionReminders constructs a SendPaymentActionReminders cron.
// All parameters except db and emailCl default safely.
func NewSendPaymentActionReminders(db *gorm.DB, em email.Client, logger *slog.Logger, counter counterVecIncrementer, clock func() time.Time) *SendPaymentActionReminders {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &SendPaymentActionReminders{
		db:      db,
		emailCl: em,
		logger:  logger,
		counter: counter,
		clock:   clock,
	}
}

// Run executes one pass: for each offset (14d/7d/1d), finds subscriptions in
// payment_action_required whose audit transition INTO that status occurred
// exactly DaysBefore days ago, then attempts an idempotency insert before
// sending the email. Row-level failures are logged and skipped.
func (s *SendPaymentActionReminders) Run(ctx context.Context) error {
	now := s.clock().UTC()
	for _, t := range paymentActionTargets {
		if err := s.runForOffset(ctx, now, t); err != nil {
			s.logger.Error("SCA reminders: offset batch failed",
				"offset", t.OffsetKey, "err", err.Error())
			// Continue to next offset rather than aborting all.
		}
	}
	return nil
}

func (s *SendPaymentActionReminders) runForOffset(ctx context.Context, now time.Time, t paymentActionTarget) error {
	targetDay := now.AddDate(0, 0, -t.DaysBefore)

	// Find subscriptions in payment_action_required that have a hosted invoice
	// URL and whose audit entry INTO that status was exactly DaysBefore days ago.
	// audit.EmitStateTransition stores "to_status" (not "to_state") in metadata.
	var rows []subscription.StoreSubscription
	err := s.db.WithContext(ctx).
		Where("status = ?", subscription.StatusPaymentActionRequired).
		Where("hosted_invoice_url IS NOT NULL").
		Where(`store_id IN (
			SELECT store_id FROM audit_logs
			WHERE action = 'subscription.state_transition'
			  AND metadata->>'to_status' = ?
			  AND date_trunc('day', created_at) = date_trunc('day', ? ::timestamptz)
		)`, string(subscription.StatusPaymentActionRequired), targetDay).
		Find(&rows).Error
	if err != nil {
		return err
	}

	for i := range rows {
		if err := s.processOne(ctx, &rows[i], t, now); err != nil {
			s.logger.Error("SCA reminders: row failed; continuing",
				"store_id", rows[i].StoreID.String(),
				"offset", t.OffsetKey,
				"err", err.Error())
		}
	}
	return nil
}

// processOne attempts to claim the idempotency slot for (store_id, offset_key).
// If another pod already inserted the row (RowsAffected == 0), we skip. Only
// the pod that actually inserted sends the email. We never delete the
// idempotency row after a send failure — that would risk a double-send.
func (s *SendPaymentActionReminders) processOne(ctx context.Context, row *subscription.StoreSubscription, t paymentActionTarget, now time.Time) error {
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO payment_action_reminders (subscription_id, offset_key, sent_at)
		VALUES (?, ?, ?)
		ON CONFLICT DO NOTHING`,
		row.StoreID, t.OffsetKey, now,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Another pod (or a prior cron tick) already claimed this slot.
		return nil
	}

	if err := s.emailCl.Send(ctx, email.TemplatePaymentActionReminder, row.StoreID.String(), map[string]any{
		"store_id":           row.StoreID.String(),
		"offset":             t.OffsetKey,
		"hosted_invoice_url": row.HostedInvoiceURL,
	}); err != nil {
		s.logger.Warn("SCA reminder email failed",
			"store_id", row.StoreID.String(), "offset", t.OffsetKey, "err", err.Error())
		// Don't delete the idempotency row — we'd risk double-send on retry.
		return nil
	}

	if s.counter != nil {
		s.counter.WithDay(t.OffsetKey).Inc()
	}
	return nil
}
