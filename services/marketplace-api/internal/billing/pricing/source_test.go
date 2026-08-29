package pricing_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// #304 cutover: the console becomes the DATA SOURCE behind these helpers
// while their signatures and callers stay exactly as they are — the issue is
// explicit that deleting this package is not proposed.
//
// Everything here turns on one property: an installed source that cannot
// answer must fall through to the compiled catalog rather than return a
// miss. That fallthrough IS the baked-snapshot cold start and the fail-open
// behaviour BACKLOG §P requires; there is no separate mechanism for it.

type stubSource struct {
	ppp       map[string]pricing.Amount // keyed by currency
	developed map[string]pricing.Amount
	answer    bool
}

func (s *stubSource) PPPOption(_ pricing.Plan, _ pricing.Period, currency string) (pricing.Amount, bool) {
	if !s.answer {
		return pricing.Amount{}, false
	}
	a, ok := s.ppp[currency]
	return a, ok
}

func (s *stubSource) DevelopedOptions(_ pricing.Plan, _ pricing.Period) (map[string]pricing.Amount, bool) {
	if !s.answer {
		return nil, false
	}
	return s.developed, true
}

// restore keeps tests independent — an installed source is process-wide.
func restore(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { pricing.UseSource(nil) })
}

// The regression that matters most: with nothing installed, behaviour is
// byte-for-byte what it is today. This is what makes the cutover flag safe
// to ship turned off.
func TestNoSourceInstalledUsesTheCompiledCatalog(t *testing.T) {
	restore(t)
	pricing.UseSource(nil)

	amt, ok := pricing.LookupPPPOption("pro", "annual", "vnd")
	require.True(t, ok)
	require.Equal(t, int64(1978800000), amt.UnitAmountMinor)

	opts, ok := pricing.DevelopedCurrencyOptions("pro", "annual")
	require.True(t, ok)
	require.NotEmpty(t, opts)
}

func TestInstalledSourceAnswersPPP(t *testing.T) {
	restore(t)
	pricing.UseSource(&stubSource{
		answer: true,
		ppp:    map[string]pricing.Amount{"vnd": {Currency: "vnd", UnitAmountMinor: 42}},
	})

	amt, ok := pricing.LookupPPPOption("pro", "annual", "vnd")
	require.True(t, ok)
	require.Equal(t, int64(42), amt.UnitAmountMinor, "the console's amount must win once it is authoritative")
}

// The cold start. A source installed but unable to answer — a fresh pod
// during a console outage — must NOT produce a miss. money.go treats a miss
// as "no price", and reporting no price is worse than reporting a slightly
// stale one.
func TestSourceThatCannotAnswerFallsBackToTheCompiledCatalog(t *testing.T) {
	restore(t)
	pricing.UseSource(&stubSource{answer: false})

	amt, ok := pricing.LookupPPPOption("pro", "annual", "vnd")
	require.True(t, ok, "a cold or failed source must fall through to the baked snapshot, not miss")
	require.Equal(t, int64(1978800000), amt.UnitAmountMinor)

	opts, ok := pricing.DevelopedCurrencyOptions("pro", "annual")
	require.True(t, ok)
	require.NotEmpty(t, opts)
}

// A source that answers for one currency but not another must fall through
// per lookup, not wholesale.
func TestFallthroughIsPerLookupNotAllOrNothing(t *testing.T) {
	restore(t)
	pricing.UseSource(&stubSource{
		answer: true,
		ppp:    map[string]pricing.Amount{"vnd": {Currency: "vnd", UnitAmountMinor: 42}},
	})

	fromConsole, ok := pricing.LookupPPPOption("pro", "annual", "vnd")
	require.True(t, ok)
	require.Equal(t, int64(42), fromConsole.UnitAmountMinor)

	fromCatalog, ok := pricing.LookupPPPOption("pro", "annual", "inr")
	require.True(t, ok, "a currency the source does not carry must still resolve from the catalog")
	require.NotZero(t, fromCatalog.UnitAmountMinor)
}

// The source is installed at startup and refreshed by a background ticker
// while requests read it. Racing here would be a data race on the price of
// a subscription.
func TestUseSourceIsSafeUnderConcurrentReadsAndSwaps(t *testing.T) {
	restore(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = pricing.LookupPPPOption("pro", "annual", "vnd")
				_, _ = pricing.DevelopedCurrencyOptions("pro", "annual")
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				pricing.UseSource(&stubSource{answer: true, ppp: map[string]pricing.Amount{}})
				pricing.UseSource(nil)
			}
		}()
	}
	wg.Wait()
}
