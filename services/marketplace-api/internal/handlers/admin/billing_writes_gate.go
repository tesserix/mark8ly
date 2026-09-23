package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireBillingWrites aborts with 503 unless billing writes are enabled.
//
// Production has run a live Stripe key since 2026-09-09 (tesserix-k8s
// 6b8f91b7), and the swap is what made price resolution work for the first
// time — so the subscribe flow went from broken to functional. What it was
// not was correct: cancellation was recorded locally and never sent to
// Stripe, so a real subscriber was told access continues to period end, lost
// it at the next FinalizeCron tick, and kept being charged.
//
// Both halves of that are now fixed. Cancellation reaches Stripe through
// cancel.StripeCanceller — and refuses rather than recording a local-only
// cancellation when Stripe cannot be reached — the save offer reverses the
// schedule at Stripe, and current_period_end is written at subscribe time
// from the created subscription instead of waiting on a webhook.
//
// The gate stays closed anyway, because enabling it is a decision and not a
// consequence: it wants an end-to-end run against a live-mode test
// subscription (subscribe, cancel, un-cancel, let a period roll) and a read
// of the merchant-facing copy. Turn it on with BILLING_WRITES_ENABLED once
// that is done. Nothing has been charged to date, and this is what keeps
// that true until someone decides otherwise.
//
// Deliberately fail-closed: the config default is false, so an environment
// that forgets to set BILLING_WRITES_ENABLED blocks writes rather than taking
// money by accident.
func RequireBillingWrites(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error":   "billing_writes_disabled",
			"message": "Subscription changes are temporarily unavailable. No charge has been made. Please contact support if you need this actioned.",
		})
	}
}
