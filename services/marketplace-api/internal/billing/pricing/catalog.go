// Package pricing is mark8ly's compiled price catalog.
//
// It is no longer where a price is authored. The console owns the catalog,
// and the platform console's billing read routes resolve their amounts from
// it (tesserix-home#328 phase C). What lives here is the fallback those
// routes serve from when the console is unconfigured or unreachable, and the
// side internal/billing/consolecatalog compares the console against. It
// therefore still has to be complete: tesserix/mark8ly#631 pinned it at 78
// amounts across 42 lookup keys, because a fallback missing a currency would
// be worse than one that is obviously absent and every count test here would
// still pass.
//
// The two amount tables (developedAmounts, pppAmounts) are generated into
// catalog_data.go — regenerate with cmd/gencatalog rather than editing them
// by hand. Everything in this file is hand-written: the types, the
// lookup-key derivation, and the query helpers below.
//
// Architecture: developed-market tier gets one Price object per (plan, period) —
// Starter+Studio+Pro × monthly+annual — with currency_options carrying all 7
// developed-market currencies on that same object. PPP tiers use a separate
// Price object per (plan, period, currency), since currency_options amounts
// can't be shared across the PPP and developed-market tiers.
//
// For the exact object count by tier, see AllDescriptors and the
// TestCatalog_DevelopedDescriptorCount / TestCatalog_PPPDescriptorCount /
// TestCatalog_TotalDescriptorCount assertions in catalog_test.go — those tests
// are the source of truth and fail if the shape changes without being updated.
//
// Spec reference: docs/superpowers/specs/2026-04-17-subscription-model-design.md §4.1 and §4.1.1.
package pricing

import (
	"fmt"
	"sort"
)

// Plan identifies a billing plan.
type Plan string

const (
	PlanStarter Plan = "starter"
	PlanStudio  Plan = "studio"
	PlanPro     Plan = "pro"
	// PlanTrial and PlanMarketplace have no Price objects — excluded.
)

// Period identifies a billing period.
type Period string

const (
	PeriodMonthly Period = "monthly"
	PeriodAnnual  Period = "annual"
)

// Tier separates developed-market pricing from PPP-adjusted pricing.
type Tier string

const (
	// TierDeveloped covers US, CA, GB, EU, AU, NZ, SG.
	TierDeveloped Tier = "developed"
	// TierPPP covers IN, MY, TH, PH, ID, VN.
	TierPPP Tier = "ppp"
)

// Amount represents a single-currency price entry.
type Amount struct {
	Currency        string // lowercase ISO 4217
	UnitAmountMinor int64  // cents, paise, sen, satang, etc. (× 100 for zero-decimal currencies)
	TaxBehavior     string // "exclusive" for AU GST; "" elsewhere (Stripe default)
}

// PriceDescriptor is the complete definition of one Stripe Price object.
//
// Developed-tier: Baseline holds the primary currency; Options carries all 7
// currency_options that Stripe merges onto the same Price object.
//
// PPP-tier: Baseline holds the single PPP currency; Options is a one-entry map
// with the same currency (each PPP currency is its own Price object).
type PriceDescriptor struct {
	Plan      Plan
	Period    Period
	Tier      Tier
	Currency  string            // lowercase ISO 4217 — baseline currency
	Baseline  Amount            // matches Currency
	Options   map[string]Amount // all currency_options keyed by lowercase ISO 4217
	LookupKey string            // Stripe price.lookup_key for idempotent bootstrap
}

// ---------------------------------------------------------------------------
// Developed-market catalog (7 currencies per Price object)
// All amounts in minor units (cents/pence/cents…).
// Spec §4.1; AU sets TaxBehavior "exclusive" per §19.4.
// ---------------------------------------------------------------------------

// developedLookupKeys returns the canonical Stripe lookup_key for a developed-tier descriptor.
func developedLookupKey(plan Plan, period Period) string {
	return fmt.Sprintf("mark8ly_%s_%s_developed_v1", plan, period)
}

// ---------------------------------------------------------------------------
// PPP-adjusted catalog (one Price object per currency per plan/period)
// Spec §4.1.1: IN, MY, TH, PH, ID, VN.
// Pro IS included in PPP per §4.1.1 (₹65,999/yr listed explicitly).
//
// Zero-decimal currency note: this catalog stores every amount in
// "minor units × 100" as an internal convention, regardless of currency.
// VND is one of Stripe's zero-decimal currencies (unit_amount is the raw
// value, not ×100), so the ×100 has to come back out for it at the Stripe
// boundary. IDR is NOT zero-decimal per Stripe — its ×100 storage is simply
// the ordinary two-decimal representation and needs no special handling at
// the boundary.
//   VND 329,000 → UnitAmountMinor 32900000 → Stripe unit_amount 329000
//   IDR 199,000 → UnitAmountMinor 19900000 → Stripe unit_amount 19900000
// This service no longer writes Prices to Stripe (#303 retired
// cmd/billing-bootstrap once the console became the catalog's authoring
// surface); the boundary conversion, and the zero-decimal currency list it
// depends on, now live in the console's own catalog publisher, which
// mirrors this convention.
// ---------------------------------------------------------------------------

type pppKey struct {
	plan     Plan
	period   Period
	currency string
}

// pppLookupKey returns the canonical Stripe lookup_key for a PPP-tier descriptor.
func pppLookupKey(plan Plan, period Period, currency string) string {
	return fmt.Sprintf("mark8ly_%s_%s_ppp_%s_v1", plan, period, currency)
}

// ---------------------------------------------------------------------------
// Catalog: slice of all PriceDescriptors (built once at init time).
// ---------------------------------------------------------------------------

var allDescriptors []PriceDescriptor

func init() {
	developedCurrencies := []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"}

	// Developed-market descriptors: one descriptor per (plan, period) —
	// baseline = usd; Options = all 7 currencies.
	for _, plan := range []Plan{PlanStarter, PlanStudio, PlanPro} {
		for _, period := range []Period{PeriodMonthly, PeriodAnnual} {
			byPeriod, ok := developedAmounts[plan][period]
			if !ok {
				continue
			}
			baselineAmt := byPeriod["usd"]
			opts := make(map[string]Amount, len(developedCurrencies))
			for _, c := range developedCurrencies {
				opts[c] = byPeriod[c]
			}
			allDescriptors = append(allDescriptors, PriceDescriptor{
				Plan:      plan,
				Period:    period,
				Tier:      TierDeveloped,
				Currency:  "usd",
				Baseline:  baselineAmt,
				Options:   opts,
				LookupKey: developedLookupKey(plan, period),
			})
		}
	}

	// PPP descriptors: one descriptor per (plan, period, currency).
	// Collect keys first so iteration order is deterministic.
	pppKeys := make([]pppKey, 0, len(pppAmounts))
	for k := range pppAmounts {
		pppKeys = append(pppKeys, k)
	}
	sort.SliceStable(pppKeys, func(i, j int) bool {
		a, b := pppKeys[i], pppKeys[j]
		if a.plan != b.plan {
			return a.plan < b.plan
		}
		if a.period != b.period {
			return a.period < b.period
		}
		return a.currency < b.currency
	})
	for _, k := range pppKeys {
		amt := pppAmounts[k]
		opts := map[string]Amount{k.currency: amt}
		allDescriptors = append(allDescriptors, PriceDescriptor{
			Plan:      k.plan,
			Period:    k.period,
			Tier:      TierPPP,
			Currency:  k.currency,
			Baseline:  amt,
			Options:   opts,
			LookupKey: pppLookupKey(k.plan, k.period, k.currency),
		})
	}

	// Sort all descriptors by (Plan, Period, Tier, Currency) for stable output.
	sort.SliceStable(allDescriptors, func(i, j int) bool {
		a, b := allDescriptors[i], allDescriptors[j]
		if a.Plan != b.Plan {
			return a.Plan < b.Plan
		}
		if a.Period != b.Period {
			return a.Period < b.Period
		}
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}
		return a.Currency < b.Currency
	})
}

// ---------------------------------------------------------------------------
// Public query functions
// ---------------------------------------------------------------------------

// LookupBaseline returns the baseline Amount for (plan, period, tier).
// For TierDeveloped the baseline currency is "usd".
// For TierPPP this returns the first (and only) entry — prefer LookupPPPOption instead.
func LookupBaseline(p Plan, period Period, tier Tier) (Amount, bool) {
	for _, d := range allDescriptors {
		if d.Plan == p && d.Period == period && d.Tier == tier {
			return d.Baseline, true
		}
	}
	return Amount{}, false
}

// LookupPPPOption returns the Amount for a specific PPP currency.
func LookupPPPOption(p Plan, period Period, currency string) (Amount, bool) {
	k := pppKey{plan: p, period: period, currency: currency}
	amt, ok := pppAmounts[k]
	return amt, ok
}

// DevelopedCurrencyOptions returns the Options map for the developed-tier descriptor
// of (plan, period). The map contains all 7 developed-market currencies.
func DevelopedCurrencyOptions(p Plan, period Period) (map[string]Amount, bool) {
	for _, d := range allDescriptors {
		if d.Plan == p && d.Period == period && d.Tier == TierDeveloped {
			return d.Options, true
		}
	}
	return nil, false
}

// MustGet returns the Amount for (plan, period) in the requested currency,
// searching both developed Options maps and PPP entries. Panics on miss.
// Intended for tests and bootstrap tooling only.
func MustGet(p Plan, period Period, currency string) Amount {
	// Check developed options first.
	if opts, ok := DevelopedCurrencyOptions(p, period); ok {
		if amt, present := opts[currency]; present {
			return amt
		}
	}
	// Check PPP.
	if amt, ok := LookupPPPOption(p, period, currency); ok {
		return amt
	}
	panic(fmt.Sprintf("pricing: no amount for plan=%s period=%s currency=%s", p, period, currency))
}

// AllDescriptors returns every PriceDescriptor in the catalog.
// Used by the bootstrap CLI to push Price objects to Stripe.
func AllDescriptors() []PriceDescriptor {
	result := make([]PriceDescriptor, len(allDescriptors))
	copy(result, allDescriptors)
	return result
}

// MustGetDescriptor returns the PriceDescriptor for (plan, period, tier). Panics on miss.
// For TierPPP with multiple currencies, returns the first match — prefer direct map access.
// LookupKeyFor returns the canonical Stripe lookup_key for one price, and
// is the ONLY way anything outside this package should obtain one (#459).
//
// It exists because MustGetDescriptor keys on (plan, period, tier) alone,
// which is ambiguous for TierPPP: PPP descriptors are one-per-currency, so
// that lookup returns whichever happens to sort first. Callers worked
// around this by re-deriving the key with their own fmt.Sprintf, putting a
// second copy of the format in another package with nothing enforcing
// agreement. Change the format here and the copy kept writing the old
// string: no compile error, no failing test, prices silently missed.
//
// currency is ignored for TierDeveloped (one Price object carries all
// currency_options) and required for TierPPP.
func LookupKeyFor(p Plan, period Period, tier Tier, currency string) (string, bool) {
	for _, d := range allDescriptors {
		if d.Plan != p || d.Period != period || d.Tier != tier {
			continue
		}
		if tier == TierPPP && d.Currency != currency {
			continue
		}
		return d.LookupKey, true
	}
	return "", false
}

func MustGetDescriptor(p Plan, period Period, tier Tier) PriceDescriptor {
	for _, d := range allDescriptors {
		if d.Plan == p && d.Period == period && d.Tier == tier {
			return d
		}
	}
	panic(fmt.Sprintf("pricing: no descriptor for plan=%s period=%s tier=%s", p, period, tier))
}
