package subscription

// email_claim.go — claim-first idempotency for billing mail (#381).
//
// The contract mirrors payment_action_reminders: claim the slot BEFORE
// sending, and never release it on a send failure. That makes delivery
// at-most-once — a transient provider error costs the merchant that one
// notice rather than risking a duplicate. The failure is visible through
// the caller's skipped counter and Warn log.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ClaimEmailSend attempts to claim (subscriptionID, templateKey, periodKey).
//
// Returns true when this caller inserted the row and is therefore the one
// that must send. Returns false when the slot was already claimed — by
// another pod, or by an earlier run of the same cron today.
func ClaimEmailSend(ctx context.Context, db *gorm.DB, subscriptionID uuid.UUID, templateKey, periodKey string, now time.Time) (bool, error) {
	res := db.WithContext(ctx).Exec(`
		INSERT INTO billing_email_sends (subscription_id, template_key, period_key, sent_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		subscriptionID, templateKey, periodKey, now,
	)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
