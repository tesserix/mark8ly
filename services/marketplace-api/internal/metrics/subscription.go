// Package metrics — SubscriptionCollectors groups all subscription-related
// Prometheus metrics behind a typed struct so emit sites use consistent label
// vocabularies and tests can construct an isolated registry without touching
// the DefaultRegisterer.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SubscriptionCollectors holds every Prometheus metric that is emitted by the
// subscription, billing, promo, and refund subsystems. Use the package-level
// Subscription var for production code; pass a fresh prometheus.NewRegistry()
// to NewSubscriptionCollectors in tests.
type SubscriptionCollectors struct {
	// SubscriptionStateTransitionedTotal counts every successful state-machine
	// transition. Labels: from_status, to_status, reason.
	SubscriptionStateTransitionedTotal *prometheus.CounterVec

	// SubscriptionActiveCountByPlan is a gauge scraped periodically by a
	// collector func and labelled by plan and currency. Updated by a cron or
	// collector — not incremented directly on each request.
	SubscriptionActiveCountByPlan *prometheus.GaugeVec

	// StripeWebhookProcessingDuration observes the handler latency for each
	// Stripe webhook event type. Labels: event_type, status ("ok"|"error").
	StripeWebhookProcessingDuration *prometheus.HistogramVec

	// StripeWebhookFailedTotal counts Stripe webhook handler failures.
	// Labels: event_type, reason ("cas_conflict"|"invalid_transition"|"db"|"stripe_api"|"unknown").
	StripeWebhookFailedTotal *prometheus.CounterVec

	// SubscriptionArbitrageFlaggedTotal counts subscriptions flagged for
	// arbitrage. Labels: reason. Wired to a placeholder; P8 populates.
	SubscriptionArbitrageFlaggedTotal *prometheus.CounterVec

	// PromoAppliedTotal counts promo code applications. Labels: plan, currency,
	// outcome ("applied"|"below_floor"|"expired"|"not_found"|"max_redemptions"|"max_per_email"|"plan_mismatch"|"annual_only"|"unknown").
	PromoAppliedTotal *prometheus.CounterVec

	// RefundIssuedTotal counts successful refund issuances.
	// Labels: currency, reason.
	RefundIssuedTotal *prometheus.CounterVec

	// BillingArchiveCreatedTotal counts billing archive rows created on
	// hard-delete events. No labels — one per store permanently deleted.
	BillingArchiveCreatedTotal prometheus.Counter

	// billing_archive_expiry_soon is emitted by BillingArchiveExpiryCollector
	// in billing_archive_expiry.go (scrape-time query, self-updating). No
	// package-level gauge here — registering both caused a duplicate-descriptor
	// panic at boot (2026-04-19 incident).
}

// NewSubscriptionCollectors constructs and registers all subscription-related
// metrics against reg. Use prometheus.DefaultRegisterer for production and
// prometheus.NewRegistry() for unit tests to avoid registration conflicts.
func NewSubscriptionCollectors(reg prometheus.Registerer) *SubscriptionCollectors {
	c := &SubscriptionCollectors{
		SubscriptionStateTransitionedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "subscription_state_transitioned_total",
				Help:      "Count of subscription state transitions, labelled by from_status, to_status, and reason.",
			},
			[]string{"from_status", "to_status", "reason"},
		),

		SubscriptionActiveCountByPlan: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mark8ly",
				Name:      "subscription_active_count_by_plan",
				Help:      "Instantaneous count of active subscriptions grouped by plan and currency.",
			},
			[]string{"plan", "currency"},
		),

		StripeWebhookProcessingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mark8ly",
				Name:      "stripe_webhook_processing_duration_seconds",
				Help:      "Latency of Stripe webhook handler execution in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"event_type", "status"},
		),

		StripeWebhookFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "stripe_webhook_failed_total",
				Help:      "Count of Stripe webhook handler failures, labelled by event_type and reason.",
			},
			[]string{"event_type", "reason"},
		),

		SubscriptionArbitrageFlaggedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "subscription_arbitrage_flagged_total",
				Help:      "Count of subscriptions flagged for arbitrage. P8 populates this counter.",
			},
			[]string{"reason"},
		),

		PromoAppliedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "subscription_promo_applied_total",
				Help:      "Count of promo code application attempts, labelled by plan, currency, and outcome.",
			},
			[]string{"plan", "currency", "outcome"},
		),

		RefundIssuedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "subscription_refund_issued_total",
				Help:      "Count of successful refund issuances, labelled by currency and reason.",
			},
			[]string{"currency", "reason"},
		),

		BillingArchiveCreatedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "mark8ly",
				Name:      "billing_archive_created_total",
				Help:      "Count of billing archive rows created on store hard-delete events.",
			},
		),

	}

	reg.MustRegister(
		c.SubscriptionStateTransitionedTotal,
		c.SubscriptionActiveCountByPlan,
		c.StripeWebhookProcessingDuration,
		c.StripeWebhookFailedTotal,
		c.SubscriptionArbitrageFlaggedTotal,
		c.PromoAppliedTotal,
		c.RefundIssuedTotal,
		c.BillingArchiveCreatedTotal,
	)

	return c
}

// Subscription is the package-level singleton registered against the default
// Prometheus registry. All production emit sites reference this var.
var Subscription *SubscriptionCollectors

func init() {
	Subscription = NewSubscriptionCollectors(prometheus.DefaultRegisterer)
}
