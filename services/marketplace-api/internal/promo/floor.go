package promo

import (
	"strings"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// PlanAbsoluteFloorMinor is the static per-plan, per-currency absolute price
// floor in minor units. A discounted subscription price must not fall below
// this value after all promotions (§7.4, Council finding #5).
//
// Values: Starter $12 USD / Studio $30 USD / Pro $75 USD;
//         Starter ₹800 INR / Studio ₹1,800 INR / Pro ₹4,200 INR.
//
// Currency codes are lower-cased for lookup normalisation. Plans not in this
// map (trial, marketplace) have no floor — they are not publicly purchasable.
var PlanAbsoluteFloorMinor = map[subscription.SubscriptionPlan]map[string]int64{
	subscription.PlanStarter: {
		"usd": 1200,  // $12.00
		"inr": 80000, // ₹800.00 (minor = paise × 100? No — Stripe INR uses paise, 800 rupees = 80000 paise)
	},
	subscription.PlanStudio: {
		"usd": 3000,   // $30.00
		"inr": 180000, // ₹1,800.00
	},
	subscription.PlanPro: {
		"usd": 7500,   // $75.00
		"inr": 420000, // ₹4,200.00
	},
}

// CheckFloor returns ErrBelowAbsoluteFloor when effectiveMinor is below the
// floor for (plan, currency), or ErrCurrencyNotCovered when the currency is
// not in the floor table for the given plan.
// Plans without a floor entry (trial, marketplace) always return nil.
//
// effectiveMinor must be in the same minor-unit denomination as the floor table.
func CheckFloor(plan subscription.SubscriptionPlan, currency string, effectiveMinor int64) error {
	currencyKey := strings.ToLower(strings.TrimSpace(currency))

	floors, hasPlan := PlanAbsoluteFloorMinor[plan]
	if !hasPlan {
		// No floor defined for this plan (trial/marketplace) — always passes.
		return nil
	}

	floor, hasCurrency := floors[currencyKey]
	if !hasCurrency {
		return ErrCurrencyNotCovered
	}

	if effectiveMinor < floor {
		return ErrBelowAbsoluteFloor
	}
	return nil
}

// ComputeEffective computes the effective price after a discount, in minor
// units. percentOffBps is basis points (5000 = 50.00%); amountOffMinor is a
// flat reduction in minor units. Exactly one of the two should be non-zero
// depending on discount_type. Result is floored at 0 (never negative).
func ComputeEffective(baseMinor int64, percentOffBps int, amountOffMinor int64) int64 {
	effective := baseMinor
	if percentOffBps > 0 {
		// Integer arithmetic: (base * (10000 - bps)) / 10000.
		effective = baseMinor * int64(10000-percentOffBps) / 10000
	}
	effective -= amountOffMinor
	if effective < 0 {
		effective = 0
	}
	return effective
}
