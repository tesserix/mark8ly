package promo

import (
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// BasePriceMinorFor returns the undiscounted recurring price, in minor units,
// that ApplyInput.BasePriceMinor must carry for the §7.4 absolute-floor check
// to mean anything.
//
// Every caller used to pass a literal 0 with a comment saying the floor "keys
// off plan + currency". It does not. Validate computes the discounted price
// from BasePriceMinor and compares THAT against the floor, so a zero base
// yields a zero effective price, which is below every floor in
// PlanAbsoluteFloorMinor. The result was that no store on starter, studio or
// pro could redeem any promo code at all — the request came back
// below_absolute_floor whatever the code said — while a store still on the
// trial plan was accepted, because trial has no floor entry. The floor was
// therefore rejecting exactly the merchants it was written to price-protect
// and waving through the ones it does not cover.
//
// A miss returns 0. That is the previous behaviour, and it is the safe
// direction: for a plan with a floor it rejects, and for trial/marketplace —
// the plans with no Stripe Price and no floor — the value is not consulted.
func BasePriceMinorFor(
	plan subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
	tier subscription.PriceTier,
	currency string,
) int64 {
	p, ok := pricingPlan(plan)
	if !ok {
		return 0
	}
	cur := strings.ToLower(strings.TrimSpace(currency))
	if cur == "" {
		return 0
	}
	per := pricing.PeriodMonthly
	if period == subscription.PeriodAnnual {
		per = pricing.PeriodAnnual
	}

	// Both tables are consulted regardless of the row's price_tier, in the
	// tier's own order. price_tier records which catalog the merchant was
	// SOLD from, and a currency can be reached from either table (a PPP-tier
	// row billed in usd, say). Trusting the column alone would return 0 —
	// "no price" — for a subscription that plainly has one.
	if tier == subscription.PriceTierPPP {
		if amt, found := pricing.LookupPPPOption(p, per, cur); found {
			return amt.UnitAmountMinor
		}
	}
	if opts, found := pricing.DevelopedCurrencyOptions(p, per); found {
		if amt, present := opts[cur]; present {
			return amt.UnitAmountMinor
		}
	}
	if amt, found := pricing.LookupPPPOption(p, per, cur); found {
		return amt.UnitAmountMinor
	}
	return 0
}

// pricingPlan maps a subscription plan onto the pricing catalog's plan.
// trial and marketplace have no Stripe Price object and no floor entry, so
// they map to nothing rather than to a zero price.
func pricingPlan(plan subscription.SubscriptionPlan) (pricing.Plan, bool) {
	switch plan {
	case subscription.PlanStarter:
		return pricing.PlanStarter, true
	case subscription.PlanStudio:
		return pricing.PlanStudio, true
	case subscription.PlanPro:
		return pricing.PlanPro, true
	default:
		return "", false
	}
}
