package orderrefund

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/order"
)

// TestDeriveStatus proves the partial-vs-full status derivation: a refund
// (existing refunded_amount + this amount) that meets or exceeds grand_total
// flips the order to refunded; anything less stays partially_refunded.
func TestDeriveStatus(t *testing.T) {
	gt := decimal.RequireFromString("120.00")
	cases := []struct {
		refunded, amount string
		want             order.PaymentStatus
	}{
		{"0", "50.00", order.PaymentStatusPartiallyRefunded},
		{"0", "120.00", order.PaymentStatusRefunded},
		{"60.00", "60.00", order.PaymentStatusRefunded},
		{"60.00", "30.00", order.PaymentStatusPartiallyRefunded},
	}
	for _, c := range cases {
		got := DeriveStatus(decimal.RequireFromString(c.refunded), decimal.RequireFromString(c.amount), gt)
		if got != c.want {
			t.Fatalf("refunded=%s amount=%s => %s, want %s", c.refunded, c.amount, got, c.want)
		}
	}
}

// TestIdempotencyKey_StableAndColonFree proves idempotencyKey is
// deterministic for a given (OrderID, ScopeID) pair — required so saga
// retries reserve the same ledger row instead of creating duplicates — and
// that it never contains a colon, since Razorpay's X-Refund-Idempotency
// header rejects that charset.
func TestIdempotencyKey_StableAndColonFree(t *testing.T) {
	orderID := uuid.New()
	scopeID := "rr1"

	first := idempotencyKey(orderID, scopeID)
	second := idempotencyKey(orderID, scopeID)
	if first != second {
		t.Fatalf("idempotency key not stable: %q != %q", first, second)
	}
	if strings.Contains(first, ":") {
		t.Fatalf("idempotency key contains a colon: %q", first)
	}

	// Different order or scope must produce a different key.
	if idempotencyKey(uuid.New(), scopeID) == first {
		t.Fatalf("idempotency key collided across different order IDs")
	}
	if idempotencyKey(orderID, "rr2") == first {
		t.Fatalf("idempotency key collided across different scope IDs")
	}
}
