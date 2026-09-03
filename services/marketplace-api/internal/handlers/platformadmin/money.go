package platformadmin

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// money is the wire representation of a resolved subscription price.
// Currency is always uppercase ISO 4217 on the wire.
type money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// CatalogResolver is the subset of *consolecatalog.Cache this package needs:
// one call that always answers. Declared here as a local interface for the
// same reason TenantDirectory and TenantLifecycle are — this package stays
// testable without building a console client — and typed to return the
// Resolution rather than a bare Catalog because the SOURCE is what the
// fallback logging below reports.
//
// Resolve returns no error by contract (see consolecatalog.Cache.Resolve): a
// degradation is reported through Source and Err, never by failing.
type CatalogResolver interface {
	Resolve(ctx context.Context) consolecatalog.Resolution
}

// priceCatalog is the catalog ONE RESPONSE is priced from.
//
// Resolved once per request and then queried per row. Resolving per row
// would be correct but wasteful — a 500-row page would call Resolve 500
// times — and, worse, a TTL expiry midway through a page could price the
// top of the response from one catalog and the bottom from another. One
// response quotes one catalog.
type priceCatalog struct {
	prices consolecatalog.PriceIndex
}

// compiledPriceCatalog prices from internal/billing/pricing, projected into
// the console's row shape.
//
// # This is the kill switch, and it is config, not code
//
// When CONSOLE_CATALOG_* is unconfigured, main.go passes a nil
// CatalogResolver, resolvePriceCatalog lands here, and this surface prices
// exactly as it did before the cutover (tesserix-home#328 phase C). Unsetting
// the console credentials therefore reverts the cutover with a rollout, not
// a revert — the same contract startCatalogParityRun and
// startAdminCatalogResolve already state for phases A and B.
//
// The mode argument to CompiledCatalog only labels Catalog.Mode, which
// nothing in this file reads; the compiled amounts are the same either way,
// so there is no mode to pass and none is invented.
func compiledPriceCatalog() priceCatalog {
	return priceCatalog{prices: consolecatalog.CompiledCatalog("").Index()}
}

// resolvePriceCatalog resolves the catalog for one request.
//
// Three ways in, and all three end at a usable catalog:
//
//   - r == nil — the console catalog is unconfigured. Compiled. See
//     compiledPriceCatalog: this is the deliberate rollback path, and it must
//     never be a panic or an empty answer.
//   - r.Resolve — fresh, stale or compiled, decided inside the cache. It
//     cannot fail; a degradation arrives as Source/Err on the Resolution.
//   - a resolution carrying no usable prices — compiled. Nothing in the cache
//     is supposed to produce this (a never-published mode is a 404, which the
//     cache already answers with the compiled catalog), but an empty index
//     silently strips the amount from every row on the page, which looks
//     exactly like "these merchants have no prices". Falling back costs one
//     projection and removes that failure mode entirely.
func resolvePriceCatalog(ctx context.Context, r CatalogResolver, fb *catalogFallbackLog, log *slog.Logger) priceCatalog {
	if r == nil {
		return compiledPriceCatalog()
	}

	res := r.Resolve(ctx)
	fb.note(log, res)

	ix := res.Catalog.Index()
	if ix.Len() == 0 {
		if log != nil {
			log.Warn("platformadmin: resolved catalog holds no usable prices; pricing from the compiled catalog",
				"source", string(res.Source), "revision_id", res.Catalog.RevisionID)
		}
		return compiledPriceCatalog()
	}
	return priceCatalog{prices: ix}
}

// resolveMoney returns the catalog price for a subscription's plan, period
// and currency, or ok=false when no price can be determined.
//
// ok=false is a normal outcome, not an error:
//   - billing_currency is NULL (the merchant never chose one), or
//   - the plan has no price at all — the catalog excludes `trial` and
//     `marketplace` by design ("no Price objects"), or
//   - the catalog has no entry for that plan/period/tier/currency combination.
//
// Callers OMIT the amount key entirely in that case. Never null, never 0,
// never a guessed currency: a number the system cannot determine must not
// be reported as a number.
//
// Cannot panic. That was true when this called pricing.LookupPPPOption
// directly (and deliberately not pricing.MustGet, which panics on a miss),
// and it stays true through a map lookup that reports absence with a bool.
//
// # An absent price can never surface as amount=0
//
// PriceIndex.Find returns (Price, bool) and this returns early on
// ok=false, so the only amount that can reach the wire is one that came off
// a row the index actually holds. Index in turn refuses to store a row with
// no currency, which is the specific shape the compiled catalog's
// pre-populated Options map produces for a gap. The property the previous
// implementation held with an `amt.Currency != ""` guard therefore holds
// here by construction, on both sources: a missing price yields ok=false,
// never an amount of zero.
func (pc priceCatalog) resolveMoney(plan, period string, billingCurrency *string, tier subscription.PriceTier) (money, bool) {
	if billingCurrency == nil || strings.TrimSpace(*billingCurrency) == "" {
		return money{}, false
	}

	// Only PriceTierPPP looks up a PPP price; ANY other value — including an
	// unexpected or empty one — looks up a developed price. That is the
	// branch structure this replaced, preserved exactly, so a row whose
	// price_tier column is somehow not one of the two still resolves the way
	// it does today rather than losing its amount.
	t := subscription.PriceTierDeveloped
	if tier == subscription.PriceTierPPP {
		t = subscription.PriceTierPPP
	}

	// Find folds case on all four fields, so the char(3) billing_currency
	// column's case does not matter here; the wire contract's uppercase is
	// applied below, to the CATALOG's currency rather than to the caller's,
	// so the response quotes the currency the price is actually in.
	p, ok := pc.prices.Find(plan, period, string(t), *billingCurrency)
	if !ok {
		return money{}, false
	}
	return money{Amount: p.UnitAmountMinor, Currency: strings.ToUpper(p.Currency)}, true
}

// catalogFallbackLogInterval bounds how often one handler reports that it
// priced a response from a non-fresh catalog.
const catalogFallbackLogInterval = 5 * time.Minute

// catalogFallbackLog throttles the "this page was priced from a stale or
// compiled catalog" warning.
//
// # Why not simply warn per request
//
// Because this is a request path. A console operator refreshing a list
// during a console-catalog outage would produce one warn line per refresh
// for as long as the outage lasted, which buries the first line — the one
// that actually says when the degradation started — under the noise of
// someone hitting F5. Phase A flagged unrate-limited warn-per-resolve as
// exactly the thing not to ship into phase C.
//
// # Why not simply drop it
//
// Because a stale amount displayed is indistinguishable from a correct one
// until someone compares it to Stripe. startAdminCatalogResolve's ticker
// does report the same kind of degradation every 15m, but it resolves
// through its OWN cache instance — a fresh read there is not evidence about
// the cache THIS surface served from, so the request path has to be able to
// speak for itself.
//
// So: at most one line per interval per handler, carrying how many
// resolutions it stands for. The first degradation after a quiet period is
// always logged immediately; the storm behind it is counted, not printed.
type catalogFallbackLog struct {
	mu         sync.Mutex
	lastLogged time.Time
	suppressed int
	// now is injectable so the interval is testable without sleeping.
	now func() time.Time
}

func newCatalogFallbackLog() *catalogFallbackLog {
	return &catalogFallbackLog{now: time.Now}
}

// note records one resolution, logging at most one line per interval.
//
// Nil-safe on both the receiver and the logger: a handler constructed
// without either still serves, because observability must not be able to
// take down the surface it observes.
func (l *catalogFallbackLog) note(log *slog.Logger, res consolecatalog.Resolution) {
	if l == nil || log == nil || res.Source == consolecatalog.SourceFresh {
		return
	}

	l.mu.Lock()
	now := l.now()
	if !l.lastLogged.IsZero() && now.Sub(l.lastLogged) < catalogFallbackLogInterval {
		l.suppressed++
		l.mu.Unlock()
		return
	}
	suppressed := l.suppressed
	l.suppressed = 0
	l.lastLogged = now
	l.mu.Unlock()

	attrs := []any{
		"source", string(res.Source),
		"revision_id", res.Catalog.RevisionID,
		"prices", len(res.Catalog.Prices),
		"suppressed_since_last_line", suppressed,
		"interval", catalogFallbackLogInterval.String(),
	}
	if res.Err != nil {
		attrs = append(attrs, "error", res.Err)
	}
	log.Warn("platformadmin: billing amounts priced from a non-fresh catalog", attrs...)
}
