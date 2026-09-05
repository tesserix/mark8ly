package consolepromo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// CreatedBy is written to promo_codes.created_by for every row this ingest
// writes. It is the marker that says a row's definition lives in the console
// and will be overwritten from there on the next sync.
//
// It is also the scope of the expiry sweep: only rows carrying this marker
// are ever expired for being absent from the catalog, so a code created by
// any other path is untouched by this package.
const CreatedBy = "console:promo-catalog"

// DefaultMaxPerEmail is written explicitly rather than left to the column
// default.
//
// The console cannot express a per-email limit, so mark8ly's own policy
// applies (§7.3; flagged on #726, not changed here). Writing it explicitly
// rather than relying on promo_codes.max_per_email DEFAULT 1 is deliberate:
// the field is a plain int on the model, and an int zero handed to the
// database would mean "this code may never be redeemed by anyone" — a
// silently dead code rather than a visible error.
const DefaultMaxPerEmail = 1

// Code length bounds enforced by the schema: char_length(code) >= 4 from
// migration 000131 (a console code is a human campaign code such as
// "LAUNCH50", so promo.MinCodeLength's 12 cannot apply to it) and
// varchar(64) from 000060.
//
// Both are checked here so a bad definition is rejected with a named reason
// rather than surfacing as a raw Postgres constraint violation that takes
// the whole batch's write with it.
const (
	minConsoleCodeLength = 4
	maxConsoleCodeLength = 64
)

// Reason names why a code was rejected. It is a closed set because it is a
// Prometheus label value; an unbounded reason string would make the metric a
// cardinality bomb.
type Reason string

const (
	ReasonCodeLength        Reason = "code_length"
	ReasonNoBenefit         Reason = "no_benefit"
	ReasonTrialExtension    Reason = "bad_trial_extension"
	ReasonDiscountKind      Reason = "bad_discount_kind"
	ReasonPercentOff        Reason = "bad_percent_off"
	ReasonAmountOff         Reason = "bad_amount_off"
	ReasonDuration          Reason = "bad_duration"
	ReasonDurationInMonths  Reason = "bad_duration_in_months"
	ReasonEmptyStripeCoupon Reason = "empty_stripe_coupon_id"
	ReasonUnknown           Reason = "unknown"
)

// RejectedError is a mapping failure carrying the reason it happened.
//
// One malformed definition must never abort an ingest — the other codes in
// the batch are fine and a merchant is waiting on them — so this is returned
// per code and counted, never propagated as a batch failure.
type RejectedError struct {
	Reason Reason
	Detail string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("consolepromo: rejected (%s): %s", e.Reason, e.Detail)
}

func reject(r Reason, format string, args ...any) error {
	return &RejectedError{Reason: r, Detail: fmt.Sprintf(format, args...)}
}

// ReasonOf returns the Reason carried by err, or ReasonUnknown when err is
// not a RejectedError. Metrics label on this, so it must always answer.
func ReasonOf(err error) Reason {
	if err == nil {
		return ""
	}
	var re *RejectedError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ReasonUnknown
}

// MapCode converts one published definition into the row this service
// stores. It is a PURE function of (in, now): no database, no clock, no
// network, no logging — which is what lets every branch below, including
// every rejection, be tested directly.
//
// now supplies valid_from when the console did not bound the start, matching
// the column's own DEFAULT now().
//
// It rejects, with a named reason, everything the database would reject
// anyway — a code with no benefit at all, a half-specified discount, a code
// outside the length bounds — because a mapper's error naming the offending
// code is a far better diagnostic than a raw SQLSTATE 23514 arriving from a
// batch insert with no indication of which row caused it.
func MapCode(in Code, now time.Time) (promo.PromoCode, error) {
	code := strings.TrimSpace(in.Code)
	if n := len([]rune(code)); n < minConsoleCodeLength || n > maxConsoleCodeLength {
		return promo.PromoCode{}, reject(ReasonCodeLength,
			"code %q is %d characters; the schema allows %d..%d",
			code, n, minConsoleCodeLength, maxConsoleCodeLength)
	}

	out := promo.PromoCode{
		Code:        code,
		MaxPerEmail: DefaultMaxPerEmail,
		CreatedBy:   CreatedBy,
		ValidUntil:  in.ValidUntil,
		// MaxRedemptions nil means unlimited on both sides, so it passes
		// through untouched.
		MaxRedemptions: in.MaxRedemptions,
	}

	if in.ValidFrom != nil {
		out.ValidFrom = *in.ValidFrom
	} else {
		out.ValidFrom = now
	}

	if in.TrialExtensionDays != nil {
		days := *in.TrialExtensionDays
		if days <= 0 {
			// The console's own constraint is > 0, and so is
			// promo_codes_trial_extension_days_positive. A zero here is a
			// code that claims to extend a trial by nothing.
			return promo.PromoCode{}, reject(ReasonTrialExtension,
				"code %q: trial_extension_days is %d, which must be positive", code, days)
		}
		out.TrialExtensionDays = &days
	}

	if in.Discount != nil {
		if err := applyDiscount(&out, code, *in.Discount); err != nil {
			return promo.PromoCode{}, err
		}
	}

	// promo_codes_has_benefit: a row must deliver SOMETHING. Without this a
	// code would be accepted at redemption and do nothing at all.
	if out.TrialExtensionDays == nil && out.DiscountType == nil {
		return promo.PromoCode{}, reject(ReasonNoBenefit,
			"code %q carries neither a discount nor a trial extension", code)
	}

	out.CreatedAt, out.UpdatedAt = now, now
	return out, nil
}

// applyDiscount fills the discount half of the row. It writes DiscountType
// and DiscountValue together or not at all, which is what
// promo_codes_discount_pair requires.
func applyDiscount(out *promo.PromoCode, code string, d Discount) error {
	switch d.Kind {
	case DiscountKindPercentOff:
		bp, err := percentToBasisPoints(d.PercentOff)
		if err != nil {
			return reject(ReasonPercentOff, "code %q: %v", code, err)
		}
		t := promo.DiscountTypePercentage
		out.DiscountType, out.DiscountValue = &t, &bp
	case DiscountKindAmountOff:
		if d.AmountOffMinor == nil {
			return reject(ReasonAmountOff,
				"code %q: kind is %q but amount_off_minor is absent", code, d.Kind)
		}
		if *d.AmountOffMinor <= 0 {
			// promo_codes.discount_value CHECK (discount_value > 0).
			return reject(ReasonAmountOff,
				"code %q: amount_off_minor is %d, which must be positive", code, *d.AmountOffMinor)
		}
		if *d.AmountOffMinor > math.MaxInt32 {
			// discount_value is a Postgres INTEGER, i.e. 32-bit, while the
			// console publishes an unbounded JSON number. Checked against
			// MaxInt32 rather than against Go's int, which is 64-bit here and
			// would let the value through to fail as a raw 22003 at insert
			// time — taking the batch's write with it.
			return reject(ReasonAmountOff,
				"code %q: amount_off_minor %d does not fit the discount_value integer column",
				code, *d.AmountOffMinor)
		}
		v := int(*d.AmountOffMinor)
		t := promo.DiscountTypeAmount
		out.DiscountType, out.DiscountValue = &t, &v
	default:
		return reject(ReasonDiscountKind,
			"code %q: discount kind %q is neither %q nor %q",
			code, d.Kind, DiscountKindPercentOff, DiscountKindAmountOff)
	}

	months, err := durationToMonths(d)
	if err != nil {
		return reject(reasonForDuration(d), "code %q: %v", code, err)
	}
	out.MaxDurationMonths = months

	coupon, err := couponID(d)
	if err != nil {
		return reject(ReasonEmptyStripeCoupon, "code %q: %v", code, err)
	}
	out.StripeCouponID = coupon
	return nil
}

// couponID resolves stripe_coupon_id, distinguishing the three states the
// key can be in.
//
// Absent (nil) is the documented "not minted in this Stripe mode" and maps
// to a NULL column. Present-but-empty is neither of the console's two
// legitimate answers, so it is rejected rather than quietly treated as
// absent: an empty coupon id reaching the redemption path would be handed to
// Stripe as a coupon named "".
func couponID(d Discount) (*string, error) {
	if d.StripeCouponID == nil {
		return nil, nil
	}
	id := strings.TrimSpace(*d.StripeCouponID)
	if id == "" {
		return nil, fmt.Errorf("stripe_coupon_id is present but empty; " +
			"omit the key entirely to mean \"no coupon minted in this mode\"")
	}
	return &id, nil
}

// durationToMonths maps the console's discount duration onto
// promo_codes.max_duration_months: forever -> NULL (the column's own meaning
// for unbounded), once -> 1, repeating -> duration_in_months.
func durationToMonths(d Discount) (*int, error) {
	switch d.Duration {
	case DurationForever:
		return nil, nil
	case DurationOnce:
		one := 1
		return &one, nil
	case DurationRepeating:
		if d.DurationInMonths == nil {
			return nil, fmt.Errorf("duration is %q but duration_in_months is absent", DurationRepeating)
		}
		if *d.DurationInMonths <= 0 {
			return nil, fmt.Errorf("duration_in_months is %d, which must be positive", *d.DurationInMonths)
		}
		months := *d.DurationInMonths
		return &months, nil
	default:
		return nil, fmt.Errorf("duration %q is not one of %q, %q, %q",
			d.Duration, DurationOnce, DurationRepeating, DurationForever)
	}
}

// reasonForDuration separates "the duration word itself is wrong" from "the
// month count that goes with it is wrong", so the skip metric can tell a
// console contract change apart from one badly-filled campaign.
func reasonForDuration(d Discount) Reason {
	if d.Duration == DurationRepeating {
		return ReasonDurationInMonths
	}
	return ReasonDuration
}

// decimalPercent matches what numeric(5,2) can serialise: an unsigned
// decimal with at most two fractional digits. Anything else — a sign, an
// exponent, a third decimal place — is a contract violation, and guessing at
// its intent is how a wrong discount reaches a customer.
var decimalPercent = regexp.MustCompile(`^([0-9]{1,3})(?:\.([0-9]{1,2}))?$`)

// percentToBasisPoints converts the console's numeric(5,2) percentage into
// the basis points promo_codes.discount_value holds: 50.00 -> 5000,
// 33.33 -> 3333, 7.5 -> 750.
//
// The conversion is done on the DECIMAL DIGITS, never through a float64.
// 33.33 is not representable in binary floating point, so `int(f*100)` and
// even `math.Round(f*100)` are one library change away from 3332 or 3334 —
// a discount silently a hundredth of a percent off, on every redemption,
// with nothing to notice it by. String arithmetic has no such failure mode.
func percentToBasisPoints(n json.Number) (int, error) {
	s := strings.TrimSpace(n.String())
	if s == "" {
		return 0, fmt.Errorf("kind is %q but percent_off is absent", DiscountKindPercentOff)
	}
	m := decimalPercent.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("percent_off %q is not a plain decimal with at most "+
			"two fractional digits, which is all numeric(5,2) can hold", s)
	}
	whole, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("percent_off %q: %w", s, err)
	}
	// Pad rather than parse: ".5" is five tenths, i.e. 50 basis points, and
	// parsing "5" as the fractional part would make it 5.
	frac := 0
	if m[2] != "" {
		padded := (m[2] + "00")[:2]
		if frac, err = strconv.Atoi(padded); err != nil {
			return 0, fmt.Errorf("percent_off %q: %w", s, err)
		}
	}
	bp := whole*100 + frac
	if bp <= 0 {
		// discount_value CHECK (discount_value > 0), and a zero-percent
		// discount is a code that does nothing.
		return 0, fmt.Errorf("percent_off %q is not positive", s)
	}
	if bp > 10000 {
		return 0, fmt.Errorf("percent_off %q is more than 100%%", s)
	}
	return bp, nil
}
