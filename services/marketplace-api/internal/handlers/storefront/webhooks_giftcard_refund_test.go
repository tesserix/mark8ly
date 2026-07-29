package storefront

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/payment"
)

func detail(refunded, original string, full bool) *payment.RefundDetail {
	return &payment.RefundDetail{
		RefundedTotal: decimal.RequireFromString(refunded),
		OriginalTotal: decimal.RequireFromString(original),
		Full:          full,
	}
}

// TestGiftCardRefundFrom is the handler-side guard for the customer-harm
// bug. It lives in the default suite (CI runs `go test ./...` with no build
// tag) because the rule it protects — an event that cannot prove the refund
// was FULL must never be acted on as if it were — is what stands between a
// $10 refund and a destroyed $100 gift card.
func TestGiftCardRefundFrom(t *testing.T) {
	cases := []struct {
		name         string
		evt          payment.WebhookEvent
		wantRef      string
		wantRefunded string
		wantFull     bool
		wantReason   string
	}{
		{
			name: "partial refund is passed through as partial",
			evt: payment.WebhookEvent{
				ProviderPaymentID: "pi_456",
				Refund:            detail("10.00", "100.00", false),
			},
			wantRef: "pi_456", wantRefunded: "10.00", wantFull: false,
		},
		{
			name: "full refund is passed through as full",
			evt: payment.WebhookEvent{
				ProviderPaymentID: "pi_456",
				Refund:            detail("100.00", "100.00", true),
			},
			wantRef: "pi_456", wantRefunded: "100.00", wantFull: true,
		},
		{
			// The hosted-checkout session id is the preferred correlation
			// key when the event carries one.
			name: "session id wins over the payment intent id",
			evt: payment.WebhookEvent{
				SessionID:         "cs_1",
				ProviderPaymentID: "pi_456",
				Refund:            detail("10.00", "100.00", false),
			},
			wantRef: "cs_1", wantRefunded: "10.00", wantFull: false,
		},
		{
			// Razorpay and PayPal land here. Nil means UNKNOWN, and unknown
			// must never be treated as full.
			name: "a provider that reports no breakdown is refused, not treated as full",
			evt: payment.WebhookEvent{
				ProviderPaymentID: "pay_xyz",
				Refund:            nil,
			},
			wantReason: "provider reported no refund amount breakdown",
		},
		{
			name: "a refund event with no correlation id is refused",
			evt: payment.WebhookEvent{
				Refund: detail("10.00", "100.00", false),
			},
			wantReason: "missing correlation id",
		},
		{
			name: "a zero refunded total is refused",
			evt: payment.WebhookEvent{
				ProviderPaymentID: "pi_456",
				Refund:            detail("0", "100.00", false),
			},
			wantReason: "refunded amount is not positive",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref, in, reason := giftCardRefundFrom(&c.evt)

			if c.wantReason != "" {
				if reason != c.wantReason {
					t.Fatalf("reason = %q, want %q", reason, c.wantReason)
				}
				if ref != "" {
					t.Fatalf("ref = %q on a refused event, want empty — the caller must do nothing", ref)
				}
				if in.Full {
					t.Fatalf("Full = true on a refused event — an unusable event must never look like a full refund")
				}
				return
			}

			if reason != "" {
				t.Fatalf("reason = %q, want actionable", reason)
			}
			if ref != c.wantRef {
				t.Fatalf("ref = %q, want %q", ref, c.wantRef)
			}
			if want := decimal.RequireFromString(c.wantRefunded); !in.RefundedTotal.Equal(want) {
				t.Fatalf("RefundedTotal = %s, want %s", in.RefundedTotal, want)
			}
			if in.Full != c.wantFull {
				t.Fatalf("Full = %v, want %v", in.Full, c.wantFull)
			}
		})
	}
}
