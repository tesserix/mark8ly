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

// ReleaseEmailClaim removes a claim so a later run may retry.
//
// Called ONLY when the send failed because we had no usable address — the
// backfill or a customer.updated webhook may supply one later, and a merchant
// should not permanently lose a notice because their address had not landed
// yet. Every other failure keeps its claim: at-most-once for transport errors
// is deliberate, because a duplicate billing email is worse than a missed one.
func ReleaseEmailClaim(ctx context.Context, db *gorm.DB, subscriptionID uuid.UUID, templateKey, periodKey string) error {
	return db.WithContext(ctx).Exec(`
		DELETE FROM billing_email_sends
		WHERE subscription_id = ? AND template_key = ? AND period_key = ?`,
		subscriptionID, templateKey, periodKey,
	).Error
}
