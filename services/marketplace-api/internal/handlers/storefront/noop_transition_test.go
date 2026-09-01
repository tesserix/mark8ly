package storefront

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Stripe sends BOTH checkout.session.completed and payment_intent.succeeded
// for one payment. The second arrives after the first confirmed the order,
// so Confirm refuses it with invalid_transition "confirmed" → "confirmed".
// That is a duplicate delivery, not a failure — and logging it at ERROR put
// a scary line in the logs for every successful payment, which is exactly
// how a real confirm failure gets missed.
func TestIsNoOpTransition(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "same state is a duplicate delivery",
			err:  apperrors.InvalidTransition("order", "confirmed", "confirmed"),
			want: true,
		},
		{
			name: "payment axis, same state",
			err:  apperrors.InvalidTransition("payment", "paid", "paid"),
			want: true,
		},
		// The case that must NEVER be swallowed: a genuinely wrong move.
		{
			name: "cancelled to confirmed is a real failure",
			err:  apperrors.InvalidTransition("order", "cancelled", "confirmed"),
			want: false,
		},
		{
			name: "pending to confirmed is a real failure",
			err:  apperrors.InvalidTransition("order", "pending", "confirmed"),
			want: false,
		},
		{
			name: "a different app error is not a no-op",
			err:  apperrors.NotFound("order"),
			want: false,
		},
		{
			name: "a plain error is not a no-op",
			err:  errors.New("db down"),
			want: false,
		},
		{
			name: "nil is not a no-op",
			err:  nil,
			want: false,
		},
		// Must survive wrapping: the webhook receives whatever the service
		// layer wrapped it in.
		{
			name: "wrapped same-state transition is still a duplicate",
			err:  fmt.Errorf("confirm: %w", apperrors.InvalidTransition("order", "confirmed", "confirmed")),
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoOpTransition(tc.err); got != tc.want {
				t.Errorf("isNoOpTransition(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
