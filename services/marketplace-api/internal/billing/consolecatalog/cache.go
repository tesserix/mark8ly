package consolecatalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Source names which of the three tiers answered a Resolve.
//
// It is part of the result rather than only a log line because the caller
// has to be able to act on it: the question the cutover must be able to
// answer continuously is "how often did this service price from something
// other than a fresh console read", and a degradation nobody can count is a
// degradation nobody notices.
type Source string

const (
	// SourceFresh — a cached catalog younger than the TTL, or one just
	// fetched successfully.
	SourceFresh Source = "fresh"
	// SourceStale — a previously fetched catalog now past its TTL, served
	// because the refresh failed. Fail-open: a price from last week is
	// enormously better than no price on a checkout path.
	SourceStale Source = "stale"
	// SourceCompiled — internal/billing/pricing, the snapshot baked into the
	// binary. This is what makes a cold start during a console outage
	// survivable: a pod that has never reached the console still holds a
	// complete, correct catalog.
	SourceCompiled Source = "compiled"
)

// DefaultTTL is deliberately long. The catalog changes a few times a year,
// so a stale window measured in hours costs nothing and keeps the console
// far away from anything a customer waits on. Shortening this does not make
// prices more correct; it only multiplies the number of requests that can
// discover the console is down.
const DefaultTTL = 6 * time.Hour

// SharedRevisionID is the revision BOTH the console's test and live
// publications named when the two were last verified identical: on
// 2026-09-03 each served 78 rows with a symmetric difference of 0.
//
// That equivalence is load-bearing evidence, not trivia. It is the whole
// reason mark8ly's test-mode comparison is accepted as evidence for live,
// and why no separate live-mode observation window was required
// (tesserix-home#328).
//
// The premise expires the moment the console can publish to live
// independently of test — which is exactly what tesserix-home#327 phase P2b
// enables. So it is asserted here rather than remembered: a catalog naming
// any other revision means the verification above no longer covers what is
// running, and someone must re-verify the two modes before leaning on
// test-mode evidence again.
//
// Reading a different revision is NOT itself proof that the modes diverged —
// a legitimate republish moves the id too. It only proves the check is
// stale, which is why it is reported and not refused.
const SharedRevisionID = "00000000-0000-0000-0000-000000000001"

// Resolution is one answer from the Cache, plus everything a caller needs to
// know about how much to trust it.
type Resolution struct {
	Catalog Catalog
	// Source names which tier answered. Never empty.
	Source Source
	// Stale is true whenever Source is not SourceFresh. Redundant with
	// Source by construction, and deliberately so: the one question every
	// caller asks is "is this current", and answering it should not require
	// knowing the tier vocabulary.
	Stale bool
	// FetchedAt is when the served catalog was read from the console. Zero
	// for SourceCompiled, which was never read from anywhere.
	FetchedAt time.Time
	// Err is the console read error that forced the degradation, nil when
	// Source is SourceFresh. It is carried, not returned, because Resolve
	// does not fail — Source already says what happened, and an error return
	// would tempt a caller into treating a degraded answer as no answer.
	Err error
	// RevisionUnexpected reports that the served catalog names a revision
	// other than SharedRevisionID — see that constant for why anyone cares.
	// Always false for SourceCompiled, which has no console revision to check.
	RevisionUnexpected bool
}

// Cache resolves the plan catalog through fresh -> last-known -> compiled.
//
// # The property this type exists to hold
//
// A failed refresh must never evict a good value. Every degradation path
// below either returns what was already held or falls through to the
// compiled catalog; none of them writes. An outage degrades to stale, never
// to empty.
//
// # Where this sits relative to Client
//
// Client already retains its last body and ETag so it can answer a 304 with
// a real catalog. That retention solves a different problem — the console
// sends Cache-Control: no-cache with an ETag, so every read revalidates and
// a 304 carries no body — and it only survives a SUCCESSFUL round trip.
// This cache sits above it and covers the cases Client deliberately does
// not: no round trip at all (fresh hit inside the TTL), a round trip that
// failed (stale), and a process that has never had one (compiled). Neither
// layer duplicates the other, and this one never inspects ETags.
//
// # It resolves nothing on its own
//
// Nothing wires this to serving yet. Prices still come from
// internal/billing/pricing via the existing call sites; this type is built
// and tested first so the cutover is one deliberate change rather than a
// cutover plus an untested cache.
type Cache struct {
	fetcher Fetcher
	ttl     time.Duration
	mode    string
	logger  *slog.Logger
	// now is injectable so TTL expiry is testable without sleeping. Tests
	// that sleep to cross a boundary are slow and flaky in equal measure.
	now func() time.Time

	// group collapses concurrent misses into one console call. Without it a
	// pod that wakes up with an expired entry aims its whole in-flight
	// request set at the console at once — the exact stampede that turns a
	// slow console into a failed one.
	group singleflight.Group

	mu       sync.RWMutex
	cached   Catalog
	cachedAt time.Time
	have     bool
}

// NewCache builds a Cache. A ttl of zero or less takes DefaultTTL; mode is
// the mode the caller configured, used by the contract guard below.
func NewCache(f Fetcher, ttl time.Duration, mode string, logger *slog.Logger) *Cache {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Cache{fetcher: f, ttl: ttl, mode: mode, logger: logger, now: time.Now}
}

// Resolve returns the best catalog available, never an error.
//
// The tiers are tried in order and the first that can answer wins:
//
//  1. fresh    — a cached catalog younger than the TTL; no console call
//  2. stale    — the last good catalog, when the refresh failed
//  3. compiled — internal/billing/pricing, when nothing was ever cached
//
// Every fallback is logged, because a service silently pricing from a
// week-old catalog is indistinguishable from a healthy one until someone
// looks at the amounts.
func (c *Cache) Resolve(ctx context.Context) Resolution {
	if res, ok := c.fresh(); ok {
		return res
	}

	// singleflight.Do, not DoChan: the waiters want the same answer, and a
	// caller whose ctx is cancelled mid-flight still gets a usable
	// resolution below rather than an error, so there is nothing to select
	// on. `shared` is ignored — which goroutine actually made the call is
	// not information any caller acts on.
	v, err, _ := c.group.Do("refresh", func() (any, error) {
		// Re-check under the flight: while this goroutine was queueing, the
		// previous flight may have refreshed the entry. Without this the
		// first waiter after a successful refresh issues a second,
		// pointless console call.
		if res, ok := c.fresh(); ok {
			return res, nil
		}
		return c.refresh(ctx)
	})
	if err == nil {
		return v.(Resolution)
	}

	// From here the console did not answer. Nothing below writes to the
	// cache: a failed read must not be able to change what is held.
	if res, ok := c.lastKnown(err); ok {
		c.warn("consolecatalog: console read failed; serving the last-known catalog",
			"error", err, "age", c.now().Sub(res.FetchedAt).String(),
			"revision_id", res.Catalog.RevisionID)
		return res
	}

	// Cold start during an outage. ErrNotPublished lands here too, and that
	// is correct: "this mode has never been published" is a reason to price
	// from the compiled snapshot, never a reason to hold zero prices. It is
	// carried on Err distinctly from ErrUnavailable so the two remain
	// tellable apart by whoever reads the logs.
	c.warn("consolecatalog: no console catalog available; falling back to the compiled catalog",
		"error", err, "not_published", errors.Is(err, ErrNotPublished))
	return Resolution{
		Catalog: CompiledCatalog(c.mode),
		Source:  SourceCompiled,
		Stale:   true,
		Err:     err,
	}
}

// fresh returns the cached catalog when it is still inside the TTL.
func (c *Cache) fresh() (Resolution, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.have || c.now().Sub(c.cachedAt) >= c.ttl {
		return Resolution{}, false
	}
	return Resolution{
		Catalog:            c.cached,
		Source:             SourceFresh,
		FetchedAt:          c.cachedAt,
		RevisionUnexpected: c.cached.RevisionID != SharedRevisionID,
	}, true
}

// lastKnown returns the cached catalog regardless of age, for use when the
// refresh failed.
func (c *Cache) lastKnown(cause error) (Resolution, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.have {
		return Resolution{}, false
	}
	return Resolution{
		Catalog:            c.cached,
		Source:             SourceStale,
		Stale:              true,
		FetchedAt:          c.cachedAt,
		Err:                cause,
		RevisionUnexpected: c.cached.RevisionID != SharedRevisionID,
	}, true
}

// refresh reads the console and, only on success, replaces what is held.
func (c *Cache) refresh(ctx context.Context) (Resolution, error) {
	cat, err := c.fetcher.Fetch(ctx)
	if err != nil {
		return Resolution{}, err
	}
	if err := c.guard(cat); err != nil {
		// A catalog that fails the contract guard is not stored. Treating a
		// wrong-mode response as good would silently overwrite correct
		// prices with another mode's, which is worse than any outage this
		// package is designed to survive.
		return Resolution{}, err
	}

	now := c.now()
	c.mu.Lock()
	c.cached, c.cachedAt, c.have = cat, now, true
	c.mu.Unlock()

	res := Resolution{
		Catalog:            cat,
		Source:             SourceFresh,
		FetchedAt:          now,
		RevisionUnexpected: cat.RevisionID != SharedRevisionID,
	}
	if res.RevisionUnexpected {
		c.warn("consolecatalog: catalog revision is not the one the test-for-live "+
			"equivalence was verified against; that verification is now stale and "+
			"must be redone before test-mode evidence is used for live "+
			"(tesserix-home#328; #327 P2b is what lets the modes diverge)",
			"revision_id", cat.RevisionID, "verified_revision_id", SharedRevisionID,
			"mode", cat.Mode)
	}
	return res, nil
}

// guard rejects a response that contradicts the configured mode.
//
// The service reads exactly one mode and asks for it by query parameter, so
// a response labelled another one means the console answered a question
// nobody asked. Refusing keeps the failure loud and bounded — the caller
// degrades to stale or compiled — instead of quietly pricing live customers
// from the test catalog or the reverse. This does NOT reach across modes to
// compare them: one process reads one mode, and cross-mode comparison is the
// console's job, not this service's.
func (c *Cache) guard(cat Catalog) error {
	if c.mode != "" && cat.Mode != "" && cat.Mode != c.mode {
		return fmt.Errorf("%w: console answered mode %q for a %q request",
			ErrUnavailable, cat.Mode, c.mode)
	}
	return nil
}

func (c *Cache) warn(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, args...)
	}
}

// compiledRevisionID marks a catalog as having come from the binary rather
// than from any console publication. It is not a uuid on purpose: anything
// that logs or compares a revision should be unable to mistake this for one
// the console issued.
const compiledRevisionID = "compiled"

// CompiledCatalog projects internal/billing/pricing into the console's
// Catalog shape, so the fallback tier answers with the same type as the
// other two and callers need no second code path.
//
// It flattens descriptors one row per currency, exactly as compare.go's
// compiledRows does and for the same two reasons: a developed-tier
// descriptor is one Price object carrying seven currencies, and Options is
// pre-populated with zero-value entries for currencies that have no price,
// which must be skipped rather than emitted as an amount of zero. Getting
// that wrong here would hand a checkout a price of nothing.
func CompiledCatalog(mode string) Catalog {
	descriptors := pricing.AllDescriptors()
	out := Catalog{
		Mode:       mode,
		RevisionID: compiledRevisionID,
		Prices:     make([]Price, 0, len(descriptors)),
	}
	for _, d := range descriptors {
		for cur, amt := range d.Options {
			if amt.Currency == "" {
				continue
			}
			out.Prices = append(out.Prices, Price{
				LookupKey:       d.LookupKey,
				Plan:            string(d.Plan),
				Period:          string(d.Period),
				Tier:            string(d.Tier),
				Currency:        cur,
				UnitAmountMinor: amt.UnitAmountMinor,
				TaxBehavior:     amt.TaxBehavior,
			})
		}
	}
	// Map iteration order is random, so without this two calls in the same
	// process return the same prices in a different order. Sorted, the
	// fallback catalog is byte-comparable between calls and a test that
	// compares two projections is not flaky.
	sort.Slice(out.Prices, func(i, j int) bool {
		if out.Prices[i].LookupKey != out.Prices[j].LookupKey {
			return out.Prices[i].LookupKey < out.Prices[j].LookupKey
		}
		return out.Prices[i].Currency < out.Prices[j].Currency
	})
	return out
}
