package cancel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// stubPromo is a PromoApplier double. Each call records its input so tests can
// assert the save offer redeems the documented package constant.
type stubPromo struct {
	validateOut promo.ApplyOutput
	validateErr error
	applyOut    promo.ApplyOutput
	applyErr    error

	validateCalls []promo.ApplyInput
	applyCalls    []promo.ApplyInput
}

func (s *stubPromo) ValidateForSaveOffer(_ context.Context, in promo.ApplyInput) (promo.ApplyOutput, error) {
	s.validateCalls = append(s.validateCalls, in)
	return s.validateOut, s.validateErr
}

func (s *stubPromo) ApplyPromo(_ context.Context, in promo.ApplyInput) (promo.ApplyOutput, error) {
	s.applyCalls = append(s.applyCalls, in)
	return s.applyOut, s.applyErr
}

func testService(p PromoApplier) *Service {
	return (&Service{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).WithPromo(p)
}

func testSub() *subscription.StoreSubscription {
	email := "merchant@example.com"
	stripeSub := "sub_123"
	currency := "aud"
	return &subscription.StoreSubscription{
		ID:                   uuid.New(),
		TenantID:             uuid.New(),
		StoreID:              uuid.New(),
		Email:                &email,
		StripeSubscriptionID: &stripeSub,
		BillingCurrency:      &currency,
		Status:               subscription.StatusCancelScheduled,
	}
}

func testInput(sub *subscription.StoreSubscription) Input {
	return Input{
		TenantID:        sub.TenantID,
		StoreID:         sub.StoreID,
		Actor:           "user:" + uuid.NewString(),
		AcceptSaveOffer: true,
	}
}

// TestSaveOfferPromoCode_SatisfiesLengthCheck guards the promo_codes
// CHECK (char_length(code) >= 12) constraint: a shorter constant could never be
// provisioned, so the discount could never be switched on.
func TestSaveOfferPromoCode_SatisfiesLengthCheck(t *testing.T) {
	require.GreaterOrEqual(t, len(SaveOfferPromoCode), 12,
		"promo_codes CHECK (char_length(code) >= 12) — a shorter code cannot be provisioned")
}

// TestSaveOfferMessage_ClaimsDiscountOnlyWhenApplied is the core of #701: the
// merchant must never be told a discount applies when none was applied.
func TestSaveOfferMessage_ClaimsDiscountOnlyWhenApplied(t *testing.T) {
	applied := SaveOfferMessage(true)
	require.Contains(t, applied, "20%", "an applied discount must be stated")

	notApplied := SaveOfferMessage(false)
	require.NotContains(t, notApplied, "20%")
	require.NotContains(t, strings.ToLower(notApplied), "discount",
		"a discount that was not applied must not be claimed in any form")
	require.NotEmpty(t, notApplied, "the reversal itself must still be confirmed")
}

// TestSaveOfferOutput_AlwaysActive proves a promo failure can never leave the
// merchant looking cancel_scheduled: the output status is active either way.
func TestSaveOfferOutput_AlwaysActive(t *testing.T) {
	for _, applied := range []bool{true, false} {
		out := saveOfferOutput(applied)
		require.Equal(t, string(subscription.StatusActive), out.Status)
		require.Empty(t, out.CancelsAt, "an un-cancelled subscription has no cancellation date")
		require.Equal(t, SaveOfferMessage(applied), out.SaveOfferMsg)
	}
}

func TestApplySaveOfferDiscount_AppliedWhenPromoSucceeds(t *testing.T) {
	sub := testSub()
	stub := &stubPromo{}
	svc := testService(stub)

	require.True(t, svc.applySaveOfferDiscount(context.Background(), testInput(sub), sub))
	require.Len(t, stub.validateCalls, 1)
	require.Len(t, stub.applyCalls, 1)
	require.Equal(t, SaveOfferPromoCode, stub.applyCalls[0].Code)
	require.Equal(t, "merchant@example.com", stub.applyCalls[0].MerchantEmail)
	require.Equal(t, "sub_123", stub.applyCalls[0].StripeSubscriptionID)
	require.Equal(t, "aud", stub.applyCalls[0].Currency)
	require.Equal(t, sub.ID, stub.applyCalls[0].SubscriptionID)
}

func TestApplySaveOfferDiscount_NotAppliedWhenValidationRejects(t *testing.T) {
	sub := testSub()
	// The live behaviour today: no promo_codes row exists, so validation is a
	// uniform invalid-or-expired rejection.
	stub := &stubPromo{validateErr: promo.ErrInvalidOrExpired}
	svc := testService(stub)

	require.False(t, svc.applySaveOfferDiscount(context.Background(), testInput(sub), sub))
	require.Empty(t, stub.applyCalls, "a rejected code must never be applied")
}

func TestApplySaveOfferDiscount_NotAppliedWhenApplyFails(t *testing.T) {
	sub := testSub()
	stub := &stubPromo{applyErr: errors.New("stripe: coupon attach failed")}
	svc := testService(stub)

	require.False(t, svc.applySaveOfferDiscount(context.Background(), testInput(sub), sub))
	require.Len(t, stub.applyCalls, 1)
}

func TestApplySaveOfferDiscount_NilPromoDependencyIsSafe(t *testing.T) {
	sub := testSub()
	svc := testService(nil)

	require.NotPanics(t, func() {
		require.False(t, svc.applySaveOfferDiscount(context.Background(), testInput(sub), sub))
	})
}

// TestApplySaveOfferDiscount_NilSubscriptionIsSafe covers the defensive path:
// a missing subscription must not panic mid-reversal.
func TestApplySaveOfferDiscount_NilSubscriptionIsSafe(t *testing.T) {
	stub := &stubPromo{}
	svc := testService(stub)

	require.NotPanics(t, func() {
		require.False(t, svc.applySaveOfferDiscount(context.Background(), Input{}, nil))
	})
	require.Empty(t, stub.validateCalls)
}
