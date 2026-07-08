//go:build integration

package payment

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestReserveRefund_Idempotent proves the ON CONFLICT (idempotency_key) DO
// NOTHING semantics: a second ReserveRefund call with the same idempotency
// key must not insert a second row — it must return the original row with
// created=false. This is the saga re-entry guard the RefundCoordinator
// (Task 8) relies on to make retries safe.
func TestReserveRefund_Idempotent(t *testing.T) {
	db := testdb.NewDB(t, "refund_transactions")
	svc := NewService(NewRepository(db))
	ctx := context.Background()

	in := ReserveRefundInput{
		TenantID:          uuid.New().String(),
		StoreID:           uuid.New().String(),
		OrderID:           uuid.New().String(),
		Provider:          "stripe",
		ProviderPaymentID: "pi_123",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "usd",
		Reason:            "customer requested",
		IdempotencyKey:    "refund:" + uuid.New().String(),
	}

	first, created, err := svc.ReserveRefund(ctx, db, in)
	if err != nil {
		t.Fatalf("first ReserveRefund: %v", err)
	}
	if !created {
		t.Fatalf("first ReserveRefund: created = false, want true")
	}
	if first.ID == "" {
		t.Fatal("first ReserveRefund: row has no ID")
	}
	if first.Status != "pending" {
		t.Fatalf("first ReserveRefund: status = %q, want pending", first.Status)
	}

	second, created, err := svc.ReserveRefund(ctx, db, in)
	if err != nil {
		t.Fatalf("second ReserveRefund: %v", err)
	}
	if created {
		t.Fatal("second ReserveRefund: created = true, want false (idempotency guard)")
	}
	if second.ID != first.ID {
		t.Fatalf("second ReserveRefund: ID = %q, want same as first %q", second.ID, first.ID)
	}

	rows, err := NewRepository(db).ListRefundsByPaymentID(ctx, "pi_123")
	if err != nil {
		t.Fatalf("ListRefundsByPaymentID: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("refund_transactions has %d rows for pi_123, want 1 (no duplicate insert)", len(rows))
	}
}

// TestFinalizeRefund_UpdatesStatusAndProviderRefundID confirms FinalizeRefund
// flips a pending row to a terminal status and stamps the provider refund id.
func TestFinalizeRefund_UpdatesStatusAndProviderRefundID(t *testing.T) {
	db := testdb.NewDB(t, "refund_transactions")
	svc := NewService(NewRepository(db))
	ctx := context.Background()

	in := ReserveRefundInput{
		TenantID:          uuid.New().String(),
		StoreID:           uuid.New().String(),
		OrderID:           uuid.New().String(),
		Provider:          "stripe",
		ProviderPaymentID: "pi_456",
		Amount:            decimal.NewFromInt(75),
		CurrencyCode:      "usd",
		Reason:            "defective item",
		IdempotencyKey:    "refund:" + uuid.New().String(),
	}

	row, created, err := svc.ReserveRefund(ctx, db, in)
	if err != nil {
		t.Fatalf("ReserveRefund: %v", err)
	}
	if !created {
		t.Fatal("ReserveRefund: created = false, want true")
	}

	if err := svc.FinalizeRefund(ctx, db, row.ID, "re_789", "succeeded"); err != nil {
		t.Fatalf("FinalizeRefund: %v", err)
	}

	updated, err := NewRepository(db).GetRefundByIdempotencyKey(ctx, in.IdempotencyKey)
	if err != nil {
		t.Fatalf("GetRefundByIdempotencyKey: %v", err)
	}
	if updated.Status != "succeeded" {
		t.Fatalf("Status = %q, want succeeded", updated.Status)
	}
	if updated.ProviderRefundID != "re_789" {
		t.Fatalf("ProviderRefundID = %q, want re_789", updated.ProviderRefundID)
	}
}

// TestFinalizeRefund_UnknownLedgerID_Errors proves FinalizeRefund cannot
// silently no-op: a ledgerID that matches zero refund_transactions rows must
// return an error rather than nil, otherwise the saga would falsely report
// success while the ledger row never reflects the gateway's outcome.
func TestFinalizeRefund_UnknownLedgerID_Errors(t *testing.T) {
	db := testdb.NewDB(t, "refund_transactions")
	svc := NewService(NewRepository(db))
	ctx := context.Background()

	err := svc.FinalizeRefund(ctx, db, uuid.New().String(), "re_x", "succeeded")
	if err == nil {
		t.Fatal("FinalizeRefund: err = nil, want non-nil for unknown ledger ID")
	}
}
