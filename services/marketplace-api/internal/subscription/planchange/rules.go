// Package planchange orchestrates §4.4 + §4.5 plan and period changes.
// rules.go holds the pure-function decision helpers; orchestration and DB
// writes live in planchange.go.
package planchange

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Direction classifies a plan/period change relative to the current row.
type Direction string

const (
	DirectionUpgrade   Direction = "upgrade"
	DirectionDowngrade Direction = "downgrade"
	DirectionNoChange  Direction = "no_change"
)

// planRank returns a lattice ordering: trial < starter < studio < pro.
// Marketplace is treated as its own top tier (returns 5) but is not
// user-reachable via the change-plan endpoint.
func planRank(p subscription.SubscriptionPlan) int {
	switch p {
	case subscription.PlanTrial:
		return 0
	case subscription.PlanStarter:
		return 1
	case subscription.PlanStudio:
		return 2
	case subscription.PlanPro:
		return 3
	case subscription.PlanMarketplace:
		return 5
	default:
		return -1
	}
}

// IsUpgrade reports whether (from → to) increases the plan rank.
func IsUpgrade(from, to subscription.SubscriptionPlan) bool {
	return planRank(to) > planRank(from)
}

// IsPeriodUpgrade reports whether switching billing periods is an upgrade.
// Monthly → Annual is treated as an upgrade (immediate + prorate credit
// per §4.4). Annual → Monthly is a downgrade (end-of-period).
func IsPeriodUpgrade(from, to subscription.SubscriptionPeriod) bool {
	return from == subscription.PeriodMonthly && to == subscription.PeriodAnnual
}

// Classify returns the overall direction of a (plan, period) → (plan, period)
// change. Plan direction dominates — a Studio→Starter change is always a
// downgrade regardless of period direction.
func Classify(
	fromPlan subscription.SubscriptionPlan,
	fromPeriod subscription.SubscriptionPeriod,
	toPlan subscription.SubscriptionPlan,
	toPeriod subscription.SubscriptionPeriod,
) Direction {
	if fromPlan == toPlan && fromPeriod == toPeriod {
		return DirectionNoChange
	}
	if IsUpgrade(fromPlan, toPlan) {
		return DirectionUpgrade
	}
	if planRank(toPlan) < planRank(fromPlan) {
		return DirectionDowngrade
	}
	// Same plan, period change only.
	if IsPeriodUpgrade(fromPeriod, toPeriod) {
		return DirectionUpgrade
	}
	return DirectionDowngrade
}

// RequiresStoreCountCheck reports whether the given plan transition needs the
// preflight store-count guard (§4.5.1). Only Studio→Starter enforces it in v1.
func RequiresStoreCountCheck(from, to subscription.SubscriptionPlan) bool {
	return from == subscription.PlanStudio && to == subscription.PlanStarter
}

// EffectiveAt returns (when, immediate) for a given direction.
// Upgrades apply now; downgrades wait until current period end.
func EffectiveAt(dir Direction, now, currentPeriodEnd time.Time) (time.Time, bool) {
	if dir == DirectionUpgrade {
		return now, true
	}
	return currentPeriodEnd, false
}
