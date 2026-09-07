package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubGroupReader returns canned aggregation rows, standing in for Postgres.
type stubGroupReader struct {
	groups []mrrGroup
	err    error
}

func (s stubGroupReader) activeSubscriptionGroups(context.Context) ([]mrrGroup, error) {
	return s.groups, s.err
}

type stubRates struct {
	rates map[string]decimal.Decimal
	err   error
}

func (s stubRates) GetAll(context.Context) (map[string]decimal.Decimal, error) {
	return s.rates, s.err
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// gather collects the emitted samples as plan/currency -> value.
func gather(t *testing.T, groups []mrrGroup, readErr error, rates map[string]decimal.Decimal) map[[2]string]float64 {
	t.Helper()
	c := newMRRRollupWithReader(
		stubGroupReader{groups: groups, err: readErr},
		stubRates{rates: rates},
		quietLogger(),
	)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err, "gather must not error (duplicate label sets would surface here)")
	require.Len(t, mfs, 1)
	require.Equal(t, "mark8ly_subscription_mrr_usd", mfs[0].GetName())

	out := make(map[[2]string]float64)
	for _, mtr := range mfs[0].GetMetric() {
		var plan, currency string
		for _, l := range mtr.GetLabel() {
			switch l.GetName() {
			case "plan":
				plan = l.GetValue()
			case "currency":
				currency = l.GetValue()
			}
		}
		require.NotNil(t, mtr.GetGauge(), "expected a gauge sample")
		out[[2]string{plan, currency}] = mtr.GetGauge().GetValue()
	}
	return out
}

var usdOnly = map[string]decimal.Decimal{"USD": decimal.NewFromInt(1)}

// A monthly plan bills its catalog price once per subscription.
// starter/monthly/usd = 1900 minor = $19.00 × 3 = $57.00.
func TestMRRRollup_MonthlyPlan(t *testing.T) {
	got := gather(t, []mrrGroup{
		{Plan: "starter", Period: "monthly", Currency: "USD", Subscriptions: 3},
	}, nil, usdOnly)

	require.Len(t, got, 1)
	assert.InDelta(t, 57.0, got[[2]string{"starter", "USD"}], 1e-9)
}

// An annual plan must be normalised to a monthly figure: starter/annual/usd
// = 18200 minor = $182.00/yr → $15.1666…/mo × 2 = $30.3333….
// Asserting the un-normalised $364.00 would be the bug this guards.
func TestMRRRollup_AnnualPlanNormalisedToMonthly(t *testing.T) {
	got := gather(t, []mrrGroup{
		{Plan: "starter", Period: "annual", Currency: "USD", Subscriptions: 2},
	}, nil, usdOnly)

	require.Len(t, got, 1)
	assert.InDelta(t, 182.0*2/12, got[[2]string{"starter", "USD"}], 1e-9)
	assert.Less(t, got[[2]string{"starter", "USD"}], 40.0, "must not be the un-normalised $364 annual total")
}

// Monthly and annual rows for the same plan+currency collapse into one series
// (the metric has no period label) and must be summed, not emitted twice.
// $19.00 × 1 + $182.00/12 × 1 = 19 + 15.1666… = 34.1666….
func TestMRRRollup_MonthlyAndAnnualShareOneSeries(t *testing.T) {
	got := gather(t, []mrrGroup{
		{Plan: "starter", Period: "monthly", Currency: "USD", Subscriptions: 1},
		{Plan: "starter", Period: "annual", Currency: "USD", Subscriptions: 1},
	}, nil, usdOnly)

	require.Len(t, got, 1)
	assert.InDelta(t, 19.0+182.0/12, got[[2]string{"starter", "USD"}], 1e-9)
}

// Two currencies stay in separate buckets and are never added together.
// USD: $19.00 × 1 = 19.00. INR: ₹999.00 × 1 × 0.012 = 11.988 USD.
func TestMRRRollup_CurrenciesStayInSeparateBuckets(t *testing.T) {
	rates := map[string]decimal.Decimal{
		"USD": decimal.NewFromInt(1),
		"INR": decimal.NewFromFloat(0.012),
	}
	got := gather(t, []mrrGroup{
		{Plan: "starter", Period: "monthly", Currency: "USD", Subscriptions: 1},
		{Plan: "starter", Period: "monthly", Currency: "INR", Subscriptions: 1},
	}, nil, rates)

	require.Len(t, got, 2, "one series per currency")
	assert.InDelta(t, 19.0, got[[2]string{"starter", "USD"}], 1e-9)
	assert.InDelta(t, 11.988, got[[2]string{"starter", "INR"}], 1e-9)
}

// A plan the catalog cannot price is excluded from the gauge (not emitted as a
// misleading zero) while priceable groups in the same scrape are unaffected.
func TestMRRRollup_UnpriceablePlanExcluded(t *testing.T) {
	got := gather(t, []mrrGroup{
		{Plan: "starter", Period: "monthly", Currency: "USD", Subscriptions: 1},
		{Plan: "marketplace", Period: "monthly", Currency: "USD", Subscriptions: 5},
		{Plan: "starter", Period: "quarterly", Currency: "USD", Subscriptions: 7},
	}, nil, usdOnly)

	require.Len(t, got, 1, "only the priceable group is emitted")
	assert.InDelta(t, 19.0, got[[2]string{"starter", "USD"}], 1e-9)
	_, ok := got[[2]string{"marketplace", "USD"}]
	assert.False(t, ok, "unpriceable plan must not appear as a zero series")
}

// If nothing at all can be priced the collector still emits the placeholder
// zero so the series does not gap.
func TestMRRRollup_AllGroupsUnpriceableEmitsZero(t *testing.T) {
	got := gather(t, []mrrGroup{
		{Plan: "marketplace", Period: "monthly", Currency: "USD", Subscriptions: 5},
	}, nil, usdOnly)

	assert.Equal(t, map[[2]string]float64{{"all", "USD"}: 0}, got)
}

// Zero active subscriptions — the production state today — emits a zero.
func TestMRRRollup_NoActiveSubscriptions(t *testing.T) {
	got := gather(t, []mrrGroup{}, nil, usdOnly)
	assert.Equal(t, map[[2]string]float64{{"all", "USD"}: 0}, got)
}

// A DB error emits the fallback zero rather than gapping or panicking.
func TestMRRRollup_DBErrorEmitsZero(t *testing.T) {
	got := gather(t, nil, errors.New("boom"), usdOnly)
	assert.Equal(t, map[[2]string]float64{{"unknown", "USD"}: 0}, got)
}

// An FX outage must not lose the USD bucket, and a currency with no rate
// contributes 0 while keeping its series visible.
func TestMRRRollup_FXErrorKeepsUSDAndZeroesOthers(t *testing.T) {
	c := newMRRRollupWithReader(
		stubGroupReader{groups: []mrrGroup{
			{Plan: "starter", Period: "monthly", Currency: "USD", Subscriptions: 1},
			{Plan: "starter", Period: "monthly", Currency: "INR", Subscriptions: 4},
		}},
		stubRates{err: errors.New("fx down")},
		quietLogger(),
	)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, mfs, 1)

	values := map[string]float64{}
	for _, mtr := range mfs[0].GetMetric() {
		for _, l := range mtr.GetLabel() {
			if l.GetName() == "currency" {
				values[l.GetValue()] = mtr.GetGauge().GetValue()
			}
		}
	}
	assert.InDelta(t, 19.0, values["USD"], 1e-9)
	assert.InDelta(t, 0.0, values["INR"], 1e-9)
}

// The collector must never panic, whatever the input.
func TestMRRRollup_NeverPanics(t *testing.T) {
	c := newMRRRollupWithReader(
		stubGroupReader{groups: []mrrGroup{
			{Plan: "", Period: "", Currency: "", Subscriptions: 0},
			{Plan: "STARTER", Period: "ANNUAL", Currency: "usd", Subscriptions: 1},
		}},
		stubRates{rates: nil},
		quietLogger(),
	)
	ch := make(chan prometheus.Metric, 8)
	require.NotPanics(t, func() { c.Collect(ch) })
	close(ch)

	// Case-insensitive plan/period/currency still resolve.
	var found bool
	for mtr := range ch {
		var pb dto.Metric
		require.NoError(t, mtr.Write(&pb))
		if pb.GetGauge().GetValue() > 0 {
			found = true
			assert.InDelta(t, 182.0/12, pb.GetGauge().GetValue(), 1e-9)
		}
	}
	assert.True(t, found, "expected the case-varied row to be priced")
}
