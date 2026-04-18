// Package dunning implements the §16 dunning ladder: automated status
// progression for subscriptions in payment failure states, plus email
// notification crons for past-due and payment-action-required subscribers.
package dunning

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// RefundWindowDays is the §16.5 refund-window length. Merchants within this
// window of their first charge must NOT be auto-dunned into expired.
const RefundWindowDays = 14

// IsInRefundWindow returns true if the subscription's first-charge moment is
// within the last RefundWindowDays. Trial subscriptions (FirstChargeAt nil)
// are NOT in the refund window — they have no charge to refund.
func IsInRefundWindow(sub *subscription.StoreSubscription, now time.Time) bool {
	if sub == nil || sub.FirstChargeAt == nil {
		return false
	}
	return now.Sub(*sub.FirstChargeAt) < RefundWindowDays*24*time.Hour
}
