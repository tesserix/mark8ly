package consolecatalog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Source answers price lookups from the console's catalog, and declines when
// it has nothing trustworthy to answer with (#304 cutover).
//
// It implements pricing.Source, so installing it swaps the DATA behind
// LookupPPPOption and DevelopedCurrencyOptions while every caller and
// signature stays as it is.
//
// # Declining is the safety property
//
// This type answers only from a catalog it actually fetched. Cold, or after
// a failure with nothing cached, it returns false and pricing falls through
// to the compiled catalog — the baked snapshot. It must never answer with a
// zero Amount: money.go reads a zero as a real price, so a fabricated answer
// is silent mispricing, which is strictly worse than a slightly stale one.
//
// # Fail open to last known
//
// Once it holds a catalog, a later failed refresh does not discard it. The
// console being unreachable is not a reason to reprice anything —
// BACKLOG §P again: nothing on a payment path may depend on the console.
type Source struct {
	fetcher  Fetcher
	interval time.Duration
	logger   *slog.Logger

	mu        sync.RWMutex
	ppp       map[pppKey]pricing.Amount
	developed map[planKey]map[string]pricing.Amount
	loaded    bool
	fetchedAt time.Time
	revision  string
}

type pppKey struct {
	plan, period, currency string
}

type planKey struct {
	plan, period string
}

func NewSource(f Fetcher, interval time.Duration, logger *slog.Logger) *Source {
	return &Source{fetcher: f, interval: interval, logger: logger}
}

// Refresh fetches and replaces the held catalog. A failure leaves any
// previously held catalog in place and is returned for logging.
func (s *Source) Refresh(ctx context.Context) error {
	catalog, err := s.fetcher.Fetch(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("consolecatalog: refresh failed; serving last known prices", "error", err)
		}
		return err
	}

	ppp := make(map[pppKey]pricing.Amount)
	developed := make(map[planKey]map[string]pricing.Amount)
	for _, p := range catalog.Prices {
		cur := strings.ToLower(p.Currency)
		amt := pricing.Amount{
			Currency:        cur,
			UnitAmountMinor: p.UnitAmountMinor,
			TaxBehavior:     taxBehaviorForCatalog(p.TaxBehavior),
		}
		switch strings.ToLower(p.Tier) {
		case "ppp":
			ppp[pppKey{p.Plan, p.Period, cur}] = amt
		case "developed":
			k := planKey{p.Plan, p.Period}
			if developed[k] == nil {
				developed[k] = make(map[string]pricing.Amount)
			}
			developed[k][cur] = amt
		}
	}

	s.mu.Lock()
	s.ppp, s.developed = ppp, developed
	s.loaded, s.fetchedAt, s.revision = true, time.Now(), catalog.RevisionID
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("consolecatalog: prices refreshed from the console",
			"revision_id", catalog.RevisionID, "ppp_prices", len(ppp), "developed_groups", len(developed))
	}
	return nil
}

// PPPOption implements pricing.Source.
func (s *Source) PPPOption(plan pricing.Plan, period pricing.Period, currency string) (pricing.Amount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return pricing.Amount{}, false
	}
	amt, ok := s.ppp[pppKey{string(plan), string(period), strings.ToLower(currency)}]
	return amt, ok
}

// DevelopedOptions implements pricing.Source.
func (s *Source) DevelopedOptions(plan pricing.Plan, period pricing.Period) (map[string]pricing.Amount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return nil, false
	}
	opts, ok := s.developed[planKey{string(plan), string(period)}]
	if !ok || len(opts) == 0 {
		return nil, false
	}
	// Copied because callers receive it as the catalog's own map and must
	// not be able to mutate the source out from under other requests.
	out := make(map[string]pricing.Amount, len(opts))
	for k, v := range opts {
		out[k] = v
	}
	return out, true
}

// taxBehaviorForCatalog converts the console's "unspecified" back to the
// empty string the compiled catalog uses for Stripe's default, so an Amount
// from either source compares equal — the same normalisation Diff applies,
// kept consistent deliberately.
func taxBehaviorForCatalog(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "unspecified") {
		return ""
	}
	return v
}
