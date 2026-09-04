package consolecatalog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Fetcher is the console read the Monitor depends on. An interface so the
// monitor is testable without a network, and so the cutover can swap what
// feeds it without touching the comparison.
type Fetcher interface {
	Fetch(context.Context) (Catalog, error)
}

// Result is one comparison's outcome.
//
// Compared is deliberately separate from Differences. A failed read must not
// look like agreement: reporting zero differences when the console could not
// be reached would make an outage indistinguishable from a clean run, and
// the cutover gate — "durably zero" — would then be satisfiable by an
// outage. That is the single most important distinction in this type.
type Result struct {
	Compared    bool
	Differences int
	Sample      []Difference
	RevisionID  string
	Err         error
}

// sampleSize caps how many differences are carried for logging. A hundred
// identical lines help nobody find the first one.
const sampleSize = 10

// Monitor runs the parallel comparison of the console's catalog against the
// compiled one (#304, #392).
//
// It NEVER resolves a price. Prices continue to come from
// internal/billing/pricing until the cutover, which is a separate, deliberate
// change gated on this reporting durably zero. Keeping the monitor incapable
// of answering "what is the price" is what makes slice one safe to run in
// production on the payment path's own process.
//
// The interval keeps the console entirely off the hot path: the data changes
// a few times a year, so a comparison every few minutes is generous.
type Monitor struct {
	fetcher  Fetcher
	interval time.Duration
	logger   *slog.Logger

	mu       sync.Mutex
	lastRun  time.Time
	lastRes  Result
	haveLast bool
}

func NewMonitor(f Fetcher, interval time.Duration, logger *slog.Logger) *Monitor {
	return &Monitor{fetcher: f, interval: interval, logger: logger}
}

// Check compares the two sources, refetching at most once per interval and
// otherwise returning the previous result.
//
// It never returns an error to its caller and never blocks a request on a
// decision: its only outputs are a Result and a log line.
func (m *Monitor) Check(ctx context.Context) Result {
	m.mu.Lock()
	if m.haveLast && time.Since(m.lastRun) < m.interval {
		res := m.lastRes
		m.mu.Unlock()
		return res
	}
	m.mu.Unlock()

	catalog, err := m.fetcher.Fetch(ctx)
	res := Result{}
	if err != nil {
		res.Err = err
		// Counted, but ParityDifferences and LastSuccessTimestamp are left
		// untouched: Result.Compared's whole reason for existing is that a
		// failed read must not look like agreement, and the metrics have to
		// carry that same distinction or they re-create the defect.
		CheckFailuresTotal.Inc()
		m.warn("consolecatalog: could not read the console catalog; comparison skipped", "error", err)
	} else {
		diffs := Diff(catalog, pricing.AllDescriptors())
		res.Compared = true
		res.Differences = len(diffs)
		res.RevisionID = catalog.RevisionID
		if n := len(diffs); n > sampleSize {
			res.Sample = diffs[:sampleSize]
		} else {
			res.Sample = diffs
		}
		// Set the count first and stamp the time second, so a scrape landing
		// between the two reads a stale timestamp beside a fresh count rather
		// than a fresh timestamp vouching for a count not yet written.
		ParityDifferences.Set(float64(res.Differences))
		LastSuccessTimestamp.SetToCurrentTime()
		m.report(res)
	}

	m.mu.Lock()
	m.lastRun, m.lastRes, m.haveLast = time.Now(), res, true
	m.mu.Unlock()
	return res
}

// report logs the outcome. A clean run logs at info so the evidence trail
// the cutover rests on is visible; a divergence logs at warn with a bounded
// sample naming the keys.
func (m *Monitor) report(res Result) {
	if m.logger == nil {
		return
	}
	if res.Differences == 0 {
		m.logger.Info("consolecatalog: parity clean",
			"revision_id", res.RevisionID, "differences", 0)
		return
	}
	for _, d := range res.Sample {
		m.logger.Warn("consolecatalog: parity difference",
			"revision_id", res.RevisionID,
			"lookup_key", d.LookupKey, "currency", d.Currency, "detail", d.Detail)
	}
	m.logger.Warn("consolecatalog: parity summary",
		"revision_id", res.RevisionID, "differences", res.Differences,
		"sampled", len(res.Sample))
}

func (m *Monitor) warn(msg string, args ...any) {
	if m.logger != nil {
		m.logger.Warn(msg, args...)
	}
}
