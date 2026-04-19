package appaddon

import (
	"math"
	"testing"
	"time"
)

func TestProrationCents_FullYear_Remaining(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	renewalAt := now.AddDate(1, 0, 0) // +365 days (non-leap reference)

	got := ProrationCents(now, renewalAt)
	// Full year: $2388 prorated + $2000 setup = $4388 = 438_800 cents.
	want := int64(438_800)
	if got != want {
		t.Errorf("ProrationCents(full year) = %d, want %d", got, want)
	}
}

func TestProrationCents_HalfYear_Remaining(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	renewalAt := now.Add(183 * 24 * time.Hour)

	got := ProrationCents(now, renewalAt)
	// 183/365 × 238_800 = 119_727.1232... → rounds to 119_727.
	// Then +200_000 setup = 319_727.
	want := int64(319_727)
	if diff := absI64(got - want); diff > 1 {
		t.Errorf("ProrationCents(half year) = %d, want ~%d (±1 for rounding)", got, want)
	}
}

func TestProrationCents_SameDay_OnlySetupFee(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := ProrationCents(now, now); got != 200_000 {
		t.Errorf("ProrationCents(same day) = %d, want 200_000", got)
	}
}

// Defensive: renewal in the past → no negative proration. Only the
// $2000 setup is charged. Prevents a data-corruption bug (e.g. anchor
// row with stale current_period_end) from silently refunding money.
func TestProrationCents_NegativeRemaining_ClampedToZero(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)

	if got := ProrationCents(now, past); got != 200_000 {
		t.Errorf("ProrationCents(past renewal) = %d, want 200_000", got)
	}
}

// Defensive: remaining > 365 (caller passed a two-years-out date) must
// not overcharge. Capped at one full year of app fees + setup.
func TestProrationCents_RemainingBeyondYear_ClampedTo365(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	twoYears := now.AddDate(2, 0, 0)

	if got := ProrationCents(now, twoYears); got != 438_800 {
		t.Errorf("ProrationCents(2 years) = %d, want 438_800 (clamped)", got)
	}
}

// Cents-only invariant — ProrationCents must never return a fractional
// currency unit. Covered by int64 return; verified here by exhaustive
// sampling against day grain.
func TestProrationCents_AlwaysIntegerCents(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d <= 365; d++ {
		renewal := now.Add(time.Duration(d) * 24 * time.Hour)
		got := ProrationCents(now, renewal)
		if got < 200_000 {
			t.Errorf("d=%d: got %d cents; proration must never undercut the setup fee", d, got)
		}
		if got > 438_800 {
			t.Errorf("d=%d: got %d cents; proration must never exceed full-year max", d, got)
		}
	}
}

func TestProrationCents_QuarterYear(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// ~91.25 days
	renewalAt := now.Add(91 * 24 * time.Hour)

	got := ProrationCents(now, renewalAt)
	// 91/365 × 238_800 = 59_535.6164... → 59_536; + 200_000 = 259_536.
	want := int64(259_536)
	if diff := absI64(got - want); diff > 1 {
		t.Errorf("ProrationCents(quarter) = %d, want ~%d (±1 for rounding)", got, want)
	}
}

func TestRoundHalfEven_Banker(t *testing.T) {
	cases := []struct {
		in   float64
		want int64
	}{
		{0.0, 0},
		{0.4, 0},
		{0.5, 0},   // even
		{1.5, 2},   // even
		{2.5, 2},   // even
		{3.5, 4},   // even
		{-0.5, 0},  // even
		{-1.5, -2}, // even
		{-2.5, -2}, // even
		{123.456, 123},
		{123.9, 124},
	}
	for _, tc := range cases {
		if got := roundHalfEven(tc.in); got != tc.want {
			t.Errorf("roundHalfEven(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func absI64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Sanity: the math constants match the spec. A refactor that changes these
// would silently slip past reviewers otherwise.
func TestPricingConstants_MatchSpec(t *testing.T) {
	if AppAnnualCents != 2388_00 {
		t.Errorf("AppAnnualCents = %d, want 238800 ($2388 = $199 × 12)", AppAnnualCents)
	}
	if SetupFeeCents != 2000_00 {
		t.Errorf("SetupFeeCents = %d, want 200000 ($2000)", SetupFeeCents)
	}
	// Double-check there's no float drift in the default constants.
	if math.Abs(float64(AppAnnualCents)-238_800.0) > 0.0001 {
		t.Fatal("AppAnnualCents float cast drift")
	}
}
