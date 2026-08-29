//go:build integration

package dispatch_test

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TestReportArbitrageFailure_LogsAndCounts pins the #423 fix at the call site.
// The recorder error stays non-fatal to the webhook — but it used to be
// discarded into `_ = fmt.Errorf(...)`, producing no log and no metric despite a
// comment claiming it "surfaces in webhook metrics". It does now.
func TestReportArbitrageFailure_LogsAndCounts(t *testing.T) {
	sub := subscription.StoreSubscription{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
	}

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	require.NotNil(t, metrics.Subscription)
	counter := metrics.Subscription.StripeWebhookFailedTotal.
		WithLabelValues("checkout.session.completed", "arbitrage_record")
	before := testutil.ToFloat64(counter)

	dispatch.ReportArbitrageFailureForTest(sub, errors.New("insert arbitrage audit: boom"))

	require.Equal(t, before+1, testutil.ToFloat64(counter),
		"a swallowed arbitrage failure must still increment stripe_webhook_failed_total")

	logged := logBuf.String()
	require.Contains(t, logged, "arbitrage record failed")
	require.Contains(t, logged, sub.ID.String())
	require.Contains(t, logged, sub.TenantID.String())
	require.Contains(t, logged, sub.StoreID.String())
	require.Contains(t, logged, "insert arbitrage audit: boom")
}
