package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
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
