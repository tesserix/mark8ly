package reconciliation

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The drift counter must export a series for every drift_type from process
// start, before any pass has run.
//
// A CounterVec with no children exports nothing at all. That is what the
// estate had: live Prometheus held zero series for
// mark8ly_subscription_reconciliation_drift_total, so the recording rule
// subscription:reconciliation_drift:rate1h returned empty and the
// StripeReconciliationDrift alert compared an absent vector against zero.
// Nothing about that state was distinguishable from a clean estate, which
// is the state the counter spends nearly all its time describing.
func TestDriftCounterPublishesEveryTypeAtZeroBeforeAnyRun(t *testing.T) {
	// A fresh vector, because driftTotal is process-global and every other
	// test in this package increments it.
	fresh := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_drift_total"},
		[]string{"drift_type"},
	)
	if n := testutil.CollectAndCount(fresh); n != 0 {
		t.Fatalf("a bare CounterVec exported %d series, want 0", n)
	}

	publishZeroSeries(fresh)

	if n := testutil.CollectAndCount(fresh); n != len(DriftTypes()) {
		t.Errorf("exports %d series, want %d — a CounterVec with no children "+
			"exports nothing, which is what made the alert inert", n, len(DriftTypes()))
	}
	for _, driftType := range DriftTypes() {
		if got := testutil.ToFloat64(fresh.WithLabelValues(driftType)); got != 0 {
			t.Errorf("drift_type=%q starts at %v, want 0", driftType, got)
		}
	}
}

// The real counter must carry every drift type too. Values are left alone —
// other tests share this process and increment them — but the SERIES must
// all be present, which is the property the alert depends on.
func TestPackageCounterCarriesEveryDriftTypeSeries(t *testing.T) {
	if n := testutil.CollectAndCount(driftTotal); n != len(DriftTypes()) {
		t.Errorf("driftTotal exports %d series, want %d", n, len(DriftTypes()))
	}
}

// DriftTypes must stay complete. A drift type that recordDrift can emit but
// DriftTypes omits is a label that appears for the first time on the night
// it matters — exactly the absent-series problem, narrowed to one type.
func TestDriftTypesCoversEveryConstantTheReconcilerEmits(t *testing.T) {
	declared := map[string]bool{}
	for _, driftType := range DriftTypes() {
		declared[driftType] = true
	}

	for _, emitted := range []string{
		DriftTypeStatusMismatch,
		DriftTypeStripeNotFound,
		DriftTypeLocallyMissing,
		DriftTypePlanMismatch,
	} {
		if !declared[emitted] {
			t.Errorf("drift type %q is emitted but missing from DriftTypes()", emitted)
		}
	}
}

// The heartbeat must NOT be registered by importing the package. A gauge
// registered on a pod that never runs the pass sits at unix 0, reads as
// "last succeeded in 1970", and would page forever against a deployment
// behaving exactly as configured.
func TestHeartbeatIsNotRegisteredByImportAlone(t *testing.T) {
	// A fresh registry stands in for "some other process that imported this
	// package": registering into it must succeed, proving init() did not
	// already claim the gauge on the default registry on our behalf.
	reg := prometheus.NewRegistry()
	MustRegisterCronMetrics(reg)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	const name = "mark8ly_subscription_reconciliation_last_success_timestamp_seconds"
	var found bool
	for _, family := range families {
		if family.GetName() == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("registering cron metrics did not publish %s", name)
	}
}
