package pricing

// This file exists for two reasons, kept together because both exist only
// to let cmd/genpricing emit packages/ui/src/subscription/pricing-data.ts:
//
//  1. Thin accessors onto catalog.go's unexported tables (developedAmounts,
//     pppAmounts). They live here, not in catalog.go, so the #393 comment
//     fix in catalog.go stays comment-only.
//  2. The display-only values billing genuinely does not own: the Pro+App
//     add-on has no billing.Plan of its own, and there is no catalog
//     concept of "which currencies does the public pricing page show, in
//     what order." These are declared here, explicitly, rather than
//     invented inside the generator where they would be invisible to the
//     next reader.

// DevelopedAmount returns the developed-tier amount for (plan, period,
// currency) and whether it exists. Accessor onto catalog.go's unexported
// developedAmounts.
func DevelopedAmount(plan Plan, period Period, currency string) (Amount, bool) {
	byPeriod, ok := developedAmounts[plan]
	if !ok {
		return Amount{}, false
	}
	byCurrency, ok := byPeriod[period]
	if !ok {
		return Amount{}, false
	}
	amt, ok := byCurrency[currency]
	return amt, ok
}

// PPPAmount returns the PPP-tier amount for (plan, period, currency) and
// whether it exists. Accessor onto catalog.go's unexported pppAmounts.
//
// The TS pricing table carries exactly one PPP currency among its rows:
// INR. MYR, THB, PHP, IDR, and VND exist in pppAmounts but MUST NOT be
// added to the TS table — they are absent today, and adding them would
// silently widen what marketing surfaces can display. This accessor makes
// that a caller-side choice (cmd/genpricing only ever calls it with "inr")
// rather than exporting the whole PPP table.
func PPPAmount(plan Plan, period Period, currency string) (Amount, bool) {
	amt, ok := pppAmounts[pppKey{plan: plan, period: period, currency: currency}]
	return amt, ok
}

// DisplayCurrencyOrder is the ordered list of currencies the public TS
// pricing table shows, matching packages/ui/src/subscription/pricing-data.ts.
// Billing's catalog has no notion of "display order for a public page" —
// this is a presentation decision the generator needs but catalog.go has no
// reason to encode.
//
// AED and JPY are deliberately absent: we do not serve the Arab or
// Japanese markets they represent (no tested shipping carrier — see
// apps/onboarding/app/onboarding/page.tsx's carrier allowlist), and the
// rows that used to sit here carried the raw USD integer under an AED/JPY
// label, quoting roughly 27% and a fraction of the real price
// respectively. Removing them lets an AE/JP visitor fall through to the
// USD row, which the display layer renders labelled USD — the honest
// answer for a market we do not sell in.
var DisplayCurrencyOrder = []string{
	"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd", "inr",
}

// FallbackCurrencies lists TS-table currencies with no Stripe Price object
// at all, for which cmd/genpricing falls back to each plan's USD
// developed-tier amount. Empty today: AED and JPY were the only such
// currencies, and both were removed from DisplayCurrencyOrder above rather
// than kept as fallback rows, because a fallback row displays as if it
// were a real, if approximate, local price. Kept as a mechanism (not
// deleted) because a future currency may legitimately need the same
// USD-fallback treatment.
var FallbackCurrencies []string

// FallbackComment names the exact trailing comment cmd/genpricing must emit
// for a given plan's fallback-currency row, keyed by plan then currency.
// Empty today — see FallbackCurrencies.
var FallbackComment = map[Plan]map[string]string{}

// ProAppMonthly and ProAppAnnual are the Pro+App add-on's single global
// price, per spec §4.1.2: billed in USD only, everywhere. There is no
// Stripe Price object for it — no PlanProApp constant exists in catalog.go —
// so it cannot be derived from the catalog.
//
// That absence is INTENDED, not a gap (#650): the add-on is a one-time
// purchase billed through a single invoice, and a one-off charge needs no
// Price object. These two constants are display and proration inputs, not
// a subscription's price. See internal/billing/appaddon for the decision
// and what the proration is actually for.
//
// The TS table nonetheless
// repeats this same figure across every display currency (DisplayCurrencyOrder)
// for a consistent card layout; the display layer annotates "billed in USD"
// for non-USD viewers. The $2,000 setup fee is a separate one-off charge,
// not represented here.
var (
	ProAppMonthly = Amount{Currency: "usd", UnitAmountMinor: 19900}
	ProAppAnnual  = Amount{Currency: "usd", UnitAmountMinor: 238800}
)
