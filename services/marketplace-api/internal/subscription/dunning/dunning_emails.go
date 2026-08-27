package dunning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
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
// Email and StoreName come from the merchant-facing side of the join: the
// address to send to, and the name the templates address them by.
type emailRow struct {
	SubscriptionID   uuid.UUID
	StoreID          string
	TenantID         string
	Email            *string
	StoreName        string
	HostedInvoiceURL *string
}

// SendDunningEmails is a daily cron that sends day-5 and day-7 nudge emails
// to merchants whose subscription entered past_due status N days ago and is
// still past_due. Email routing goes through email.Client — since #381 that
// is the real template client (render → SendGrid → Resend), not a no-op.
type SendDunningEmails struct {
	db      *gorm.DB
	emailCl email.Client
	logger  *slog.Logger
	clock   func() time.Time
	counter CounterVecIncrementer
	skip    SkipCounter
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

// WithSkipCounter attaches the counter for emails deliberately not sent.
// Optional: nil means skips are logged but not counted.
func (s *SendDunningEmails) WithSkipCounter(c SkipCounter) *SendDunningEmails {
	s.skip = c
	return s
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
		    ss.id   AS subscription_id,
		    ss.store_id,
		    ss.tenant_id,
		    ss.email,
		    ss.hosted_invoice_url,
		    COALESCE(st.name, 'your store') AS store_name
		FROM store_subscriptions ss
		JOIN audit_logs a ON a.store_id = ss.store_id
		LEFT JOIN stores st ON st.id = ss.store_id
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

	dayLabel := fmt.Sprintf("day_%d", t.Day)
	periodKey := targetDay.Format("2006-01-02")

	for _, r := range rows {
		// Claim before sending. Dunning re-derives eligibility from
		// audit_logs on every run, so without this a second run on the
		// same day re-sends to the same merchants (#381).
		won, err := subscription.ClaimEmailSend(ctx, s.db, r.SubscriptionID, string(t.Template), periodKey, now)
		if err != nil {
			s.logger.Error("dunning email: claim failed; skipping row",
				"day", t.Day, "store_id", r.StoreID, "err", err.Error())
			continue
		}
		if !won {
			continue // already claimed by another pod or an earlier run
		}

		to := ""
		if r.Email != nil {
			to = *r.Email
		}
		invoiceURL := ""
		if r.HostedInvoiceURL != nil {
			invoiceURL = *r.HostedInvoiceURL
		}

		if err := s.emailCl.Send(ctx, t.Template, to, map[string]any{
			"store_id":           r.StoreID,
			"tenant_id":          r.TenantID,
			"store_name":         r.StoreName,
			"day":                t.Day,
			"hosted_invoice_url": invoiceURL,
		}); err != nil {
			// Never increment the sent counter here. Before #381 this
			// branch was unreachable because the client was a no-op that
			// always returned nil, so the counter reported deliveries
			// that never happened.
			s.logger.Warn("dunning email not sent",
				"day", t.Day, "store_id", r.StoreID,
				"reason", email.SkipReason(err), "err", err.Error())
			if s.skip != nil {
				s.skip.WithTemplateReason(string(t.Template), email.SkipReason(err)).Inc()
			}
			if errors.Is(err, email.ErrUndeliverable) {
				// The address is missing or wrong — recoverable via the
				// backfill or a customer.updated webhook. Release the
				// claim so a later run can still deliver this notice.
				if relErr := subscription.ReleaseEmailClaim(ctx, s.db, r.SubscriptionID, string(t.Template), periodKey); relErr != nil {
					s.logger.Error("dunning email: release claim failed",
						"day", t.Day, "store_id", r.StoreID, "err", relErr.Error())
				} else {
					s.logger.Info("dunning email: claim released for retry",
						"day", t.Day, "store_id", r.StoreID)
				}
			}
			continue
		}
		if s.counter != nil {
			s.counter.WithDay(dayLabel).Inc()
		}
	}
	return nil
}
