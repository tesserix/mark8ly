package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireBillingWrites aborts with 503 unless billing writes are enabled.
//
// Production has run a live Stripe key since 2026-09-09 (tesserix-k8s
// 6b8f91b7), and the swap is what made price resolution work for the first
// time — so the subscribe flow went from broken to functional. It is not,
// however, correct: there is no Subscriptions.Cancel call anywhere in this
// service. Cancellation is recorded locally only, which means a real
// subscriber is told access continues to period end, loses it at the next
// FinalizeCron tick, and keeps being charged by Stripe.
//
// Nothing has been charged yet (0 live customers, subscriptions, invoices and
// charges at 2026-09-21), so this gate is what keeps that true. Remove it only
// once cancellation reaches Stripe and current_period_end is persisted at
// subscribe time — not merely once the key is judged safe.
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
