package pricing

// This file exists for two reasons, kept together because both exist only
// to let cmd/genpricing emit packages/ui/src/subscription/pricing-data.ts:
//
//  1. Thin accessors onto catalog.go's unexported tables (developedAmounts,
//     pppAmounts). They live here, not in catalog.go, so the #393 comment
//     fix in catalog.go stays comment-only.
//  2. The display-only values billing genuinely does not own: AED/JPY have
//     no Stripe Price object (out of v2.3 launch scope), the Pro+App add-on
//     has no billing.Plan of its own, and there is no catalog concept of
//     "which currencies does the public pricing page show, in what order."
//     These are declared here, explicitly, rather than invented inside the
//     generator where they would be invisible to the next reader.

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
var DisplayCurrencyOrder = []string{
	"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd", "inr", "aed", "jpy",
}

// FallbackCurrencies lists the TS-table currencies with no Stripe Price
// object at all. Per spec §4.1, AED (UAE) and JPY (not in the 18-country
// list) are out of v2.3 launch scope; the TS table retains them only as
// USD-fallback rows for display, with signup blocked for those countries
// until a later version. Because billing never bills in these currencies,
// catalog.go correctly has no data for them — cmd/genpricing falls back to
// each plan's USD developed-tier amount for every currency in this list.
var FallbackCurrencies = []string{"aed", "jpy"}

// FallbackComment names the exact trailing comment cmd/genpricing must emit
// for a given plan's fallback-currency row. The committed pricing-data.ts
// uses different wording per plan: Starter's AED says "USD fallback,
// deferred" and its JPY says "USD fallback, not in scope", while Studio and
// Pro just say "USD fallback" for both currencies. That asymmetry is
// pre-existing prose with no derivable rule behind it, so it is pinned here
// verbatim rather than invented by the generator.
var FallbackComment = map[Plan]map[string]string{
	PlanStarter: {
		"aed": "USD fallback, deferred",
		"jpy": "USD fallback, not in scope",
	},
	PlanStudio: {
		"aed": "USD fallback",
		"jpy": "USD fallback",
	},
	PlanPro: {
		"aed": "USD fallback",
		"jpy": "USD fallback",
	},
}

// ProAppMonthly and ProAppAnnual are the Pro+App add-on's single global
// price, per spec §4.1.2: billed in USD only, everywhere. There is no
// Stripe Price object for it — no PlanProApp constant exists in catalog.go —
// so it cannot be derived from the catalog. The TS table nonetheless
// repeats this same figure across every display currency (DisplayCurrencyOrder)
// for a consistent card layout; the display layer annotates "billed in USD"
// for non-USD viewers. The $2,000 setup fee is a separate one-off charge,
// not represented here.
var (
	ProAppMonthly = Amount{Currency: "usd", UnitAmountMinor: 19900}
	ProAppAnnual  = Amount{Currency: "usd", UnitAmountMinor: 238800}
)
