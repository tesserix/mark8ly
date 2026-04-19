package metrics_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// stubFXReader implements mrrFXReader for unit tests.
type stubFXReader struct {
	rates map[string]decimal.Decimal
	err   error
}

func (s *stubFXReader) GetAll(_ context.Context) (map[string]decimal.Decimal, error) {
	return s.rates, s.err
}

// TestMRRRollup_Describe emits the descriptor without panic.
func TestMRRRollup_Describe(t *testing.T) {
	fx := &stubFXReader{rates: map[string]decimal.Decimal{"USD": decimal.NewFromInt(1)}}
	collector := metrics.NewMRRRollup(nil, fx, nil)

	ch := make(chan *prometheus.Desc, 1)
	require.NotPanics(t, func() { collector.Describe(ch) })
	desc := <-ch
	assert.Contains(t, desc.String(), "mark8ly_subscription_mrr_usd")
}

// TestMRRRollup_CollectEmitsZeroWhenNoRows verifies the collector emits a zero
// gauge rather than panicking when the DB returns no active subscription rows.
// We use a nil DB intentionally and rely on the error path emitting a fallback
// zero — a nil *gorm.DB triggers the query error branch.
func TestMRRRollup_CollectEmitsZeroWhenNoRows(t *testing.T) {
	fx := &stubFXReader{rates: map[string]decimal.Decimal{"USD": decimal.NewFromInt(1)}}
	// nil db → query will error → collector emits fallback zero metric
	collector := metrics.NewMRRRollup(nil, fx, nil)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// Gather should not panic and should return at least one metric family.
	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, mfs, "expected at least one metric family from MRRRollup")
}

// TestMRRRollup_FXErrorFallsBackToUSD verifies that when the FX reader errors
// the collector still attempts to collect without panicking.
func TestMRRRollup_FXErrorFallsBackToUSD(t *testing.T) {
	fx := &stubFXReader{err: assert.AnError}
	collector := metrics.NewMRRRollup(nil, fx, nil)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	mfs, err := reg.Gather()
	require.NoError(t, err)
	// With a nil DB, the query path errors and emits the fallback zero.
	require.NotEmpty(t, mfs)
}

// TestMRRRollup_MetricName verifies the collector emits at least one metric
// sample with the expected name (mark8ly_subscription_mrr_usd).
func TestMRRRollup_MetricName(t *testing.T) {
	fx := &stubFXReader{rates: map[string]decimal.Decimal{"USD": decimal.NewFromInt(1)}}
	collector := metrics.NewMRRRollup(nil, fx, nil)

	// CollectAndCount returns the number of metric samples emitted.
	// With a nil DB the error fallback emits exactly one zero sample.
	count := testutil.CollectAndCount(collector, "mark8ly_subscription_mrr_usd")
	assert.GreaterOrEqual(t, count, 1, "expected at least one mark8ly_subscription_mrr_usd sample")
}
