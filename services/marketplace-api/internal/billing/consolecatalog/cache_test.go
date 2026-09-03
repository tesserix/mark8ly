package consolecatalog

// These tests are in the package rather than consolecatalog_test so they can
// drive the cache's injected clock. TTL expiry tested by sleeping would make
// the suite both slow and flaky, and the fail-open behaviour is exactly the
// thing that must not be tested approximately.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// fakeClock is a hand-advanced clock. Every test moves time explicitly, so
// no test depends on how long it took to run.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeFetcher answers with whatever the test currently wants and counts
// calls, so "did not touch the console" is an assertion and not an
// assumption.
type fakeFetcher struct {
	mu      sync.Mutex
	calls   int32
	catalog Catalog
	err     error
	// before runs inside Fetch, used to widen the window the single-flight
	// test needs to observe concurrent misses.
	before func()
}

func (f *fakeFetcher) Fetch(context.Context) (Catalog, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.before != nil {
		f.before()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catalog, f.err
}

func (f *fakeFetcher) set(cat Catalog, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.catalog, f.err = cat, err
}

func (f *fakeFetcher) count() int32 { return atomic.LoadInt32(&f.calls) }

const testTTL = time.Hour

// consoleCatalog builds a minimal well-formed console response. The contents
// do not matter to the cache — it never inspects an amount — so one row is
// enough to tell two catalogs apart.
func consoleCatalog(mode, revision string, amount int64) Catalog {
	return Catalog{
		Mode:        mode,
		RevisionID:  revision,
		PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Prices: []Price{{
			LookupKey:       "starter_monthly_developed",
			Plan:            "starter",
			Period:          "monthly",
			Tier:            "developed",
			Currency:        "usd",
			UnitAmountMinor: amount,
		}},
	}
}

func newTestCache(f Fetcher, clock *fakeClock, mode string) *Cache {
	c := NewCache(f, testTTL, mode, nil)
	c.now = clock.now
	return c
}

func TestCacheResolveTiers(t *testing.T) {
	catalogA := consoleCatalog("test", SharedRevisionID, 1900)
	catalogB := consoleCatalog("test", SharedRevisionID, 2100)

	tests := []struct {
		name string
		// prime, when set, is fetched and cached before the case's own
		// setup runs — it is how a case distinguishes "nothing was ever
		// cached" from "a good value is held".
		prime      *Catalog
		advance    time.Duration
		then       Catalog
		thenErr    error
		wantSource Source
		wantStale  bool
		wantAmount int64 // 0 means "expect the compiled catalog instead"
		wantErrIs  error
		// wantExtraCalls counts console reads AFTER priming.
		wantExtraCalls int32
	}{
		{
			name:           "fresh inside the TTL never touches the console",
			prime:          &catalogA,
			advance:        testTTL - time.Minute,
			thenErr:        ErrUnavailable, // would fail if it were called
			wantSource:     SourceFresh,
			wantAmount:     1900,
			wantExtraCalls: 0,
		},
		{
			name:           "expired and the console answers returns the new value",
			prime:          &catalogA,
			advance:        testTTL,
			then:           catalogB,
			wantSource:     SourceFresh,
			wantAmount:     2100,
			wantExtraCalls: 1,
		},
		{
			// The fail-open property, and the reason this package exists:
			// an outage must degrade to a stale price, never to no price.
			name:           "expired and the console is down returns the last-known value, flagged stale",
			prime:          &catalogA,
			advance:        testTTL,
			thenErr:        ErrUnavailable,
			wantSource:     SourceStale,
			wantStale:      true,
			wantAmount:     1900,
			wantErrIs:      ErrUnavailable,
			wantExtraCalls: 1,
		},
		{
			// A cold pod during a console outage must not itself be the
			// outage. The compiled snapshot ships in the binary for exactly
			// this moment.
			name:           "cold start with the console down falls back to the compiled catalog",
			advance:        0,
			thenErr:        ErrUnavailable,
			wantSource:     SourceCompiled,
			wantStale:      true,
			wantErrIs:      ErrUnavailable,
			wantExtraCalls: 1,
		},
		{
			// 404 means "this mode was never published", which is a reason
			// to price from the binary — never a reason to hold zero
			// prices. It must also stay distinguishable from ErrUnavailable.
			name:           "cold start with an unpublished mode falls back to the compiled catalog",
			thenErr:        ErrNotPublished,
			wantSource:     SourceCompiled,
			wantStale:      true,
			wantErrIs:      ErrNotPublished,
			wantExtraCalls: 1,
		},
		{
			name:           "an unpublished mode does not evict a good value",
			prime:          &catalogA,
			advance:        testTTL,
			thenErr:        ErrNotPublished,
			wantSource:     SourceStale,
			wantStale:      true,
			wantAmount:     1900,
			wantErrIs:      ErrNotPublished,
			wantExtraCalls: 1,
		},
		{
			// The mode guard. A response labelled another mode is refused
			// rather than stored: overwriting correct prices with the other
			// mode's is worse than any outage this cache survives.
			name:           "a wrong-mode response is refused and does not evict a good value",
			prime:          &catalogA,
			advance:        testTTL,
			then:           consoleCatalog("live", SharedRevisionID, 9900),
			wantSource:     SourceStale,
			wantStale:      true,
			wantAmount:     1900,
			wantErrIs:      ErrUnavailable,
			wantExtraCalls: 1,
		},
		{
			name:           "a wrong-mode response on a cold start falls back to the compiled catalog",
			then:           consoleCatalog("live", SharedRevisionID, 9900),
			wantSource:     SourceCompiled,
			wantStale:      true,
			wantErrIs:      ErrUnavailable,
			wantExtraCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newClock()
			f := &fakeFetcher{}
			c := newTestCache(f, clock, "test")

			if tc.prime != nil {
				f.set(*tc.prime, nil)
				primed := c.Resolve(context.Background())
				require.Equal(t, SourceFresh, primed.Source, "priming must succeed")
				require.Equal(t, int32(1), f.count())
			}
			base := f.count()

			f.set(tc.then, tc.thenErr)
			clock.advance(tc.advance)

			res := c.Resolve(context.Background())

			require.Equal(t, tc.wantSource, res.Source)
			require.Equal(t, tc.wantStale, res.Stale)
			require.Equal(t, tc.wantExtraCalls, f.count()-base, "console reads after priming")

			if tc.wantErrIs != nil {
				require.ErrorIs(t, res.Err, tc.wantErrIs)
			} else {
				require.NoError(t, res.Err)
			}

			if tc.wantAmount != 0 {
				require.Len(t, res.Catalog.Prices, 1)
				require.Equal(t, tc.wantAmount, res.Catalog.Prices[0].UnitAmountMinor)
				return
			}

			// The compiled tier must answer with a COMPLETE catalog, not a
			// token one: a fallback that returns three prices would be an
			// outage wearing a success's clothing. Zero differences against
			// the compiled descriptors is the strongest available statement
			// of that.
			require.NotEmpty(t, res.Catalog.Prices)
			require.Empty(t, Diff(res.Catalog, pricing.AllDescriptors()))
		})
	}
}

// The revision guard. It reports, and does not refuse: a republish moves the
// id legitimately. What it asserts is that the test-for-live equivalence
// verified on 2026-09-03 still covers what is running.
func TestCacheRevisionGuard(t *testing.T) {
	tests := []struct {
		name     string
		revision string
		want     bool
	}{
		{"the verified shared revision is expected", SharedRevisionID, false},
		{"any other revision is reported", "11111111-1111-1111-1111-111111111111", true},
		{"an absent revision is reported", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newClock()
			f := &fakeFetcher{}
			f.set(consoleCatalog("test", tc.revision, 1900), nil)
			c := newTestCache(f, clock, "test")

			fresh := c.Resolve(context.Background())
			require.Equal(t, SourceFresh, fresh.Source)
			require.Equal(t, tc.want, fresh.RevisionUnexpected)

			// It must survive into the degraded answer too — a stale price
			// carrying an unverified revision is exactly the combination
			// someone needs to see.
			clock.advance(testTTL)
			f.set(Catalog{}, ErrUnavailable)
			stale := c.Resolve(context.Background())
			require.Equal(t, SourceStale, stale.Source)
			require.Equal(t, tc.want, stale.RevisionUnexpected)
		})
	}
}

// The compiled tier never claims a console revision, so nothing can mistake
// the binary's snapshot for a publication.
func TestCompiledResolutionCarriesNoConsoleRevision(t *testing.T) {
	f := &fakeFetcher{}
	f.set(Catalog{}, ErrUnavailable)
	c := newTestCache(f, newClock(), "test")

	res := c.Resolve(context.Background())
	require.Equal(t, SourceCompiled, res.Source)
	require.False(t, res.RevisionUnexpected)
	require.Equal(t, compiledRevisionID, res.Catalog.RevisionID)
	require.Equal(t, "test", res.Catalog.Mode)
	require.True(t, res.FetchedAt.IsZero(), "the compiled catalog was never fetched from anywhere")
}

func TestCompiledCatalogIsCompleteAndOrdered(t *testing.T) {
	a := CompiledCatalog("test")
	b := CompiledCatalog("test")

	require.NotEmpty(t, a.Prices)
	require.Equal(t, a.Prices, b.Prices, "two projections must agree despite map iteration order")
	require.Empty(t, Diff(a, pricing.AllDescriptors()))
	for _, p := range a.Prices {
		require.NotZero(t, p.UnitAmountMinor, "a zero amount would price a checkout at nothing: %+v", p)
		require.NotEmpty(t, p.Currency)
		require.NotEmpty(t, p.LookupKey)
	}
}

// A stampede must reach the console once. Without single-flight, a pod whose
// entry has just expired aims its whole in-flight request set at the console
// at the same instant.
func TestCacheConcurrentMissesCollapseToOneCall(t *testing.T) {
	const goroutines = 64

	release := make(chan struct{})
	f := &fakeFetcher{before: func() { <-release }}
	f.set(consoleCatalog("test", SharedRevisionID, 1900), nil)
	c := newTestCache(f, newClock(), "test")

	var start, done sync.WaitGroup
	start.Add(goroutines)
	done.Add(goroutines)
	results := make([]Resolution, goroutines)
	for i := range results {
		go func(i int) {
			defer done.Done()
			start.Done()
			results[i] = c.Resolve(context.Background())
		}(i)
	}
	start.Wait()
	close(release)
	done.Wait()

	require.Equal(t, int32(1), f.count(), "concurrent misses must not stampede the console")
	for i, res := range results {
		require.Equal(t, SourceFresh, res.Source, "goroutine %d", i)
		require.Len(t, res.Catalog.Prices, 1, "goroutine %d", i)
	}
}

// Concurrent readers of a warm cache, for -race. The refresher runs
// alongside them so the read lock and the write lock are actually contended.
func TestCacheConcurrentReadersAndRefresh(t *testing.T) {
	clock := newClock()
	f := &fakeFetcher{}
	f.set(consoleCatalog("test", SharedRevisionID, 1900), nil)
	c := newTestCache(f, clock, "test")
	require.Equal(t, SourceFresh, c.Resolve(context.Background()).Source)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				res := c.Resolve(context.Background())
				// Whatever the tier, a resolution always carries prices.
				// There is no path through this cache that answers nothing.
				require.NotEmpty(t, res.Catalog.Prices)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			clock.advance(testTTL)
			c.Resolve(context.Background())
		}
	}()
	wg.Wait()
}

// The distinction the whole package rests on: "we could not ask" and "there
// are no prices" must never converge, at any tier.
func TestCacheNeverAnswersWithNoPrices(t *testing.T) {
	for _, err := range []error{ErrUnavailable, ErrNotPublished, errors.New("some transport failure")} {
		f := &fakeFetcher{}
		f.set(Catalog{}, err)
		c := newTestCache(f, newClock(), "test")

		res := c.Resolve(context.Background())
		require.NotEmpty(t, res.Catalog.Prices, "error %v produced an empty catalog", err)
	}
}

// The compiled fallback must be COMPLETE, not merely non-empty.
//
// SourceCompiled is what a cold-start pod serves during a console outage, on
// the checkout path. A fallback that is present but missing currencies would
// be worse than the compiled map this service serves today: checkout would
// break for exactly the countries whose rows were dropped, and every other
// test here would still pass because they only assert that prices came back.
//
// The numbers are the console's own, measured against production on
// 2026-09-03: the published catalog holds 42 prices which flatten to 78
// (lookup_key, currency) rows -- the same 42/78 relationship
// `catalog.go` documents for the console's response shape. Pinning them here
// asserts that the compiled projection covers the published catalog exactly,
// so a descriptor or a currency lost from `pricing` fails here rather than
// during an outage.
//
// If a deliberate catalog change moves these numbers, the console's
// published catalog has to move with them -- that is the point, not an
// inconvenience: the two are meant to agree, and `Diff` is what proves they
// still do.
func TestCompiledCatalogCoversThePublishedCatalog(t *testing.T) {
	for _, mode := range []string{"test", "live"} {
		t.Run(mode, func(t *testing.T) {
			cat := CompiledCatalog(mode)

			keys := make(map[string]struct{}, len(cat.Prices))
			for _, p := range cat.Prices {
				keys[p.LookupKey] = struct{}{}
			}

			require.Equal(t, 78, len(cat.Prices),
				"compiled fallback must flatten to the same 78 (lookup_key, currency) rows the console publishes")
			require.Equal(t, 42, len(keys),
				"compiled fallback must carry all 42 lookup keys the console publishes")

			// Mode travels with the answer even on the tier that never read
			// the console, so a caller logging the resolution cannot report a
			// live-mode degradation as a test-mode one.
			require.Equal(t, mode, cat.Mode)
		})
	}
}
