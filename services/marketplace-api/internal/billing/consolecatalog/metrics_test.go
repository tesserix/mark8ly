package consolecatalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
)

// The metrics exist so a parity difference ARRIVES somewhere instead of being
// written to a log nothing watches (tesserix-home#328 gap 3). These tests pin
// the one property that makes them worth having: a monitor that has stopped
// checking must not read like a monitor that keeps finding nothing.

// snapshot is what the three metrics say right now.
type snapshot struct {
	differences float64
	lastSuccess float64
	failures    float64
}

func readMetrics() snapshot {
	return snapshot{
		differences: testutil.ToFloat64(consolecatalog.ParityDifferences),
		lastSuccess: testutil.ToFloat64(consolecatalog.LastSuccessTimestamp),
		failures:    testutil.ToFloat64(consolecatalog.CheckFailuresTotal),
	}
}

// alwaysCheck builds a monitor whose interval never suppresses a refetch, so
// each Check is a real attempt.
func alwaysCheck(f consolecatalog.Fetcher) *consolecatalog.Monitor {
	return consolecatalog.NewMonitor(f, time.Nanosecond, nil)
}

func TestMetrics_ACleanComparisonRecordsZeroDifferencesAndStampsTheTime(t *testing.T) {
	before := readMetrics()
	m := alwaysCheck(&stubFetcher{catalog: matchingCatalog()})

	m.Check(context.Background())

	got := readMetrics()
	require.Zero(t, got.differences)
	require.Greater(t, got.lastSuccess, before.lastSuccess,
		"a completed comparison must advance the success timestamp — it is the only "+
			"evidence that the zero above describes now rather than last week")
	require.Equal(t, before.failures, got.failures,
		"a clean comparison is not a failure")
}

func TestMetrics_ADifferenceIsCounted(t *testing.T) {
	c := matchingCatalog()
	c.Prices[0].UnitAmountMinor += 1
	m := alwaysCheck(&stubFetcher{catalog: c})

	m.Check(context.Background())

	require.Equal(t, float64(1), readMetrics().differences)
}

// THE test. It is the metrics-side statement of the invariant Result.Compared
// exists for: "a failed read must not look like agreement".
//
// Both collapsed implementations fail here:
//
//   - stamping LastSuccessTimestamp on a failed read too — then a monitor that
//     can no longer reach the console goes on looking freshly-checked forever,
//     and the staleness alert never fires. Caught by the timestamp assertion.
//   - Set(0) on ParityDifferences when a read fails — then an outage reports
//     exactly what agreement reports, which is the original defect with extra
//     steps. Caught by the differences assertion, which requires the value
//     from the last REAL comparison to survive.
func TestMetrics_AFailedReadIsDistinguishableFromACleanRun(t *testing.T) {
	// First a real comparison that finds something, so "differences" holds a
	// value a collapse would have to destroy.
	diverged := matchingCatalog()
	diverged.Prices[0].UnitAmountMinor += 1
	m := alwaysCheck(&stubFetcher{catalog: diverged})
	m.Check(context.Background())

	afterCompare := readMetrics()
	require.Equal(t, float64(1), afterCompare.differences)

	// Now the console goes away. Sleep first so a wrongly-stamped timestamp
	// is unmistakably different rather than a rounding coincidence.
	time.Sleep(5 * time.Millisecond)
	failing := alwaysCheck(&stubFetcher{err: errors.New("console 503")})
	res := failing.Check(context.Background())
	require.False(t, res.Compared)

	got := readMetrics()
	require.Equal(t, afterCompare.lastSuccess, got.lastSuccess,
		"a failed read must NOT advance the last-success timestamp: if it does, a "+
			"monitor that has stopped checking is indistinguishable from one that "+
			"keeps finding nothing, and the staleness alert can never fire")
	require.Equal(t, afterCompare.differences, got.differences,
		"a failed read must NOT rewrite the difference count: reporting zero because "+
			"the console was unreachable is exactly the outage-looks-like-agreement "+
			"defect Result.Compared exists to prevent")
	require.Equal(t, afterCompare.failures+1, got.failures,
		"the failure must be counted somewhere — it is what separates 'running and "+
			"cannot reach the console' from 'not running at all'")
}
