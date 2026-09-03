package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// TestCarrierSecretCounter_IncrementsRegisteredMetric verifies the shared
// sink wired into all three carriersecrets binaries (cmd/marketplace-api,
// cmd/refund-sweep-cron, cmd/carrier-secrets-backfill) actually increments
// the registered CarrierSecretEventsTotal CounterVec, for each of the three
// event values the carriersecrets package fires.
func TestCarrierSecretCounter_IncrementsRegisteredMetric(t *testing.T) {
	for _, event := range []string{"gsm_fallback_read", "stale_read", "rewrap_failed"} {
		before := testutil.ToFloat64(metrics.CarrierSecretEventsTotal.WithLabelValues(event))

		metrics.CarrierSecretCounter(event, 1)

		after := testutil.ToFloat64(metrics.CarrierSecretEventsTotal.WithLabelValues(event))
		if after != before+1 {
			t.Errorf("event %q: counter = %v, want %v", event, after, before+1)
		}
	}
}
