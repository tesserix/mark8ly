package consolepromo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// now is a fixed instant. MapCode is pure in (in, now), so every assertion
// below is exact rather than approximate.
var now = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// mustMap fails the test when a definition that should map does not.
func mustMap(t *testing.T, in Code) promo.PromoCode {
	t.Helper()
	got, err := MapCode(in, now)
	if err != nil {
		t.Fatalf("MapCode(%q): unexpected error: %v", in.Code, err)
	}
	return got
}

// TestMapCode_PercentOffToBasisPoints is the test this package exists for.
//
// promo_codes.discount_value is basis points and the console publishes
// numeric(5,2), so every value here is a x100 conversion that MUST be exact.
// 33.33 is the case that matters: it has no exact binary representation, so
// a float64 round-trip is one optimisation away from 3332 or 3334 — a
// discount silently wrong on every redemption, with nothing to notice it by.
func TestMapCode_PercentOffToBasisPoints(t *testing.T) {
	cases := []struct {
		percent string
		wantBP  int
	}{
		{"50.00", 5000},
		{"33.33", 3333}, // the float-drift case
		{"66.67", 6667},
		{"0.01", 1}, // the smallest expressible discount
		{"100.00", 10000},
		{"7.5", 750},  // one decimal place: five tenths is 50bp, not 5
		{"7.50", 750}, // and it means the same written out
		{"10", 1000},  // no decimal point at all
		{"99.99", 9999},
		{"1.23", 123},
	}
	for _, tc := range cases {
		t.Run(tc.percent, func(t *testing.T) {
			got := mustMap(t, Code{
				Code: "LAUNCH50",
				Discount: &Discount{
					Kind:           DiscountKindPercentOff,
					PercentOff:     json.Number(tc.percent),
					Duration:       DurationForever,
					StripeCouponID: ptr("cpn_live_1"),
				},
			})
			if got.DiscountType == nil || *got.DiscountType != promo.DiscountTypePercentage {
				t.Fatalf("discount_type = %v, want percentage", got.DiscountType)
			}
			if got.DiscountValue == nil || *got.DiscountValue != tc.wantBP {
				t.Fatalf("discount_value = %v, want %d basis points", got.DiscountValue, tc.wantBP)
			}
		})
	}
}

// TestMapCode_PercentOffNeverGoesThroughAFloat pins the property rather than
// the values: for every hundredth of a percent, the basis points must be the
// digits with the point removed. A float64 implementation fails somewhere in
// this range; string arithmetic cannot.
func TestMapCode_PercentOffNeverGoesThroughAFloat(t *testing.T) {
	for whole := 0; whole <= 100; whole++ {
		for frac := 0; frac < 100; frac++ {
			want := whole*100 + frac
			if want <= 0 || want > 10000 {
				continue
			}
			s := json.Number(itoa(whole) + "." + pad2(frac))
			got, err := percentToBasisPoints(s)
			if err != nil {
				t.Fatalf("percentToBasisPoints(%q): %v", s, err)
			}
			if got != want {
				t.Fatalf("percentToBasisPoints(%q) = %d, want %d", s, got, want)
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad2(n int) string {
	s := itoa(n)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func TestMapCode_AmountOff(t *testing.T) {
	got := mustMap(t, Code{
		Code: "B2B1",
		Discount: &Discount{
			Kind:           DiscountKindAmountOff,
			AmountOffMinor: ptr(int64(2500)),
			Currency:       "usd",
			Duration:       DurationOnce,
			StripeCouponID: ptr("cpn_amt"),
		},
	})
	if got.DiscountType == nil || *got.DiscountType != promo.DiscountTypeAmount {
		t.Fatalf("discount_type = %v, want amount", got.DiscountType)
	}
	// Minor units pass through unchanged; unlike percent there is no scaling.
	if got.DiscountValue == nil || *got.DiscountValue != 2500 {
		t.Fatalf("discount_value = %v, want 2500 minor units", got.DiscountValue)
	}
}

// TestMapCode_TrialExtensionOnly is the shape migration 000131 exists for: a
// code with no discount, no coupon, and a trial extension.
func TestMapCode_TrialExtensionOnly(t *testing.T) {
	got := mustMap(t, Code{
		Code:               "EXTRA14DAYS",
		TrialExtensionDays: ptr(14),
	})
	if got.TrialExtensionDays == nil || *got.TrialExtensionDays != 14 {
		t.Fatalf("trial_extension_days = %v, want 14", got.TrialExtensionDays)
	}
	if got.DiscountType != nil || got.DiscountValue != nil {
		t.Fatalf("expected no discount, got type=%v value=%v", got.DiscountType, got.DiscountValue)
	}
	if got.StripeCouponID != nil {
		t.Fatalf("stripe_coupon_id = %v, want nil for a trial-extension-only code", *got.StripeCouponID)
	}
	if got.MaxDurationMonths != nil {
		t.Fatalf("max_duration_months = %v, want nil when there is no discount", *got.MaxDurationMonths)
	}
}

// TestMapCode_AbsentStripeCouponIDBecomesNull covers the console's "not
// minted in this Stripe mode" signal, which is an ABSENT KEY. It must reach
// the database as NULL, never as the empty string.
func TestMapCode_AbsentStripeCouponIDBecomesNull(t *testing.T) {
	// Decoded from JSON rather than constructed, so the test proves the
	// absent key really does arrive as nil through the wire type.
	var in Code
	raw := `{"code":"NOCOUPON50","discount":{"kind":"percent_off","percent_off":50.00,"duration":"forever"}}`
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if in.Discount.StripeCouponID != nil {
		t.Fatalf("an absent stripe_coupon_id key decoded to %q, want nil", *in.Discount.StripeCouponID)
	}

	got := mustMap(t, in)
	if got.StripeCouponID != nil {
		t.Fatalf("stripe_coupon_id = %q, want nil (a NULL column)", *got.StripeCouponID)
	}
	if got.DiscountValue == nil || *got.DiscountValue != 5000 {
		t.Fatalf("discount_value = %v, want 5000", got.DiscountValue)
	}
}

// TestMapCode_PresentStripeCouponIDIsKept is the other half: a minted coupon
// must survive, so redemption can attach it.
func TestMapCode_PresentStripeCouponIDIsKept(t *testing.T) {
	got := mustMap(t, Code{
		Code: "LAUNCH50",
		Discount: &Discount{
			Kind:           DiscountKindPercentOff,
			PercentOff:     json.Number("50.00"),
			Duration:       DurationForever,
			StripeCouponID: ptr("  cpn_abc123  "),
		},
	})
	if got.StripeCouponID == nil || *got.StripeCouponID != "cpn_abc123" {
		t.Fatalf("stripe_coupon_id = %v, want the trimmed \"cpn_abc123\"", got.StripeCouponID)
	}
}

func TestMapCode_Durations(t *testing.T) {
	cases := []struct {
		name     string
		duration string
		months   *int
		want     *int
	}{
		// forever -> NULL, which is what max_duration_months already means
		// for an unbounded discount (000060).
		{"forever", DurationForever, nil, nil},
		{"once", DurationOnce, nil, ptr(1)},
		{"repeating", DurationRepeating, ptr(3), ptr(3)},
		{"repeating ignores a longer run", DurationRepeating, ptr(12), ptr(12)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustMap(t, Code{
				Code: "DURATION1",
				Discount: &Discount{
					Kind:             DiscountKindPercentOff,
					PercentOff:       json.Number("10.00"),
					Duration:         tc.duration,
					DurationInMonths: tc.months,
				},
			})
			switch {
			case tc.want == nil && got.MaxDurationMonths != nil:
				t.Fatalf("max_duration_months = %d, want NULL", *got.MaxDurationMonths)
			case tc.want != nil && got.MaxDurationMonths == nil:
				t.Fatalf("max_duration_months = NULL, want %d", *tc.want)
			case tc.want != nil && *got.MaxDurationMonths != *tc.want:
				t.Fatalf("max_duration_months = %d, want %d", *got.MaxDurationMonths, *tc.want)
			}
		})
	}
}

func TestMapCode_PassesThroughWindowAndLimits(t *testing.T) {
	from := now.Add(-24 * time.Hour)
	until := now.Add(24 * time.Hour)
	got := mustMap(t, Code{
		Code:               "WINDOWED1",
		TrialExtensionDays: ptr(7),
		ValidFrom:          &from,
		ValidUntil:         &until,
		MaxRedemptions:     ptr(100),
	})
	if !got.ValidFrom.Equal(from) {
		t.Fatalf("valid_from = %v, want %v", got.ValidFrom, from)
	}
	if got.ValidUntil == nil || !got.ValidUntil.Equal(until) {
		t.Fatalf("valid_until = %v, want %v", got.ValidUntil, until)
	}
	if got.MaxRedemptions == nil || *got.MaxRedemptions != 100 {
		t.Fatalf("max_redemptions = %v, want 100", got.MaxRedemptions)
	}
	if got.CreatedBy != CreatedBy {
		t.Fatalf("created_by = %q, want %q", got.CreatedBy, CreatedBy)
	}
	// max_per_email must be written explicitly. A zero would mean "nobody may
	// ever redeem this" — a dead code that looks alive.
	if got.MaxPerEmail != DefaultMaxPerEmail {
		t.Fatalf("max_per_email = %d, want %d", got.MaxPerEmail, DefaultMaxPerEmail)
	}
}

// TestMapCode_DefaultsValidFromToNow covers the console omitting the start
// bound; the column is NOT NULL DEFAULT now(), so the mapper supplies it.
func TestMapCode_DefaultsValidFromToNow(t *testing.T) {
	got := mustMap(t, Code{Code: "NOWINDOW1", TrialExtensionDays: ptr(7)})
	if !got.ValidFrom.Equal(now) {
		t.Fatalf("valid_from = %v, want the ingest time %v", got.ValidFrom, now)
	}
	if got.ValidUntil != nil {
		t.Fatalf("valid_until = %v, want NULL", *got.ValidUntil)
	}
}

// TestMapCode_UnlimitedRedemptionsStaysNil pins that an absent
// max_redemptions is unlimited, not zero. A zero would be a code nobody can
// redeem.
func TestMapCode_UnlimitedRedemptionsStaysNil(t *testing.T) {
	got := mustMap(t, Code{Code: "UNLIMITED1", TrialExtensionDays: ptr(30)})
	if got.MaxRedemptions != nil {
		t.Fatalf("max_redemptions = %d, want NULL (unlimited)", *got.MaxRedemptions)
	}
}

// TestMapCode_Rejections walks every rejection path. Each asserts the REASON
// as well as the failure, because the reason is a Prometheus label and the
// whole diagnostic value of rejecting here rather than at the database.
func TestMapCode_Rejections(t *testing.T) {
	cases := []struct {
		name string
		in   Code
		want Reason
	}{
		{
			"empty code",
			Code{Code: "", TrialExtensionDays: ptr(7)},
			ReasonCodeLength,
		},
		{
			"code below the schema floor of 4",
			Code{Code: "AB", TrialExtensionDays: ptr(7)},
			ReasonCodeLength,
		},
		{
			"code beyond varchar(64)",
			Code{Code: longCode(65), TrialExtensionDays: ptr(7)},
			ReasonCodeLength,
		},
		{
			"no benefit at all",
			Code{Code: "USELESS1"},
			ReasonNoBenefit,
		},
		{
			"zero trial extension",
			Code{Code: "ZERODAYS", TrialExtensionDays: ptr(0)},
			ReasonTrialExtension,
		},
		{
			"negative trial extension",
			Code{Code: "NEGDAYS1", TrialExtensionDays: ptr(-3)},
			ReasonTrialExtension,
		},
		{
			"unknown discount kind",
			Code{Code: "WEIRDKIND", Discount: &Discount{
				Kind: "buy_one_get_one", Duration: DurationOnce,
			}},
			ReasonDiscountKind,
		},
		{
			"percent kind with no percent_off",
			Code{Code: "NOPERCENT", Discount: &Discount{
				Kind: DiscountKindPercentOff, Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"zero percent",
			Code{Code: "ZEROPCT1", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("0.00"), Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"more than 100 percent",
			Code{Code: "OVER100P", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("100.01"), Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"negative percent",
			Code{Code: "NEGPCT12", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("-10.00"), Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"three decimal places is beyond numeric(5,2)",
			Code{Code: "TOOPRECISE", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("33.333"), Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"exponent notation is not a numeric(5,2) literal",
			Code{Code: "EXPONENT1", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("5e1"), Duration: DurationOnce,
			}},
			ReasonPercentOff,
		},
		{
			"amount kind with no amount",
			Code{Code: "NOAMOUNT1", Discount: &Discount{
				Kind: DiscountKindAmountOff, Duration: DurationOnce,
			}},
			ReasonAmountOff,
		},
		{
			"zero amount",
			Code{Code: "ZEROAMT12", Discount: &Discount{
				Kind: DiscountKindAmountOff, AmountOffMinor: ptr(int64(0)), Duration: DurationOnce,
			}},
			ReasonAmountOff,
		},
		{
			"negative amount",
			Code{Code: "NEGAMT123", Discount: &Discount{
				Kind: DiscountKindAmountOff, AmountOffMinor: ptr(int64(-500)), Duration: DurationOnce,
			}},
			ReasonAmountOff,
		},
		{
			"amount beyond the integer column",
			Code{Code: "HUGEAMT12", Discount: &Discount{
				Kind: DiscountKindAmountOff, AmountOffMinor: ptr(int64(1) << 40), Duration: DurationOnce,
			}},
			ReasonAmountOff,
		},
		{
			"unknown duration",
			Code{Code: "BADDUR123", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("10.00"), Duration: "annually",
			}},
			ReasonDuration,
		},
		{
			"repeating with no month count",
			Code{Code: "REPEATNO1", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("10.00"), Duration: DurationRepeating,
			}},
			ReasonDurationInMonths,
		},
		{
			"repeating for zero months",
			Code{Code: "REPEAT0MO", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("10.00"),
				Duration: DurationRepeating, DurationInMonths: ptr(0),
			}},
			ReasonDurationInMonths,
		},
		{
			"stripe_coupon_id present but empty",
			Code{Code: "EMPTYCPN1", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("10.00"),
				Duration: DurationOnce, StripeCouponID: ptr(""),
			}},
			ReasonEmptyStripeCoupon,
		},
		{
			"stripe_coupon_id present but only whitespace",
			Code{Code: "BLANKCPN1", Discount: &Discount{
				Kind: DiscountKindPercentOff, PercentOff: json.Number("10.00"),
				Duration: DurationOnce, StripeCouponID: ptr("   "),
			}},
			ReasonEmptyStripeCoupon,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MapCode(tc.in, now)
			if err == nil {
				t.Fatal("expected a rejection, got none")
			}
			if got := ReasonOf(err); got != tc.want {
				t.Fatalf("reason = %q, want %q (error: %v)", got, tc.want, err)
			}
		})
	}
}

// TestMapCode_TrialExtensionAndDiscountTogether — the console can publish
// both, and the row must carry both.
func TestMapCode_TrialExtensionAndDiscountTogether(t *testing.T) {
	got := mustMap(t, Code{
		Code:               "BOTHBENEFITS",
		TrialExtensionDays: ptr(30),
		Discount: &Discount{
			Kind:             DiscountKindPercentOff,
			PercentOff:       json.Number("25.00"),
			Duration:         DurationRepeating,
			DurationInMonths: ptr(6),
			StripeCouponID:   ptr("cpn_both"),
		},
	})
	if got.TrialExtensionDays == nil || *got.TrialExtensionDays != 30 {
		t.Fatalf("trial_extension_days = %v, want 30", got.TrialExtensionDays)
	}
	if got.DiscountValue == nil || *got.DiscountValue != 2500 {
		t.Fatalf("discount_value = %v, want 2500", got.DiscountValue)
	}
	if got.MaxDurationMonths == nil || *got.MaxDurationMonths != 6 {
		t.Fatalf("max_duration_months = %v, want 6", got.MaxDurationMonths)
	}
}

// TestReasonOf_NonRejection keeps the metric label total: an error that is
// not a RejectedError must still produce a usable label.
func TestReasonOf_NonRejection(t *testing.T) {
	if got := ReasonOf(nil); got != "" {
		t.Fatalf("ReasonOf(nil) = %q, want the empty reason", got)
	}
	if got := ReasonOf(errString("something else")); got != ReasonUnknown {
		t.Fatalf("ReasonOf(other) = %q, want %q", got, ReasonUnknown)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func longCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'A'
	}
	return string(b)
}
