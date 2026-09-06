package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

type stubChecker struct {
	out   promo.ApplyOutput
	err   error
	calls []promo.ApplyInput
}

func (s *stubChecker) ValidateCode(_ context.Context, in promo.ApplyInput) (promo.ApplyOutput, error) {
	s.calls = append(s.calls, in)
	return s.out, s.err
}

func offerCron(p WinBackPromoChecker) *WinBackCron {
	c := NewWinBackCron(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	return c.WithPromo(p)
}

func expiredRow() *subscription.StoreSubscription {
	cur := "usd"
	return &subscription.StoreSubscription{
		ID:                 uuid.New(),
		TenantID:           uuid.New(),
		StoreID:            uuid.New(),
		Plan:               subscription.PlanStarter,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		BillingCurrency:    &cur,
		Status:             subscription.StatusExpired,
	}
}

func goodOutput() promo.ApplyOutput {
	return promo.ApplyOutput{PercentOffBps: 2000, MaxDurationMonths: 6, EffectiveMinor: 1520}
}

// ---------------------------------------------------------------------------
// The code exists, is named by a constant, and is provisionable.
// ---------------------------------------------------------------------------

func TestWinBackPromoCode_IsProvisionable(t *testing.T) {
	if len(WinBackPromoCode) < 4 {
		t.Fatalf("promo_codes CHECK (char_length(code) >= 4) — %q cannot be provisioned", WinBackPromoCode)
	}
	if WinBackPromoCode == "" {
		t.Fatal("empty code")
	}
}

// ---------------------------------------------------------------------------
// offerFor
// ---------------------------------------------------------------------------

func TestOfferFor_ValidCodeIsOffered(t *testing.T) {
	stub := &stubChecker{out: goodOutput()}
	offer, ok := offerCron(stub).offerFor(context.Background(), expiredRow(), "merchant@example.com")

	if !ok {
		t.Fatal("a valid code was not offered")
	}
	if offer.Code != WinBackPromoCode {
		t.Errorf("Code = %q", offer.Code)
	}
	if offer.PercentOff != "20" || offer.DurationMonths != 6 {
		t.Errorf("terms = %s%% for %d months, want 20%% for 6", offer.PercentOff, offer.DurationMonths)
	}
}

// The offer check must ask the same question the merchant's redemption will
// ask. A zero base price would make the §7.4 floor comparison meaningless and
// let the email promise a discount the apply-promo endpoint then refuses.
func TestOfferFor_AsksWithTheRealBasePrice(t *testing.T) {
	stub := &stubChecker{out: goodOutput()}
	row := expiredRow()
	offerCron(stub).offerFor(context.Background(), row, "merchant@example.com")

	if len(stub.calls) != 1 {
		t.Fatalf("%d validate calls, want 1", len(stub.calls))
	}
	got := stub.calls[0]
	want := promo.BasePriceMinorFor(row.Plan, row.SubscriptionPeriod, row.PriceTier, "usd")
	if want == 0 {
		t.Fatal("the pricing catalog must know starter monthly USD")
	}
	if got.BasePriceMinor != want {
		t.Errorf("BasePriceMinor = %d, want %d", got.BasePriceMinor, want)
	}
	if got.Code != WinBackPromoCode {
		t.Errorf("Code = %q", got.Code)
	}
	if got.MerchantEmail != "merchant@example.com" {
		t.Errorf("MerchantEmail = %q — per-email caps cannot be evaluated without it", got.MerchantEmail)
	}
}

func TestOfferFor_NoOfferWhenCodeIsRejected(t *testing.T) {
	stub := &stubChecker{
		out: promo.ApplyOutput{RejectReason: promo.RejectReasonNotFound},
		err: promo.ErrInvalidOrExpired,
	}
	if _, ok := offerCron(stub).offerFor(context.Background(), expiredRow(), "merchant@example.com"); ok {
		t.Fatal("a rejected code was offered")
	}
}

func TestOfferFor_NoOfferWhenTheLookupFails(t *testing.T) {
	stub := &stubChecker{err: errors.New("promo: validate code: lookup: connection refused")}
	if _, ok := offerCron(stub).offerFor(context.Background(), expiredRow(), "merchant@example.com"); ok {
		t.Fatal("an infrastructure failure was reported as an offer")
	}
}

func TestOfferFor_NoOfferWithoutAPromoService(t *testing.T) {
	c := NewWinBackCron(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if _, ok := c.offerFor(context.Background(), expiredRow(), "merchant@example.com"); ok {
		t.Fatal("offered a discount with no promo service attached")
	}
}

// A code that validates but cannot be described by the sentence the template
// carries must not be described by it anyway.
func TestOfferFor_NoOfferForIndescribableTerms(t *testing.T) {
	cases := []struct {
		name string
		out  promo.ApplyOutput
	}{
		{"flat amount, no percentage", promo.ApplyOutput{PercentOffBps: 0, MaxDurationMonths: 6}},
		{"no duration bound", promo.ApplyOutput{PercentOffBps: 2000, MaxDurationMonths: 0}},
		{"negative basis points", promo.ApplyOutput{PercentOffBps: -100, MaxDurationMonths: 6}},
		{"100% or more off", promo.ApplyOutput{PercentOffBps: 10000, MaxDurationMonths: 6}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubChecker{out: tc.out}
			if _, ok := offerCron(stub).offerFor(context.Background(), expiredRow(), "m@example.com"); ok {
				t.Fatal("offered")
			}
		})
	}
}

// The email states the row's number, so a console edit to the discount must
// reach the merchant rather than being contradicted by prose.
func TestOfferFor_QuotesTheRowsOwnTerms(t *testing.T) {
	stub := &stubChecker{out: promo.ApplyOutput{PercentOffBps: 1250, MaxDurationMonths: 3}}
	offer, ok := offerCron(stub).offerFor(context.Background(), expiredRow(), "m@example.com")
	if !ok {
		t.Fatal("not offered")
	}
	if offer.PercentOff != "12.5" || offer.DurationMonths != 3 {
		t.Fatalf("terms = %s%% for %d months, want 12.5%% for 3", offer.PercentOff, offer.DurationMonths)
	}
}

func TestFormatPercentBps(t *testing.T) {
	cases := []struct {
		bps  int
		want string
		ok   bool
	}{
		{2000, "20", true},
		{1250, "12.5", true},
		{1, "0.01", true},
		{9999, "99.99", true},
		{0, "", false},
		{-1, "", false},
		{10000, "", false},
		{20000, "", false},
	}
	for _, tc := range cases {
		got, ok := formatPercentBps(tc.bps)
		if got != tc.want || ok != tc.ok {
			t.Errorf("formatPercentBps(%d) = %q,%v; want %q,%v", tc.bps, got, ok, tc.want, tc.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// Template selection
// ---------------------------------------------------------------------------

func TestWinBackTemplate_FollowsTheOffer(t *testing.T) {
	if got := WinBackTemplate(true); got != email.TemplateWinBack {
		t.Errorf("offered → %q", got)
	}
	if got := WinBackTemplate(false); got != email.TemplateWinBackNoOffer {
		t.Errorf("not offered → %q", got)
	}
}
