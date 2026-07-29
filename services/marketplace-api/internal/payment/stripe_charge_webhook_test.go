package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// signStripe builds a valid Stripe-Signature header for payload.
func signStripe(payload []byte, secret string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// TestStripeVerifyWebhook_ChargeRefundedCarriesPaymentIntent guards the
// correlation id for the gift-card purchase-refund path.
//
// `charge.refunded` normalises to refund.succeeded, but its data object is a
// Charge (`ch_…`) — neither a `cs_…` session nor a `pi_…` intent. Without
// this mapping the event arrives with an EMPTY ProviderPaymentID and an
// empty SessionID, so the webhook has nothing to look the gift card up by
// and the void silently never happens.
func TestStripeVerifyWebhook_ChargeRefundedCarriesPaymentIntent(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{
		"id": "evt_1",
		"type": "charge.refunded",
		"data": {"object": {
			"id": "ch_123",
			"payment_intent": "pi_456",
			"amount": 5000,
			"currency": "aud",
			"metadata": {"gift_card_id": "11111111-1111-1111-1111-111111111111"}
		}}
	}`)

	g := NewStripeGateway("sk_test", secret, "test")
	evt, err := g.VerifyWebhook(context.Background(), payload, signStripe(payload, secret))
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}

	if evt.EventType != "refund.succeeded" {
		t.Fatalf("EventType = %q, want refund.succeeded", evt.EventType)
	}
	if evt.ProviderPaymentID != "pi_456" {
		t.Fatalf("ProviderPaymentID = %q, want pi_456 — a charge event must expose its PaymentIntent so the gift card can be correlated", evt.ProviderPaymentID)
	}
	if evt.Metadata["gift_card_id"] == "" {
		t.Fatalf("gift_card_id metadata lost; gift card routing would never fire")
	}
}

// A charge without a PaymentIntent (legacy direct charge) must not invent
// one — an empty id is a correlation miss the handler logs, not a lookup
// against the charge id that would silently match nothing.
func TestStripeVerifyWebhook_ChargeWithoutIntentLeavesIDEmpty(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_2","type":"charge.refunded","data":{"object":{"id":"ch_999","currency":"aud"}}}`)

	g := NewStripeGateway("sk_test", secret, "test")
	evt, err := g.VerifyWebhook(context.Background(), payload, signStripe(payload, secret))
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if evt.ProviderPaymentID != "" {
		t.Fatalf("ProviderPaymentID = %q, want empty", evt.ProviderPaymentID)
	}
}

// chargeRefundedPayload builds a `charge.refunded` body with the two fields
// that decide whether a gift card is voided or merely debited.
func chargeRefundedPayload(id, currency string, amount, amountRefunded int64) []byte {
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"type": "charge.refunded",
		"data": {"object": {
			"id": "ch_123",
			"payment_intent": "pi_456",
			"amount": %d,
			"amount_refunded": %d,
			"currency": %q,
			"metadata": {"gift_card_id": "11111111-1111-1111-1111-111111111111"}
		}}
	}`, id, amount, amountRefunded, currency))
}

// TestStripeVerifyWebhook_RefundDetail is the guard for the customer-harm
// bug: Stripe sends `charge.refunded` for PARTIAL refunds too, so an event
// that carries no amount breakdown leaves the handler unable to tell a $10
// clawback from destroying a $100 card. The pair (RefundedTotal, Full) is
// what makes that decidable.
func TestStripeVerifyWebhook_RefundDetail(t *testing.T) {
	secret := "whsec_test"

	cases := []struct {
		name         string
		payload      []byte
		wantNil      bool
		wantRefunded string
		wantOriginal string
		wantFull     bool
	}{
		{
			// $10 refunded off a $50 charge. Full MUST be false — this is
			// the case that must not void the card.
			name:         "partial refund is not full",
			payload:      chargeRefundedPayload("evt_p", "aud", 5000, 1000),
			wantRefunded: "10.00", wantOriginal: "50.00", wantFull: false,
		},
		{
			name:         "whole charge refunded is full",
			payload:      chargeRefundedPayload("evt_f", "aud", 5000, 5000),
			wantRefunded: "50.00", wantOriginal: "50.00", wantFull: true,
		},
		{
			// Cumulative: Stripe reports the running total, so the second
			// $10 refund arrives as amount_refunded = 2000.
			name:         "second sequential partial reports the cumulative total",
			payload:      chargeRefundedPayload("evt_p2", "aud", 5000, 2000),
			wantRefunded: "20.00", wantOriginal: "50.00", wantFull: false,
		},
		{
			// Zero-decimal currency: ¥1000 refunded off ¥5000 is 1000, not
			// 10.00. Dividing by 100 here would be a 100x money bug.
			name:         "jpy is zero-decimal",
			payload:      chargeRefundedPayload("evt_j", "jpy", 5000, 1000),
			wantRefunded: "1000", wantOriginal: "5000", wantFull: false,
		},
		{
			// Three-decimal currency: 1000 fils is 1.000 KWD.
			name:         "kwd is three-decimal",
			payload:      chargeRefundedPayload("evt_k", "kwd", 5000, 1000),
			wantRefunded: "1.000", wantOriginal: "5.000", wantFull: false,
		},
		{
			// Over-refund (Stripe permits refunding more than captured in
			// rare dispute flows) still reads as full, never as negative.
			name:         "over-refund is full",
			payload:      chargeRefundedPayload("evt_o", "aud", 5000, 6000),
			wantRefunded: "60.00", wantOriginal: "50.00", wantFull: true,
		},
		{
			// No breakdown at all — the handler must be able to tell
			// "unknown" from "full", so this is nil, NOT a full refund.
			name:    "charge with no amount_refunded reports nothing",
			payload: chargeRefundedPayload("evt_n", "aud", 5000, 0),
			wantNil: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewStripeGateway("sk_test", secret, "test")
			evt, err := g.VerifyWebhook(context.Background(), c.payload, signStripe(c.payload, secret))
			if err != nil {
				t.Fatalf("VerifyWebhook: %v", err)
			}

			if c.wantNil {
				if evt.Refund != nil {
					t.Fatalf("Refund = %+v, want nil — an event with no amount breakdown must not look like a full refund", evt.Refund)
				}
				return
			}

			if evt.Refund == nil {
				t.Fatalf("Refund = nil, want a breakdown — without it the handler cannot tell a partial refund from a full one")
			}
			if want := decimal.RequireFromString(c.wantRefunded); !evt.Refund.RefundedTotal.Equal(want) {
				t.Fatalf("RefundedTotal = %s, want %s (minor units must convert by the currency exponent)",
					evt.Refund.RefundedTotal, want)
			}
			if want := decimal.RequireFromString(c.wantOriginal); !evt.Refund.OriginalTotal.Equal(want) {
				t.Fatalf("OriginalTotal = %s, want %s", evt.Refund.OriginalTotal, want)
			}
			if evt.Refund.Full != c.wantFull {
				t.Fatalf("Full = %v, want %v", evt.Refund.Full, c.wantFull)
			}
		})
	}
}

// A non-refund event must not carry a refund breakdown at all — otherwise a
// succeeded payment could be read as a refund of its own value.
func TestStripeVerifyWebhook_NonRefundEventHasNoRefundDetail(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_s","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","amount":5000,"amount_refunded":5000,"currency":"aud"}}}`)

	g := NewStripeGateway("sk_test", secret, "test")
	evt, err := g.VerifyWebhook(context.Background(), payload, signStripe(payload, secret))
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if evt.Refund != nil {
		t.Fatalf("Refund = %+v on a %s event, want nil", evt.Refund, evt.EventType)
	}
}
