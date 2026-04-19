// Package metrics provides Prometheus metric definitions for marketplace-api.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// HTTPRequestsTotal counts HTTP requests by method, path, and status.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks HTTP request latency in seconds.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// OrdersCreatedTotal counts orders created by store.
	OrdersCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "orders_created_total",
			Help: "Total orders created.",
		},
		[]string{"store_id"},
	)

	// WebhookReceivedTotal counts webhook events by provider and type.
	WebhookReceivedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_received_total",
			Help: "Total webhook events received.",
		},
		[]string{"provider", "event_type"},
	)

	// OutboxEventsPending tracks the current outbox queue depth.
	OutboxEventsPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "outbox_events_pending",
			Help: "Number of pending outbox events.",
		},
	)

	// OutboxEventsPublishedTotal counts outbox events published.
	OutboxEventsPublishedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "outbox_events_published_total",
			Help: "Total outbox events published.",
		},
	)

	// TrialSignupAnomalyAlertsTotal counts days where yesterday's signup count
	// exceeded the anomaly threshold.
	TrialSignupAnomalyAlertsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mark8ly_trial_signup_anomaly_alerts_total",
		Help: "Count of days where yesterday's signup count exceeded the anomaly threshold.",
	})

	// TrialActivationDay30Total counts trialing stores that reached day 30 with
	// at least one product.
	TrialActivationDay30Total = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mark8ly_trial_activation_day30_total",
		Help: "Count of trialing stores that reached day 30 with at least one product.",
	})

	// DunningEmailsSentTotal counts dunning nudge emails sent, labeled by day
	// (day_5, day_7). P6 dunning metrics.
	DunningEmailsSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_subscription_dunning_emails_sent_total",
			Help: "Count of dunning nudge emails sent, labeled by day (5, 7).",
		},
		[]string{"day"},
	)

	// PaymentActionRemindersSentTotal counts SCA reminders sent, labeled by
	// offset (t_minus_14, t_minus_7, t_minus_1). P6 dunning metrics.
	PaymentActionRemindersSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_subscription_payment_action_reminders_sent_total",
			Help: "Count of SCA reminders sent, labeled by offset (t_minus_14, t_minus_7, t_minus_1).",
		},
		[]string{"offset"},
	)

	// DunningSuppressedRefundWindowTotal counts past_due→expired ladder steps
	// skipped due to the 14-day refund window (§16.5). P6 dunning metrics.
	DunningSuppressedRefundWindowTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mark8ly_subscription_dunning_suppressed_refund_window_total",
			Help: "Count of past_due→expired ladder steps skipped due to the 14-day refund window (§16.5).",
		},
	)

	// APIKeyUsedTotal counts successful public-API authentications, labeled by
	// the auth path that resolved (cache_hit | cold_lookup). P14 §18.4.
	APIKeyUsedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_apikey_used_total",
			Help: "Count of successful public-API key authentications.",
		},
		[]string{"path"},
	)

	// APIKeyAuthFailedTotal counts unsuccessful authentications, labeled by
	// reason (missing_bearer | wrong_prefix | unknown_key | revoked).
	APIKeyAuthFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_apikey_auth_failed_total",
			Help: "Count of failed public-API key authentications.",
		},
		[]string{"reason"},
	)

	// APIKeyRateLimitedTotal counts rate-limit-exceeded responses on the
	// public R/W API, keyed only by API-key id label class (high-cardinality
	// avoidance: bucket by which key tier triggered).
	APIKeyRateLimitedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_apikey_rate_limited_total",
			Help: "Count of rate-limit-exceeded responses on the public R/W API.",
		},
		[]string{"tier"},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		OrdersCreatedTotal,
		WebhookReceivedTotal,
		OutboxEventsPending,
		OutboxEventsPublishedTotal,
		TrialSignupAnomalyAlertsTotal,
		TrialActivationDay30Total,
		DunningEmailsSentTotal,
		PaymentActionRemindersSentTotal,
		DunningSuppressedRefundWindowTotal,
		APIKeyUsedTotal,
		APIKeyAuthFailedTotal,
		APIKeyRateLimitedTotal,
	)
}
