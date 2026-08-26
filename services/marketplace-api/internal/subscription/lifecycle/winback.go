package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// WinBackSpec is the cron expression for the day-30 win-back email: 10:00 UTC daily.
const WinBackSpec = "0 10 * * *"

// winBack30Days is the post-expiry window after which the win-back promo is sent.
const winBack30Days = 30 * 24 * time.Hour

// WinBackCron sends a 20%-off-6-months promo email to expired stores at day 30
// post-expiry (§15.3).
//
// Idempotence comes from the billing_email_sends claim, NOT from the window
// query. The window selects the same rows on every run within the same day —
// before #381 the comment here claimed that was idempotent, which it was only
// because the client was a no-op that never sent anything.
//
// NOTE: The actual promo code attachment (P10 promo service) is deferred.
type WinBackCron struct {
	db     *gorm.DB
	mailer email.Client
	logger *slog.Logger
	clock  func() time.Time
	skip   SkipCounter
	sent   SentCounter
}

// CounterIncrementer is a one-method counter so tests can stub it.
type CounterIncrementer interface{ Inc() }

// SkipCounter counts win-back emails deliberately not sent, labeled by
// template and reason. Declared here rather than imported from the dunning
// package so lifecycle keeps its current dependency set.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// SentCounter counts win-back emails actually delivered, labeled by
// template. Without it there would be no sent counter for win_back_day30 at
// all, and the sent+skipped identity documented on
// metrics.BillingEmailsSkippedTotal would be false for this template.
type SentCounter interface {
	WithTemplate(template string) CounterIncrementer
}

// NewWinBackCron constructs a WinBackCron.
func NewWinBackCron(db *gorm.DB, mailer email.Client, logger *slog.Logger, clock func() time.Time) *WinBackCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WinBackCron{db: db, mailer: mailer, logger: logger, clock: clock}
}

// WithSkipCounter attaches the skipped-delivery counter. Optional.
func (c *WinBackCron) WithSkipCounter(sc SkipCounter) *WinBackCron {
	c.skip = sc
	return c
}

// WithSentCounter attaches the delivered-email counter. Optional.
func (c *WinBackCron) WithSentCounter(sc SentCounter) *WinBackCron {
	c.sent = sc
	return c
}

// Run selects expired subscriptions whose updated_at is exactly in the 30-day
// window (between 30 and 31 days post-expiry) and sends the promo email.
func (c *WinBackCron) Run(ctx context.Context) error {
	now := c.clock().UTC()
	windowStart := now.Add(-31 * 24 * time.Hour)
	windowEnd := now.Add(-winBack30Days)

	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusExpired).
		Where("updated_at > ? AND updated_at <= ?", windowStart, windowEnd).
		Find(&rows).Error
	if err != nil {
		return err
	}
	c.logger.Info("lifecycle: win-back cron started", "eligible", len(rows))
	for i := range rows {
		c.sendOne(ctx, &rows[i], now)
	}
	return nil
}

func (c *WinBackCron) sendOne(ctx context.Context, row *subscription.StoreSubscription, now time.Time) {
	// The period key is anchored to the row's own updated_at, not to the
	// cron's wall-clock windowStart. Eligibility here is a sliding window,
	// so two runs (e.g. a missed run followed by a catch-up near a UTC day
	// boundary) can both select the same row with different windowStart
	// values. A wall-clock key would then differ between the two runs and
	// the claim would let both through — two win-back emails. Deriving the
	// key from row.UpdatedAt guarantees any two runs that select this row
	// agree on the same key, so the second claim always loses.
	periodKey := row.UpdatedAt.UTC().Format("2006-01-02")
	won, err := subscription.ClaimEmailSend(ctx, c.db, row.ID, string(email.TemplateWinBack), periodKey, now)
	if err != nil {
		c.logger.Error("lifecycle: win-back claim failed; skipping",
			"store_id", row.StoreID, "err", err.Error())
		if c.skip != nil {
			c.skip.WithTemplateReason(string(email.TemplateWinBack), "claim_failed").Inc()
		}
		return
	}
	if !won {
		return // already sent for this window
	}

	to := ""
	if row.Email != nil {
		to = *row.Email
	}

	err = c.mailer.Send(ctx, email.TemplateWinBack, to, map[string]any{
		"store_id":   row.StoreID.String(),
		"tenant_id":  row.TenantID.String(),
		"store_name": subscription.StoreNameFor(ctx, c.db, row.StoreID),
		"promo":      "20%-off-6-months",
	})
	if err != nil {
		c.logger.Warn("lifecycle: win-back email not sent",
			"store_id", row.StoreID, "tenant_id", row.TenantID,
			"reason", email.SkipReason(err), "err", err.Error())
		if c.skip != nil {
			c.skip.WithTemplateReason(string(email.TemplateWinBack), email.SkipReason(err)).Inc()
		}
		if errors.Is(err, email.ErrUndeliverable) {
			// The address is missing or wrong — recoverable via the
			// backfill or a customer.updated webhook. Release the claim
			// so a later run can still deliver this notice.
			if relErr := subscription.ReleaseEmailClaim(ctx, c.db, row.ID, string(email.TemplateWinBack), periodKey); relErr != nil {
				c.logger.Error("lifecycle: win-back release claim failed",
					"store_id", row.StoreID, "err", relErr.Error())
			} else {
				c.logger.Info("lifecycle: win-back claim released for retry",
					"store_id", row.StoreID)
			}
		}
		return
	}
	if c.sent != nil {
		c.sent.WithTemplate(string(email.TemplateWinBack)).Inc()
	}
	c.logger.Info("lifecycle: win-back email sent",
		"store_id", row.StoreID, "tenant_id", row.TenantID)
}
