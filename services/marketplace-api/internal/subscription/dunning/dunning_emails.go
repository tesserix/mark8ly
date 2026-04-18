package dunning

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// DunningEmailsSpec is the cron expression for the dunning email cron — runs
// at 09:05 UTC daily.
const DunningEmailsSpec = "5 9 * * *"

// CounterVecIncrementer is a narrow interface for a labeled counter so the
// cron can be constructed with a Prometheus CounterVec OR a stub for tests.
// The label is a day string ("day_5", "day_7", etc.).
type CounterVecIncrementer interface {
	WithDay(day string) CounterIncrementer
}

// dunningEmailTarget pairs a day-since-past-due offset with the template to
// send on that day.
type dunningEmailTarget struct {
	Day      int
	Template email.TemplateID
}

var dunningEmailTargets = []dunningEmailTarget{
	{5, email.TemplateDunningDay5},
	{7, email.TemplateDunningDay7},
}

// emailRow is the minimal projection returned by the dunning email query.
type emailRow struct {
	StoreID  string
	TenantID string
}

// SendDunningEmails is a daily cron that sends day-5 and day-7 nudge emails
// to merchants whose subscription entered past_due status N days ago and is
// still past_due. Email routing uses email.Client (NoOpClient until a real
// adapter wires in).
type SendDunningEmails struct {
	db      *gorm.DB
	emailCl email.Client
	logger  *slog.Logger
	clock   func() time.Time
	counter CounterVecIncrementer
}

// NewSendDunningEmails constructs a SendDunningEmails cron. All parameters
// except db and emailCl default safely: nil logger → slog.Default(), nil
// clock → time.Now().UTC(), nil counter → increments are no-ops.
func NewSendDunningEmails(db *gorm.DB, em email.Client, logger *slog.Logger, counter CounterVecIncrementer, clock func() time.Time) *SendDunningEmails {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &SendDunningEmails{
		db:      db,
		emailCl: em,
		logger:  logger,
		counter: counter,
		clock:   clock,
	}
}

// Run executes one pass: for each target day, finds subscriptions that entered
// past_due exactly that many days ago (still currently past_due) and sends the
// corresponding template. Row-level send failures are logged and skipped so
// one bad address never aborts the batch.
func (s *SendDunningEmails) Run(ctx context.Context) error {
	now := s.clock().UTC()
	for _, t := range dunningEmailTargets {
		if err := s.runForDay(ctx, now, t); err != nil {
			s.logger.Error("dunning emails: day batch failed",
				"day", t.Day, "err", err.Error())
			// Continue to next day rather than aborting all.
		}
	}
	return nil
}

func (s *SendDunningEmails) runForDay(ctx context.Context, now time.Time, t dunningEmailTarget) error {
	targetDay := now.AddDate(0, 0, -t.Day)

	// Find subscriptions where the audit_logs entry INTO past_due was exactly
	// N days ago (same calendar day) and the sub is still past_due today.
	// audit.EmitStateTransition stores "to_status" (not "to_state") in metadata.
	var rows []emailRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT DISTINCT
		    ss.store_id,
		    ss.tenant_id
		FROM store_subscriptions ss
		JOIN audit_logs a ON a.store_id = ss.store_id
		WHERE ss.status = ?
		  AND a.action = 'subscription.state_transition'
		  AND a.metadata->>'to_status' = ?
		  AND date_trunc('day', a.created_at) = date_trunc('day', ? ::timestamptz)`,
		string(subscription.StatusPastDue),
		string(subscription.StatusPastDue),
		targetDay,
	).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("dunning emails day %d: query: %w", t.Day, err)
	}

	for _, r := range rows {
		// TODO: wire real merchant email address when StoreSubscription.email
		// column lands. Until then the store_id string is passed as the "to"
		// address; NoOpClient logs intent without performing I/O.
		if err := s.emailCl.Send(ctx, t.Template, r.StoreID, map[string]any{
			"store_id": r.StoreID,
			"day":      t.Day,
		}); err != nil {
			s.logger.Warn("dunning email send failed",
				"day", t.Day, "store_id", r.StoreID, "err", err.Error())
			continue
		}
		if s.counter != nil {
			s.counter.WithDay(fmt.Sprintf("day_%d", t.Day)).Inc()
		}
	}
	return nil
}
