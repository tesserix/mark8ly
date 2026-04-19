package lifecycle

import (
	"context"
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

// TemplateWinBack is the email template ID for the day-30 win-back promo.
// The actual template lives in the email provider (SendGrid); the NoOpClient
// logs it until the real adapter is wired.
const TemplateWinBack email.TemplateID = "win_back_day30"

// WinBackCron sends a 20%-off-6-months promo email to expired stores at day 30
// post-expiry (§15.3). It is idempotent by design: stores already past 31 days
// are never selected, so double-runs on the same day produce the same send.
//
// NOTE: The actual promo code attachment (P10 promo service) is deferred.
// Until P10 is wired this cron sends only the notification email via the
// email.NoOpClient facade established in P6.
type WinBackCron struct {
	db      *gorm.DB
	mailer  email.Client
	logger  *slog.Logger
	clock   func() time.Time
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
		c.sendOne(ctx, &rows[i])
	}
	return nil
}

func (c *WinBackCron) sendOne(ctx context.Context, row *subscription.StoreSubscription) {
	// StripeCustomerID is the stable identity for the merchant. Email address
	// is not on StoreSubscription (deferred per P6 comment); the NoOpClient
	// logs the intent and the real adapter will resolve it via Stripe customer lookup.
	err := c.mailer.Send(ctx, TemplateWinBack, row.StripeCustomerID, map[string]any{
		"store_id":     row.StoreID.String(),
		"tenant_id":    row.TenantID.String(),
		"promo":        "20%-off-6-months",
		"note":         "email resolved by adapter from stripe_customer_id",
	})
	if err != nil {
		c.logger.Error("lifecycle: win-back email failed",
			"store_id", row.StoreID, "tenant_id", row.TenantID, "err", err)
		return
	}
	c.logger.Info("lifecycle: win-back email sent (or logged by no-op adapter)",
		"store_id", row.StoreID, "tenant_id", row.TenantID)
}
