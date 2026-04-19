// Package appaddon computes the co-termination proration for the white-label
// mobile app add-on (spec §3.4) and exposes the Pro-gated purchase endpoint.
//
// This file is pure math — no I/O, no database, no Stripe calls.
package appaddon

import "time"

// Pricing constants, per spec §3.4:
//
//   - App subscription : $199 / month × 12 months = $2388 / year
//   - Setup fee        : $2000 one-time, payable at add-on purchase regardless
//     of proration
//
// Amounts are in USD cents (int64) to avoid floating-point rounding drift.
const (
	// AppAnnualCents is one Pro year of white-label app fees ($199 × 12 = $2388).
	AppAnnualCents int64 = 2388_00

	// SetupFeeCents is the one-time white-label app setup charge ($2000).
	SetupFeeCents int64 = 2000_00
)

// ProrationCents returns the total up-front charge in USD cents for an
// add-on purchase at `now` when the Pro anchor subscription renews at
// `renewalAt`.
//
// Formula (§3.4):
//
//	(remaining_days / 365) × $2388 + $2000
//
// where remaining_days is clamped to [0, 365]:
//
//   - If renewalAt is in the past (remaining < 0), proration is zero and
//     only the setup fee is charged. This is the defensive path for a stale
//     or broken anchor subscription row.
//   - If remaining > 365 (e.g. a just-renewed anchor), it's clamped to 365
//     so we never charge for more than one anchor year.
//   - If remaining == 0 (buying on the exact renewal day), the caller pays
//     only the setup fee and the add-on co-terminates on the same day as
//     the upcoming renewal, bundling into the next invoice.
//
// Rounding: half-to-nearest-even on the prorated component before the setup
// fee is added. The setup fee is added as-is.
func ProrationCents(now, renewalAt time.Time) int64 {
	remainingDays := renewalAt.Sub(now).Hours() / 24.0
	switch {
	case remainingDays < 0:
		remainingDays = 0
	case remainingDays > 365:
		remainingDays = 365
	}

	prorated := float64(AppAnnualCents) * (remainingDays / 365.0)
	return roundHalfEven(prorated) + SetupFeeCents
}

// roundHalfEven rounds a float64 to the nearest int64 using banker's
// rounding (round-half-to-even). Used by ProrationCents so cumulative
// error is bounded across many co-terminated renewals.
func roundHalfEven(x float64) int64 {
	floor := int64(x)
	frac := x - float64(floor)

	if x < 0 {
		// Go's int64() truncates towards zero, so negative frac has the
		// opposite sign we want. Flip the sign and use the positive path.
		return -roundHalfEven(-x)
	}

	switch {
	case frac < 0.5:
		return floor
	case frac > 0.5:
		return floor + 1
	default:
		// Exactly 0.5 — round to the even neighbour.
		if floor%2 == 0 {
			return floor
		}
		return floor + 1
	}
}
