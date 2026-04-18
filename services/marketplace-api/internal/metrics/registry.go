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
	)
}
