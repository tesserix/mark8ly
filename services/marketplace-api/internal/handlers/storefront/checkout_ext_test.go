package storefront

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/notification"
)

// TestCheckoutExtHandler_WithNotifier verifies WithNotifier wires the
// notification service onto the handler and returns the receiver for
// chaining, mirroring CheckoutHandler.WithNotifier. This is a live-bug
// regression guard: the extended storefront checkout handler previously
// had no notify field at all, so a merchant's "new order" bell alert and
// device push never fired for orders placed through the extended
// checkout path.
//
// A full checkout-flow test (asserting notification.Emit actually fires
// on a successful, non-reused order) would require standing up
// CheckoutExtHandler's order/coupon/gift-card/encryptor/tax/loyalty
// dependency graph against a real Postgres instance — that's covered by
// the //go:build integration suite in checkout_integration_test.go, not
// a package-local unit test.
func TestCheckoutExtHandler_WithNotifier(t *testing.T) {
	h := &CheckoutExtHandler{}
	svc := &notification.Service{}

	got := h.WithNotifier(svc)

	if got != h {
		t.Fatal("expected WithNotifier to return the same receiver for chaining")
	}
	if h.notify != svc {
		t.Fatal("expected WithNotifier to set the notify field to the provided service")
	}
}

// TestCheckoutExtHandler_WithNotifier_NilSafe confirms passing a nil
// notifier is accepted without panicking — WithNotifier itself must stay
// nil-safe since main.go wires it unconditionally.
func TestCheckoutExtHandler_WithNotifier_NilSafe(t *testing.T) {
	h := &CheckoutExtHandler{}

	got := h.WithNotifier(nil)

	if got != h {
		t.Fatal("expected WithNotifier to return the same receiver for chaining")
	}
	if h.notify != nil {
		t.Fatal("expected notify field to remain nil")
	}
}
