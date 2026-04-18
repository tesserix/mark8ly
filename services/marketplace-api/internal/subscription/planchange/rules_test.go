package planchange_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
)

func TestIsUpgrade(t *testing.T) {
	cases := []struct {
		name      string
		from, to  subscription.SubscriptionPlan
		isUpgrade bool
	}{
		{"starter→studio", subscription.PlanStarter, subscription.PlanStudio, true},
		{"studio→pro", subscription.PlanStudio, subscription.PlanPro, true},
		{"studio→starter", subscription.PlanStudio, subscription.PlanStarter, false},
		{"pro→studio", subscription.PlanPro, subscription.PlanStudio, false},
		{"starter→starter", subscription.PlanStarter, subscription.PlanStarter, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.isUpgrade, planchange.IsUpgrade(tc.from, tc.to))
		})
	}
}

func TestIsPeriodUpgrade(t *testing.T) {
	require.True(t, planchange.IsPeriodUpgrade(subscription.PeriodMonthly, subscription.PeriodAnnual))
	require.False(t, planchange.IsPeriodUpgrade(subscription.PeriodAnnual, subscription.PeriodMonthly))
	require.False(t, planchange.IsPeriodUpgrade(subscription.PeriodMonthly, subscription.PeriodMonthly))
}

func TestRequiresStoreCountCheck_OnlyStudioToStarter(t *testing.T) {
	require.True(t, planchange.RequiresStoreCountCheck(subscription.PlanStudio, subscription.PlanStarter))
	require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanPro, subscription.PlanStarter), "Pro→Starter double-jump is out of scope — one step at a time")
	require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanStudio, subscription.PlanStudio))
	require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanStarter, subscription.PlanStudio), "upgrade never blocks on store count")
}

func TestEffectiveAt_UpgradeIsNow(t *testing.T) {
	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	periodEnd := now.Add(20 * 24 * time.Hour)
	at, immediate := planchange.EffectiveAt(planchange.DirectionUpgrade, now, periodEnd)
	require.True(t, immediate)
	require.Equal(t, now, at)
}

func TestEffectiveAt_DowngradeIsPeriodEnd(t *testing.T) {
	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	periodEnd := now.Add(20 * 24 * time.Hour)
	at, immediate := planchange.EffectiveAt(planchange.DirectionDowngrade, now, periodEnd)
	require.False(t, immediate)
	require.Equal(t, periodEnd, at)
}

func TestClassify_NoOp_WhenIdentical(t *testing.T) {
	d := planchange.Classify(subscription.PlanStudio, subscription.PeriodMonthly,
		subscription.PlanStudio, subscription.PeriodMonthly)
	require.Equal(t, planchange.DirectionNoChange, d)
}

func TestClassify_PlanDowngrade_BeatsPeriodDirection(t *testing.T) {
	// Studio/annual → Starter/annual is a downgrade regardless of the same period.
	d := planchange.Classify(subscription.PlanStudio, subscription.PeriodAnnual,
		subscription.PlanStarter, subscription.PeriodAnnual)
	require.Equal(t, planchange.DirectionDowngrade, d)
}

func TestClassify_SamePlan_PeriodUpgrade(t *testing.T) {
	d := planchange.Classify(subscription.PlanStarter, subscription.PeriodMonthly,
		subscription.PlanStarter, subscription.PeriodAnnual)
	require.Equal(t, planchange.DirectionUpgrade, d)
}
