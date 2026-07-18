package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/order"
)

type recordingCanceller struct{ ids []uuid.UUID }

func (r *recordingCanceller) CancelForOrder(_ context.Context, oid uuid.UUID) {
	r.ids = append(r.ids, oid)
}

func TestCancel_NonPaid_CancelsShipmentDirectly(t *testing.T) {
	rc := &recordingCanceller{}
	h := (&OrdersHandler{}).WithShipmentCanceller(rc.CancelForOrder)

	oid := uuid.New()
	// Non-paid cancel → direct shipment cancel.
	h.cancelShipmentsForNonPaid(context.Background(), oid, string(order.PaymentStatusPending))
	// Paid cancel → defers to the coordinator's post-refund hook, no direct call.
	h.cancelShipmentsForNonPaid(context.Background(), uuid.New(), string(order.PaymentStatusPaid))

	if len(rc.ids) != 1 || rc.ids[0] != oid {
		t.Fatalf("direct cancel fired for %v, want exactly [%v] (paid path defers to coordinator)", rc.ids, oid)
	}
}

func TestCancel_NonPaid_NilCancellerSafe(t *testing.T) {
	h := &OrdersHandler{}
	// Must not panic when no canceller is wired.
	h.cancelShipmentsForNonPaid(context.Background(), uuid.New(), string(order.PaymentStatusPending))
}
