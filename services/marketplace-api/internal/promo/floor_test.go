package promo

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestCheckFloor(t *testing.T) {
	tests := []struct {
		name           string
		plan           subscription.SubscriptionPlan
		currency       string
		effectiveMinor int64
		wantErr        error
	}{
		// USD — Starter floor $12 = 1200 minor
		{"starter USD above floor", subscription.PlanStarter, "usd", 1200, nil},
		{"starter USD at floor", subscription.PlanStarter, "usd", 1200, nil},
		{"starter USD below floor", subscription.PlanStarter, "usd", 1199, ErrBelowAbsoluteFloor},
		// INR — Starter floor ₹800 = 80000 minor
		{"starter INR above floor", subscription.PlanStarter, "inr", 80000, nil},
		{"starter INR below floor", subscription.PlanStarter, "inr", 79999, ErrBelowAbsoluteFloor},
		// Studio USD
		{"studio USD at floor", subscription.PlanStudio, "usd", 3000, nil},
		{"studio USD below floor", subscription.PlanStudio, "usd", 2999, ErrBelowAbsoluteFloor},
		// Pro USD
		{"pro USD at floor", subscription.PlanPro, "usd", 7500, nil},
		{"pro USD below floor", subscription.PlanPro, "usd", 7499, ErrBelowAbsoluteFloor},
		// Currency not covered
		{"starter EUR not covered", subscription.PlanStarter, "eur", 999, ErrCurrencyNotCovered},
		// Currency normalisation — uppercase accepted
		{"starter USD uppercase", subscription.PlanStarter, "USD", 1200, nil},
		// Plans with no floor — trial + marketplace always pass
		{"trial no floor", subscription.PlanTrial, "usd", 0, nil},
		{"marketplace no floor", subscription.PlanMarketplace, "usd", 0, nil},
		// Zero effective price — should fail for plans with a floor
		{"starter USD zero", subscription.PlanStarter, "usd", 0, ErrBelowAbsoluteFloor},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckFloor(tc.plan, tc.currency, tc.effectiveMinor)
			if err != tc.wantErr {
				t.Errorf("CheckFloor(%q,%q,%d) = %v, want %v",
					tc.plan, tc.currency, tc.effectiveMinor, err, tc.wantErr)
			}
		})
	}
}

func TestComputeEffective(t *testing.T) {
	tests := []struct {
		name          string
		base          int64
		percentOffBps int
		amountOff     int64
		want          int64
	}{
		{"50% off 2000", 2000, 5000, 0, 1000},
		{"no discount", 2000, 0, 0, 2000},
		{"flat 500 off 2000", 2000, 0, 500, 1500},
		{"100% off", 2000, 10000, 0, 0},
		{"overshoot flat — floor at 0", 500, 0, 1000, 0},
		{"10% off 999", 999, 1000, 0, 899},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeEffective(tc.base, tc.percentOffBps, tc.amountOff)
			if got != tc.want {
				t.Errorf("ComputeEffective(%d, %d, %d) = %d, want %d",
					tc.base, tc.percentOffBps, tc.amountOff, got, tc.want)
			}
		})
	}
}
