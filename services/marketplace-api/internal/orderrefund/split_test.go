package orderrefund

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// state builds a RefundLedgerState from the four persisted sums, in the
// order they read most naturally: what the gateway took, what it has given
// back, what the gift cards paid, what they have been given back.
func state(charged, returned, gcApplied, gcReturned, inFlight string) RefundLedgerState {
	return RefundLedgerState{
		GatewayCharged:   d(charged),
		GatewayReturned:  d(returned),
		GiftCardApplied:  d(gcApplied),
		GiftCardReturned: d(gcReturned),
		InFlight:         d(inFlight),
	}
}

// The worked example from the brief: a $100 order paid with a $40 gift card
// and $60 on the card. Real money must come back before store credit does.
func TestSplitRefund_GatewayFirst(t *testing.T) {
	s := state("60.00", "0", "40.00", "0", "0")

	cases := []struct {
		amount, wantGateway, wantGiftCard string
	}{
		{"50.00", "50.00", "0"},      // wholly inside the gateway portion
		{"60.00", "60.00", "0"},      // exactly exhausts the gateway
		{"80.00", "60.00", "20.00"},  // crosses the boundary
		{"100.00", "60.00", "40.00"}, // the whole order
		{"0.01", "0.01", "0"},
	}
	for _, c := range cases {
		got, err := SplitRefund(s, d(c.amount))
		if err != nil {
			t.Fatalf("amount=%s: %v", c.amount, err)
		}
		if !got.Gateway.Equal(d(c.wantGateway)) || !got.GiftCard.Equal(d(c.wantGiftCard)) {
			t.Fatalf("amount=%s => gateway=%s giftcard=%s, want gateway=%s giftcard=%s",
				c.amount, got.Gateway, got.GiftCard, c.wantGateway, c.wantGiftCard)
		}
		if !got.Total().Equal(d(c.amount)) {
			t.Fatalf("amount=%s: split totals %s — a split must never lose money", c.amount, got.Total())
		}
	}
}

// Multiple partial refunds on one order: each one is derived from what has
// ALREADY been returned to each side, never from the latest refund alone.
// $60 gateway + $40 gift card, refunded 30 → 30 → 30 → 10.
func TestSplitRefund_SequentialPartials(t *testing.T) {
	gwReturned, gcReturned := d("0"), d("0")

	steps := []struct{ amount, wantGateway, wantGiftCard string }{
		{"30.00", "30.00", "0"}, // gateway: 30/60
		{"30.00", "30.00", "0"}, // gateway: 60/60 — exactly exhausted
		{"30.00", "0", "30.00"}, // gateway is gone; all store credit
		{"10.00", "0", "10.00"}, // finishes the card
	}
	for i, st := range steps {
		s := RefundLedgerState{
			GatewayCharged: d("60.00"), GatewayReturned: gwReturned,
			GiftCardApplied: d("40.00"), GiftCardReturned: gcReturned,
			InFlight: decimal.Zero,
		}
		got, err := SplitRefund(s, d(st.amount))
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if !got.Gateway.Equal(d(st.wantGateway)) || !got.GiftCard.Equal(d(st.wantGiftCard)) {
			t.Fatalf("step %d (amount=%s) => gateway=%s giftcard=%s, want gateway=%s giftcard=%s",
				i, st.amount, got.Gateway, got.GiftCard, st.wantGateway, st.wantGiftCard)
		}
		gwReturned = gwReturned.Add(got.Gateway)
		gcReturned = gcReturned.Add(got.GiftCard)
	}

	// A fifth refund of any size has nothing left to return.
	s := RefundLedgerState{
		GatewayCharged: d("60.00"), GatewayReturned: gwReturned,
		GiftCardApplied: d("40.00"), GiftCardReturned: gcReturned,
		InFlight: decimal.Zero,
	}
	if _, err := SplitRefund(s, d("0.01")); !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("over-refund after the order is fully returned => %v, want ErrRefundExceedsTotal", err)
	}
}

// The gift-card portion must be REACHABLE. Before this, the coordinator
// capped every refund at grand_total — which excludes the gift card — so an
// order part-paid with store credit could never be fully refunded at all.
func TestSplitRefund_RemainingIncludesGiftCardPortion(t *testing.T) {
	s := state("60.00", "0", "40.00", "0", "0")
	if !s.Remaining().Equal(d("100.00")) {
		t.Fatalf("Remaining() = %s, want 100.00 — the gift-card portion is refundable money", s.Remaining())
	}

	// Over the combined envelope is still refused.
	if _, err := SplitRefund(s, d("100.01")); !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("amount over the combined envelope => %v, want ErrRefundExceedsTotal", err)
	}
}

// An in-flight refund reserved by a concurrent caller has already claimed
// capacity, gateway-first. It must not be handed out twice.
func TestSplitRefund_InFlightConsumesGatewayFirst(t *testing.T) {
	// $50 already reserved but not yet settled: $50 of the gateway is spoken for.
	s := state("60.00", "0", "40.00", "0", "50.00")
	got, err := SplitRefund(s, d("10.00"))
	if err != nil {
		t.Fatalf("SplitRefund: %v", err)
	}
	if !got.Gateway.Equal(d("10.00")) || !got.GiftCard.Equal(decimal.Zero) {
		t.Fatalf("gateway=%s giftcard=%s, want gateway=10.00 giftcard=0", got.Gateway, got.GiftCard)
	}

	// $70 in flight spills $10 past the gateway onto the card, so only $30
	// of card capacity is left.
	s = state("60.00", "0", "40.00", "0", "70.00")
	got, err = SplitRefund(s, d("30.00"))
	if err != nil {
		t.Fatalf("SplitRefund: %v", err)
	}
	if !got.Gateway.Equal(decimal.Zero) || !got.GiftCard.Equal(d("30.00")) {
		t.Fatalf("gateway=%s giftcard=%s, want gateway=0 giftcard=30.00", got.Gateway, got.GiftCard)
	}
	if _, err := SplitRefund(s, d("30.01")); !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("over the in-flight-adjusted envelope => %v, want ErrRefundExceedsTotal", err)
	}
}

// An order with no gift card behaves exactly as before: everything to the
// gateway, capped at what was charged.
func TestSplitRefund_NoGiftCardIsUnchanged(t *testing.T) {
	s := state("120.00", "50.00", "0", "0", "0")
	got, err := SplitRefund(s, d("70.00"))
	if err != nil {
		t.Fatalf("SplitRefund: %v", err)
	}
	if !got.Gateway.Equal(d("70.00")) || !got.GiftCard.IsZero() {
		t.Fatalf("gateway=%s giftcard=%s, want gateway=70.00 giftcard=0", got.Gateway, got.GiftCard)
	}
	if _, err := SplitRefund(s, d("70.01")); !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("over grand_total => %v, want ErrRefundExceedsTotal", err)
	}
}

func TestSplitRefund_RejectsNonPositive(t *testing.T) {
	s := state("60.00", "0", "40.00", "0", "0")
	for _, amt := range []string{"0", "-1.00"} {
		if _, err := SplitRefund(s, d(amt)); !errors.Is(err, apperrors.ErrValidationFailed) {
			t.Fatalf("amount=%s => %v, want ErrValidationFailed", amt, err)
		}
	}
}

// GiftCardCreditTarget is what makes the credit idempotent: it is derived
// from the totals on both ledgers, never from "the refund we just did".
// Replaying it can only ever return 0.
func TestGiftCardCreditTarget(t *testing.T) {
	cases := []struct {
		name                                            string
		returnedTotal, gatewayCharged, applied, already string
		want                                            string
	}{
		{"nothing past the gateway yet", "50.00", "60.00", "40.00", "0", "0"},
		{"gateway exactly exhausted", "60.00", "60.00", "40.00", "0", "0"},
		{"crossed the boundary", "80.00", "60.00", "40.00", "0", "20.00"},
		{"replay of the same refund credits nothing", "80.00", "60.00", "40.00", "20.00", "0"},
		{"whole order", "100.00", "60.00", "40.00", "0", "40.00"},
		{"never exceeds what the card paid", "150.00", "60.00", "40.00", "0", "40.00"},
		{"already over-credited stays put", "80.00", "60.00", "40.00", "40.00", "0"},
		{"order had no gift card", "60.00", "60.00", "0", "0", "0"},
	}
	for _, c := range cases {
		got := GiftCardCreditTarget(d(c.returnedTotal), d(c.gatewayCharged), d(c.applied), d(c.already))
		if !got.Equal(d(c.want)) {
			t.Fatalf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
