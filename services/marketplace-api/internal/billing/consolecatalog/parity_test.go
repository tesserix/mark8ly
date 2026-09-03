package consolecatalog_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// The parallel-run monitor. It reads both sources and records their
// differences; it never decides a price. Prices keep coming from the
// compiled catalog until the cutover, which is gated on this reporting
// durably zero.

type stubFetcher struct {
	calls   int32
	catalog consolecatalog.Catalog
	err     error
}

func (s *stubFetcher) Fetch(context.Context) (consolecatalog.Catalog, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.catalog, s.err
}

func matchingCatalog() consolecatalog.Catalog {
	var prices []consolecatalog.Price
	for _, d := range pricing.AllDescriptors() {
		for cur, amt := range d.Options {
			if amt.Currency == "" {
				continue
			}
			// Plan, Period and Tier are carried because a real console row
			// always carries them (NOT NULL in the console schema) and Diff
			// compares them: they are the fields the SERVING lookup keys on,
			// so leaving them out of the "matching" fixture would make this
			// test assert agreement over a response the console cannot send.
			prices = append(prices, consolecatalog.Price{
				LookupKey: d.LookupKey, Currency: cur,
				Plan: string(d.Plan), Period: string(d.Period), Tier: string(d.Tier),
				UnitAmountMinor: amt.UnitAmountMinor, TaxBehavior: amt.TaxBehavior,
			})
		}
	}
	return consolecatalog.Catalog{Mode: "test", RevisionID: "rev-1", Prices: prices}
}

func TestMonitor_AgreeingSourcesReportZeroDifferences(t *testing.T) {
	f := &stubFetcher{catalog: matchingCatalog()}
	m := consolecatalog.NewMonitor(f, time.Hour, nil)

	res := m.Check(context.Background())

	require.True(t, res.Compared, "an agreeing check must count as a real comparison")
	require.Zero(t, res.Differences)
	require.Empty(t, res.Err)
}

// The invariant, stated as a test: an unreachable console must never break
// anything. The monitor reports that it could not compare — it does not
// report zero differences, which would be indistinguishable from agreement
// and would let the cutover proceed on no evidence at all.
func TestMonitor_AnUnreachableConsoleIsNotZeroDifferences(t *testing.T) {
	f := &stubFetcher{err: errors.New("boom")}
	m := consolecatalog.NewMonitor(f, time.Hour, nil)

	res := m.Check(context.Background())

	require.False(t, res.Compared,
		"a failed read must be distinguishable from a clean comparison — otherwise "+
			"an outage looks exactly like agreement and the cutover gate means nothing")
	require.Error(t, res.Err)
}

// The data changes a few times a year, so a generous TTL keeps the console
// off the hot path entirely.
func TestMonitor_DoesNotRefetchWithinTheInterval(t *testing.T) {
	f := &stubFetcher{catalog: matchingCatalog()}
	m := consolecatalog.NewMonitor(f, time.Hour, nil)

	for i := 0; i < 5; i++ {
		m.Check(context.Background())
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&f.calls),
		"the console must not be read once per check; the interval is the whole point")
}

func TestMonitor_RefetchesOnceTheIntervalHasElapsed(t *testing.T) {
	f := &stubFetcher{catalog: matchingCatalog()}
	m := consolecatalog.NewMonitor(f, time.Nanosecond, nil)

	m.Check(context.Background())
	time.Sleep(2 * time.Millisecond)
	m.Check(context.Background())

	require.Equal(t, int32(2), atomic.LoadInt32(&f.calls))
}

// A real divergence must be reported with enough detail to act on.
func TestMonitor_ReportsDivergenceWithDetail(t *testing.T) {
	c := matchingCatalog()
	c.Prices[0].UnitAmountMinor += 1
	f := &stubFetcher{catalog: c}
	m := consolecatalog.NewMonitor(f, time.Hour, nil)

	res := m.Check(context.Background())

	require.True(t, res.Compared)
	require.Equal(t, 1, res.Differences)
	require.Len(t, res.Sample, 1)
	require.Contains(t, res.Sample[0].Detail, "unit_amount_minor")
}
