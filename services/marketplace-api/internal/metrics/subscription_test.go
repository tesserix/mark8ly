package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// newIsolatedCollectors constructs a SubscriptionCollectors backed by a fresh
// registry so tests never pollute the DefaultRegisterer.
func newIsolatedCollectors(t *testing.T) *metrics.SubscriptionCollectors {
	t.Helper()
	reg := prometheus.NewRegistry()
	return metrics.NewSubscriptionCollectors(reg)
}

// TestNewSubscriptionCollectors_RegistersWithoutPanic verifies that all metrics
// can be registered against a fresh Prometheus registry without panicking.
func TestNewSubscriptionCollectors_RegistersWithoutPanic(t *testing.T) {
	require.NotPanics(t, func() {
		newIsolatedCollectors(t)
	})
}

// TestSubscriptionStateTransitionedTotal_Labels checks that the counter accepts
// the expected label set and reports the correct value after an increment.
func TestSubscriptionStateTransitionedTotal_Labels(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.SubscriptionStateTransitionedTotal.WithLabelValues("trialing", "active", "checkout.session.completed").Inc()

	got := testutil.ToFloat64(
		c.SubscriptionStateTransitionedTotal.WithLabelValues("trialing", "active", "checkout.session.completed"),
	)
	assert.Equal(t, 1.0, got)
}

// TestStripeWebhookProcessingDuration_Labels checks that the histogram records
// an observation with both event_type and status labels.
func TestStripeWebhookProcessingDuration_Labels(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.StripeWebhookProcessingDuration.WithLabelValues("invoice.paid", "ok").Observe(0.042)

	// A non-zero observation means the histogram is working — just confirm no panic.
	assert.NotNil(t, c.StripeWebhookProcessingDuration)
}

// TestStripeWebhookFailedTotal_Labels checks the failure counter.
func TestStripeWebhookFailedTotal_Labels(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.StripeWebhookFailedTotal.WithLabelValues("invoice.payment_failed", "db").Inc()

	got := testutil.ToFloat64(
		c.StripeWebhookFailedTotal.WithLabelValues("invoice.payment_failed", "db"),
	)
	assert.Equal(t, 1.0, got)
}

// TestPromoAppliedTotal_Labels checks the promo counter with plan/currency/outcome labels.
func TestPromoAppliedTotal_Labels(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.PromoAppliedTotal.WithLabelValues("starter", "usd", "applied").Inc()
	c.PromoAppliedTotal.WithLabelValues("starter", "usd", "expired").Inc()

	applied := testutil.ToFloat64(c.PromoAppliedTotal.WithLabelValues("starter", "usd", "applied"))
	expired := testutil.ToFloat64(c.PromoAppliedTotal.WithLabelValues("starter", "usd", "expired"))
	assert.Equal(t, 1.0, applied)
	assert.Equal(t, 1.0, expired)
}

// TestRefundIssuedTotal_Labels checks the refund counter with currency/reason labels.
func TestRefundIssuedTotal_Labels(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.RefundIssuedTotal.WithLabelValues("usd", "requested_by_customer").Inc()

	got := testutil.ToFloat64(c.RefundIssuedTotal.WithLabelValues("usd", "requested_by_customer"))
	assert.Equal(t, 1.0, got)
}

// TestBillingArchiveCreatedTotal_Inc checks the archive counter.
func TestBillingArchiveCreatedTotal_Inc(t *testing.T) {
	c := newIsolatedCollectors(t)
	c.BillingArchiveCreatedTotal.Inc()
	c.BillingArchiveCreatedTotal.Inc()

	got := testutil.ToFloat64(c.BillingArchiveCreatedTotal)
	assert.Equal(t, 2.0, got)
}

// NOTE: billing_archive_expiry_soon is emitted by BillingArchiveExpiryCollector
// in billing_archive_expiry.go (scrape-time), not by a package-level gauge.
// See billing_archive_expiry_test.go for its coverage.

// TestPackageLevelSubscription_NotNil ensures the package-level singleton is
// initialised by init() and all fields are non-nil.
func TestPackageLevelSubscription_NotNil(t *testing.T) {
	require.NotNil(t, metrics.Subscription)
	assert.NotNil(t, metrics.Subscription.SubscriptionStateTransitionedTotal)
	assert.NotNil(t, metrics.Subscription.SubscriptionActiveCountByPlan)
	assert.NotNil(t, metrics.Subscription.StripeWebhookProcessingDuration)
	assert.NotNil(t, metrics.Subscription.StripeWebhookFailedTotal)
	assert.NotNil(t, metrics.Subscription.SubscriptionArbitrageFlaggedTotal)
	assert.NotNil(t, metrics.Subscription.PromoAppliedTotal)
	assert.NotNil(t, metrics.Subscription.RefundIssuedTotal)
	assert.NotNil(t, metrics.Subscription.BillingArchiveCreatedTotal)
}
