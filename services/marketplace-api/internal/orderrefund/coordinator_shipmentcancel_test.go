package orderrefund

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/order"
)

func TestCoordinator_maybeCancelShipments_FiresOnFullRefundOnly(t *testing.T) {
	var got []uuid.UUID
	c := (&Coordinator{}).WithShipmentCanceller(func(_ context.Context, oid uuid.UUID) {
		got = append(got, oid)
	})

	oid := uuid.New()
	// Full refund → fires.
	c.maybeCancelShipments(context.Background(), oid, order.PaymentStatusRefunded)
	// Partial refund → does not fire.
	c.maybeCancelShipments(context.Background(), uuid.New(), order.PaymentStatusPartiallyRefunded)

	if len(got) != 1 || got[0] != oid {
		t.Fatalf("cancel fired for %v, want exactly [%v]", got, oid)
	}
}

func TestCoordinator_maybeCancelShipments_NilHookSafe(t *testing.T) {
	c := &Coordinator{}
	// Must not panic when no canceller is wired.
	c.maybeCancelShipments(context.Background(), uuid.New(), order.PaymentStatusRefunded)
}
