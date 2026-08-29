// Package pricing is the Go source of truth for all Stripe pricing.
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

// developedAmounts maps (plan, period) → per-currency Amount.
// Key: lowercase ISO 4217. Baseline currency is always "usd".
var developedAmounts = map[Plan]map[Period]map[string]Amount{
	PlanStarter: {
		PeriodMonthly: {
			"usd": {Currency: "usd", UnitAmountMinor: 1900},
			"cad": {Currency: "cad", UnitAmountMinor: 2500},
			"gbp": {Currency: "gbp", UnitAmountMinor: 1500},
			"eur": {Currency: "eur", UnitAmountMinor: 1700},
			"aud": {Currency: "aud", UnitAmountMinor: 2900, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 2900},
			"sgd": {Currency: "sgd", UnitAmountMinor: 2500},
		},
		PeriodAnnual: {
			// Spec §4.1: $182, C$239, £144, €163, A$278+GST, NZ$278, S$239
			"usd": {Currency: "usd", UnitAmountMinor: 18200},
			"cad": {Currency: "cad", UnitAmountMinor: 23900},
			"gbp": {Currency: "gbp", UnitAmountMinor: 14400},
			"eur": {Currency: "eur", UnitAmountMinor: 16300},
			"aud": {Currency: "aud", UnitAmountMinor: 27800, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 27800},
			"sgd": {Currency: "sgd", UnitAmountMinor: 23900},
		},
	},
	PlanStudio: {
		PeriodMonthly: {
			"usd": {Currency: "usd", UnitAmountMinor: 4900},
			"cad": {Currency: "cad", UnitAmountMinor: 6500},
			"gbp": {Currency: "gbp", UnitAmountMinor: 3900},
			"eur": {Currency: "eur", UnitAmountMinor: 4500},
			"aud": {Currency: "aud", UnitAmountMinor: 7500, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 7500},
			"sgd": {Currency: "sgd", UnitAmountMinor: 6500},
		},
		PeriodAnnual: {
			// Spec §4.1: $470, C$625, £375, €432, A$719+GST, NZ$719, S$623
			"usd": {Currency: "usd", UnitAmountMinor: 47000},
			"cad": {Currency: "cad", UnitAmountMinor: 62500},
			"gbp": {Currency: "gbp", UnitAmountMinor: 37500},
			"eur": {Currency: "eur", UnitAmountMinor: 43200},
			"aud": {Currency: "aud", UnitAmountMinor: 71900, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 71900},
			"sgd": {Currency: "sgd", UnitAmountMinor: 62300},
		},
	},
	PlanPro: {
		PeriodMonthly: {
			// Spec §4.1: monthly at +20% premium over annual-equivalent.
			// Annual: $1188/yr → $99/mo eq → monthly = $99 × 1.20 = $118.80 → $119
			// CAD: C$1619/yr → C$134.92/mo eq → ×1.20 = C$161.90 → C$162
			// GBP: £948/yr → £79/mo eq → ×1.20 = £94.80 → £95
			// EUR: €1068/yr → €89/mo eq → ×1.20 = €106.80 → €107
			// AUD: A$1788/yr → A$149/mo eq → ×1.20 = A$178.80 → A$179
			// NZD: NZ$1788/yr → NZ$149/mo eq → ×1.20 = NZ$178.80 → NZ$179
			// SGD: S$1548/yr → S$129/mo eq → ×1.20 = S$154.80 → S$155
			"usd": {Currency: "usd", UnitAmountMinor: 11900},
			"cad": {Currency: "cad", UnitAmountMinor: 16200},
			"gbp": {Currency: "gbp", UnitAmountMinor: 9500},
			"eur": {Currency: "eur", UnitAmountMinor: 10700},
			"aud": {Currency: "aud", UnitAmountMinor: 17900, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 17900},
			"sgd": {Currency: "sgd", UnitAmountMinor: 15500},
		},
		PeriodAnnual: {
			// Spec §4.1: $1,188/yr, C$1,619/yr, £948/yr, €1,068/yr, A$1,788/yr, NZ$1,788/yr, S$1,548/yr
			"usd": {Currency: "usd", UnitAmountMinor: 118800},
			"cad": {Currency: "cad", UnitAmountMinor: 161900},
			"gbp": {Currency: "gbp", UnitAmountMinor: 94800},
			"eur": {Currency: "eur", UnitAmountMinor: 106800},
			"aud": {Currency: "aud", UnitAmountMinor: 178800, TaxBehavior: "exclusive"},
			"nzd": {Currency: "nzd", UnitAmountMinor: 178800},
			"sgd": {Currency: "sgd", UnitAmountMinor: 154800},
		},
	},
}

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
// value, not ×100), so stripeUnitAmount() divides by 100 for it at the
// Stripe boundary. IDR is NOT zero-decimal per Stripe — its ×100 storage
// is simply the ordinary two-decimal representation and needs no special
// handling at the boundary.
//   VND 329,000 → UnitAmountMinor 32900000 → stripeUnitAmount → 329000
//   IDR 199,000 → UnitAmountMinor 19900000 → stripeUnitAmount → 19900000
// See zeroDecimalCurrencies and stripeUnitAmount() in
// internal/billing/stripe/price.go:12-31.
// ---------------------------------------------------------------------------

type pppKey struct {
	plan     Plan
	period   Period
	currency string
}

// pppAmounts holds every PPP price entry keyed by (plan, period, currency).
var pppAmounts = map[pppKey]Amount{
	// --- Starter monthly PPP ---
	// Spec §4.1.1: ₹999, RM59, ฿499, ₱749, Rp199,000, ₫329,000
	{PlanStarter, PeriodMonthly, "inr"}: {Currency: "inr", UnitAmountMinor: 99900},    // 999.00 INR
	{PlanStarter, PeriodMonthly, "myr"}: {Currency: "myr", UnitAmountMinor: 5900},     // 59.00 MYR
	{PlanStarter, PeriodMonthly, "thb"}: {Currency: "thb", UnitAmountMinor: 49900},    // 499.00 THB
	{PlanStarter, PeriodMonthly, "php"}: {Currency: "php", UnitAmountMinor: 74900},    // 749.00 PHP
	{PlanStarter, PeriodMonthly, "idr"}: {Currency: "idr", UnitAmountMinor: 19900000}, // 199,000 IDR (zero-decimal ×100)
	{PlanStarter, PeriodMonthly, "vnd"}: {Currency: "vnd", UnitAmountMinor: 32900000}, // 329,000 VND (zero-decimal ×100)

	// --- Starter annual PPP ---
	// Spec §4.1.1: ₹9,599, RM569, ฿4,799, ₱7,199, Rp1,919,000, ₫3,169,000
	{PlanStarter, PeriodAnnual, "inr"}: {Currency: "inr", UnitAmountMinor: 959900},    // 9,599.00 INR
	{PlanStarter, PeriodAnnual, "myr"}: {Currency: "myr", UnitAmountMinor: 56900},     // 569.00 MYR
	{PlanStarter, PeriodAnnual, "thb"}: {Currency: "thb", UnitAmountMinor: 479900},    // 4,799.00 THB
	{PlanStarter, PeriodAnnual, "php"}: {Currency: "php", UnitAmountMinor: 719900},    // 7,199.00 PHP
	{PlanStarter, PeriodAnnual, "idr"}: {Currency: "idr", UnitAmountMinor: 191900000}, // 1,919,000 IDR (zero-decimal ×100)
	{PlanStarter, PeriodAnnual, "vnd"}: {Currency: "vnd", UnitAmountMinor: 316900000}, // 3,169,000 VND (zero-decimal ×100)

	// --- Studio monthly PPP ---
	// Spec §4.1.1: ₹2,499, RM149, ฿1,199, ₱1,899, Rp499,000, ₫799,000
	{PlanStudio, PeriodMonthly, "inr"}: {Currency: "inr", UnitAmountMinor: 249900},   // 2,499.00 INR
	{PlanStudio, PeriodMonthly, "myr"}: {Currency: "myr", UnitAmountMinor: 14900},    // 149.00 MYR
	{PlanStudio, PeriodMonthly, "thb"}: {Currency: "thb", UnitAmountMinor: 119900},   // 1,199.00 THB
	{PlanStudio, PeriodMonthly, "php"}: {Currency: "php", UnitAmountMinor: 189900},   // 1,899.00 PHP
	{PlanStudio, PeriodMonthly, "idr"}: {Currency: "idr", UnitAmountMinor: 49900000}, // 499,000 IDR (zero-decimal ×100)
	{PlanStudio, PeriodMonthly, "vnd"}: {Currency: "vnd", UnitAmountMinor: 79900000}, // 799,000 VND (zero-decimal ×100)

	// --- Studio annual PPP ---
	// Spec §4.1.1: ₹23,999, RM1,429, ฿11,519, ₱18,239, Rp4,799,000, ₫7,699,000
	{PlanStudio, PeriodAnnual, "inr"}: {Currency: "inr", UnitAmountMinor: 2399900},   // 23,999.00 INR
	{PlanStudio, PeriodAnnual, "myr"}: {Currency: "myr", UnitAmountMinor: 142900},    // 1,429.00 MYR
	{PlanStudio, PeriodAnnual, "thb"}: {Currency: "thb", UnitAmountMinor: 1151900},   // 11,519.00 THB
	{PlanStudio, PeriodAnnual, "php"}: {Currency: "php", UnitAmountMinor: 1823900},   // 18,239.00 PHP
	{PlanStudio, PeriodAnnual, "idr"}: {Currency: "idr", UnitAmountMinor: 479900000}, // 4,799,000 IDR (zero-decimal ×100)
	{PlanStudio, PeriodAnnual, "vnd"}: {Currency: "vnd", UnitAmountMinor: 769900000}, // 7,699,000 VND (zero-decimal ×100)

	// --- Pro annual PPP ---
	// Spec §4.1.1: ₹65,999/yr, RM3,588/yr, ฿28,788/yr, ₱45,588/yr, Rp11,988,000/yr, ₫19,788,000/yr
	{PlanPro, PeriodAnnual, "inr"}: {Currency: "inr", UnitAmountMinor: 6599900},    // 65,999.00 INR
	{PlanPro, PeriodAnnual, "myr"}: {Currency: "myr", UnitAmountMinor: 358800},     // 3,588.00 MYR
	{PlanPro, PeriodAnnual, "thb"}: {Currency: "thb", UnitAmountMinor: 2878800},    // 28,788.00 THB
	{PlanPro, PeriodAnnual, "php"}: {Currency: "php", UnitAmountMinor: 4558800},    // 45,588.00 PHP
	{PlanPro, PeriodAnnual, "idr"}: {Currency: "idr", UnitAmountMinor: 1198800000}, // 11,988,000 IDR (zero-decimal ×100)
	{PlanPro, PeriodAnnual, "vnd"}: {Currency: "vnd", UnitAmountMinor: 1978800000}, // 19,788,000 VND (zero-decimal ×100)

	// --- Pro monthly PPP ---
	// Spec §4.1.1 lists only the annual Pro PPP figure for India (₹65,999/yr, ₹5,499/mo eq).
	// Monthly Pro PPP is available at +20% premium over annual-equivalent:
	//   INR: ₹5,499 × 1.20 = ₹6,598.80 → ₹6,599
	//   MYR: RM3,588/12=299 × 1.20 = RM358.80 → RM359
	//   THB: ฿28,788/12=2399 × 1.20 = ฿2,878.80 → ฿2,879
	//   PHP: ₱45,588/12=3799 × 1.20 = ₱4,558.80 → ₱4,559
	//   IDR: Rp11,988,000/12=999,000 × 1.20 = Rp1,198,800 → Rp1,198,800
	//   VND: ₫19,788,000/12=1,649,000 × 1.20 = ₫1,978,800 → ₫1,978,800
	{PlanPro, PeriodMonthly, "inr"}: {Currency: "inr", UnitAmountMinor: 659900},    // 6,599.00 INR
	{PlanPro, PeriodMonthly, "myr"}: {Currency: "myr", UnitAmountMinor: 35900},     // 359.00 MYR
	{PlanPro, PeriodMonthly, "thb"}: {Currency: "thb", UnitAmountMinor: 287900},    // 2,879.00 THB
	{PlanPro, PeriodMonthly, "php"}: {Currency: "php", UnitAmountMinor: 455900},    // 4,559.00 PHP
	{PlanPro, PeriodMonthly, "idr"}: {Currency: "idr", UnitAmountMinor: 119880000}, // 1,198,800 IDR (zero-decimal ×100)
	{PlanPro, PeriodMonthly, "vnd"}: {Currency: "vnd", UnitAmountMinor: 197880000}, // 1,978,800 VND (zero-decimal ×100)
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
