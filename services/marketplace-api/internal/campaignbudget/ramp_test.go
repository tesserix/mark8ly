package campaignbudget_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
)

func TestComputeRampDay_Boundaries(t *testing.T) {
	signup := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, 4, 1, 23, 59, 0, 0, time.UTC), 1},  // still day 1
		{time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), 2},    // rollover
		{time.Date(2026, 4, 3, 5, 0, 0, 0, time.UTC), 3},
		{time.Date(2026, 4, 4, 5, 0, 0, 0, time.UTC), 4},    // transition 3→4
		{time.Date(2026, 4, 7, 23, 0, 0, 0, time.UTC), 7},
		{time.Date(2026, 4, 8, 0, 5, 0, 0, time.UTC), 8},    // transition 7→8
		{time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 31},
	}
	for _, tc := range cases {
		got := campaignbudget.ComputeRampDay(signup, tc.now)
		require.Equal(t, tc.want, got, "signup=%s now=%s", signup, tc.now)
	}
}

func TestComputeRampDay_BeforeSignup(t *testing.T) {
	signup := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	require.Equal(t, 1, campaignbudget.ComputeRampDay(signup, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)),
		"clock skew before signup must clamp to day 1, not a negative day")
}

func TestIsRampTransitionDay(t *testing.T) {
	require.True(t, campaignbudget.IsRampTransitionDay(4))
	require.True(t, campaignbudget.IsRampTransitionDay(8))
	require.False(t, campaignbudget.IsRampTransitionDay(1))
	require.False(t, campaignbudget.IsRampTransitionDay(3))
	require.False(t, campaignbudget.IsRampTransitionDay(5))
	require.False(t, campaignbudget.IsRampTransitionDay(7))
}
