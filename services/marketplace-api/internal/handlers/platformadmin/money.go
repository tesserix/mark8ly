package platformadmin

import (
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// money is the wire representation of a resolved subscription price.
// Currency is always uppercase ISO 4217 on the wire.
type money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// resolveMoney returns the catalog price for a subscription's plan, period
// and currency, or ok=false when no price can be determined.
//
// ok=false is a normal outcome, not an error:
//   - billing_currency is NULL (the merchant never chose one), or
//   - the plan has no price at all — the catalog excludes `trial` and
//     `marketplace` by design ("no Price objects"), or
//   - the catalog has no entry for that plan/period/currency combination.
//
// Callers OMIT the amount key entirely in that case. Never null, never 0,
// never a guessed currency: a number the system cannot determine must not
// be reported as a number.
//
// Deliberately does NOT use pricing.MustGet, which panics on a miss. A
// console read must not panic on an unpriced combination. This calls the
// same two lookups MustGet wraps, minus the panic.
func resolveMoney(plan, period string, billingCurrency *string, tier subscription.PriceTier) (money, bool) {
	if billingCurrency == nil || strings.TrimSpace(*billingCurrency) == "" {
		return money{}, false
	}
	// Catalog keys are lowercase ISO 4217; the column is char(3) of
	// unspecified case; the wire contract is uppercase.
	cur := strings.ToLower(strings.TrimSpace(*billingCurrency))

	p, per := pricing.Plan(plan), pricing.Period(period)

	if tier == subscription.PriceTierPPP {
		if amt, ok := pricing.LookupPPPOption(p, per, cur); ok {
			return money{Amount: amt.UnitAmountMinor, Currency: strings.ToUpper(amt.Currency)}, true
		}
		return money{}, false
	}

	if opts, ok := pricing.DevelopedCurrencyOptions(p, per); ok {
		if amt, present := opts[cur]; present {
			return money{Amount: amt.UnitAmountMinor, Currency: strings.ToUpper(amt.Currency)}, true
		}
	}
	return money{}, false
}
