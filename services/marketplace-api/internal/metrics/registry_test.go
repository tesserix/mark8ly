package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// TestCarrierSecretSeries_ZeroInitialised pins that all three event series
// exist on /metrics before anything has fired.
//
// A CounterVec exports nothing for a label value it has never observed, so
// an absent series is ambiguous: "wired and genuinely zero" and "never
// wired, or the code path is unreachable" look identical to a scraper.
// mark8ly#621 retires GCP Secret Manager on the strength of
// gsm_fallback_read reading zero, and that is precisely the ambiguity that
// must not sit under the decision. Pre-declaring the label values turns the
// evidence from "no series" into an affirmative 0.
//
// This must NOT assert via WithLabelValues, which creates the child on
// access and would make the assertion vacuous. Counting collected series is
// the real check.
func TestCarrierSecretSeries_ZeroInitialised(t *testing.T) {
	const wantSeries = 3 // gsm_fallback_read, stale_read, rewrap_failed

	if got := testutil.CollectAndCount(metrics.CarrierSecretEventsTotal); got != wantSeries {
		t.Errorf("carriersecrets_events_total exports %d series, want %d; "+
			"an unexported label value reads as absent rather than zero on /metrics", got, wantSeries)
	}
}

// TestCarrierSecretEvents_MatchConstants guards the one real risk created by
// pre-declaring the event series: the string list in registry.go is a copy of
// the carriersecrets constants, and a rename on one side would silently leave
// the renamed event unexported again — reintroducing exactly the
// absent-vs-zero ambiguity the zero-init exists to remove.
//
// The list is deliberately not imported from carriersecrets (this package
// should not depend on its consumer), so it is pinned by test instead.
func TestCarrierSecretEvents_MatchConstants(t *testing.T) {
	want := map[string]bool{
		carriersecrets.FallbackReadMetric: true,
		carriersecrets.StaleReadMetric:    true,
		carriersecrets.RewrapFailedMetric: true,
	}

	ch := make(chan prometheus.Metric, 16)
	metrics.CarrierSecretEventsTotal.Collect(ch)
	close(ch)

	got := map[string]bool{}
	for m := range ch {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			t.Fatalf("write metric: %v", err)
		}
		for _, lp := range d.Label {
			if lp.GetName() == "event" {
				got[lp.GetValue()] = true
			}
		}
	}

	for event := range want {
		if !got[event] {
			t.Errorf("carriersecrets constant %q has no pre-declared series; "+
				"the list in registry.go has drifted from the constants", event)
		}
	}
	for event := range got {
		if !want[event] {
			t.Errorf("pre-declared series %q matches no carriersecrets constant", event)
		}
	}
}

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
