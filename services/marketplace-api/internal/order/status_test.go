package order_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mark8ly/marketplace-api/internal/order"
)

// TestOrderStatus_LegalTransitions iterates the full source × target matrix.
// It is the canonical guard that any change to the transition table is conscious.
func TestOrderStatus_LegalTransitions(t *testing.T) {
	all := []order.OrderStatus{
		order.OrderStatusPending, order.OrderStatusConfirmed,
		order.OrderStatusFulfilled, order.OrderStatusCancelled,
	}
	legal := map[order.OrderStatus]map[order.OrderStatus]bool{
		order.OrderStatusPending: {
			order.OrderStatusConfirmed: true,
			order.OrderStatusCancelled: true,
		},
		order.OrderStatusConfirmed: {
			order.OrderStatusFulfilled: true,
			order.OrderStatusCancelled: true,
		},
		// fulfilled, cancelled are terminal — no legal targets
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestOrderStatus_RefundedIsNotAValue(t *testing.T) {
	// Guards against accidentally adding OrderStatusRefunded back.
	// Calling CanTransitionTo with an unknown starting state returns false.
	assert.False(t, order.OrderStatus("refunded").CanTransitionTo(order.OrderStatusPending))
}

func TestPaymentStatus_LegalTransitions(t *testing.T) {
	all := []order.PaymentStatus{
		order.PaymentStatusPending, order.PaymentStatusAuthorized, order.PaymentStatusPaid,
		order.PaymentStatusFailed, order.PaymentStatusRefunded, order.PaymentStatusPartiallyRefunded,
	}
	legal := map[order.PaymentStatus]map[order.PaymentStatus]bool{
		order.PaymentStatusPending: {
			order.PaymentStatusAuthorized: true,
			order.PaymentStatusPaid:       true,
			order.PaymentStatusFailed:     true,
		},
		order.PaymentStatusAuthorized: {
			order.PaymentStatusPaid:     true,
			order.PaymentStatusFailed:   true,
			order.PaymentStatusRefunded: true,
		},
		order.PaymentStatusPaid: {
			order.PaymentStatusRefunded:          true,
			order.PaymentStatusPartiallyRefunded: true,
		},
		order.PaymentStatusPartiallyRefunded: {
			// self-transition: a second partial refund keeps the order
			// partially_refunded (see paymentStatusTransitions).
			order.PaymentStatusPartiallyRefunded: true,
			order.PaymentStatusRefunded:          true,
		},
		order.PaymentStatusFailed: {
			order.PaymentStatusPending: true,
			order.PaymentStatusPaid:    true,
		},
		// refunded is terminal
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestFulfillmentStatus_LegalTransitions(t *testing.T) {
	all := []order.FulfillmentStatus{
		order.FulfillmentStatusUnfulfilled,
		order.FulfillmentStatusPartial,
		order.FulfillmentStatusFulfilled,
	}
	legal := map[order.FulfillmentStatus]map[order.FulfillmentStatus]bool{
		order.FulfillmentStatusUnfulfilled: {
			order.FulfillmentStatusPartial:   true,
			order.FulfillmentStatusFulfilled: true,
		},
		order.FulfillmentStatusPartial: {
			order.FulfillmentStatusFulfilled: true,
		},
		// fulfilled is terminal
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestAllowed_ReturnsCopies(t *testing.T) {
	// Mutating the returned slice must not affect the internal table.
	s := order.OrderStatusPending
	a := s.Allowed()
	if len(a) == 0 {
		t.Fatal("expected allowed targets for pending")
	}
	a[0] = order.OrderStatus("hacked")
	b := s.Allowed()
	assert.NotEqual(t, a, b, "Allowed() must return a defensive copy")
}
