//go:build integration

package orderrefund_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedPendingRefundTxn inserts a refund_transactions row directly in status
// 'pending', as if the gateway call in Coordinator.Refund succeeded but the
// process crashed (or the DB blipped) before tx #2 committed. createdAt is
// set explicitly so tests can control whether ResumePending's
// olderThan window selects the row.
func seedPendingRefundTxn(t *testing.T, db *gorm.DB, tenantID, storeID, orderID uuid.UUID, idempotencyKey, providerPaymentID, provider, amount string, createdAt time.Time) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO refund_transactions
			(id, tenant_id, store_id, order_id, idempotency_key, provider_payment_id, provider_refund_id, provider, amount, reason, status, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, '', ?, ?, 'cancel', 'pending', ?, ?)`,
		tenantID, storeID, orderID, idempotencyKey, providerPaymentID, provider, amount, createdAt, createdAt,
	).Error
	if err != nil {
		t.Fatalf("seedPendingRefundTxn: %v", err)
	}
}

// TestResumePending_CompletesStuckRefund seeds a pending ledger row (as if
// tx #2 crashed after the gateway call succeeded) and asserts ResumePending
// re-drives it to completion exactly once: same idempotency key hits the
// gateway, the ledger flips to succeeded, and refunded_amount is bumped by
// exactly the ledger row's amount (never double-bumped).
func TestResumePending_CompletesStuckRefund(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "100.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "100.00", Status: "captured",
	})
	seedGatewayConfig(t, db, tenantID, storeID, "stripe", true)

	wantKey := "refund_" + orderID.String() + "_cancel"
	seedPendingRefundTxn(t, db, tenantID, storeID, orderID, wantKey, "pi_x", "stripe", "100.00", time.Now().Add(-1*time.Hour))

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	n, err := c.ResumePending(context.Background(), 0, 200)
	if err != nil {
		t.Fatalf("ResumePending: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResumePending resumed = %d, want 1", n)
	}

	if gw.callCount() != 1 {
		t.Fatalf("gateway call count = %d, want 1", gw.callCount())
	}
	if gw.lastCall().IdempotencyKey != wantKey {
		t.Fatalf("gateway saw IdempotencyKey = %q, want %q", gw.lastCall().IdempotencyKey, wantKey)
	}

	row := getRefundTxn(t, db, wantKey)
	if row.Status != "succeeded" {
		t.Fatalf("ledger status = %q, want succeeded", row.Status)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("refunded_amount = %s, want 100.00 (bumped exactly once)", o.RefundedAmount)
	}
	if o.PaymentStatus != string(order.PaymentStatusRefunded) {
		t.Fatalf("payment_status = %q, want refunded", o.PaymentStatus)
	}
}

// TestResumePending_NoPendingRows asserts that a sweep with nothing stuck is
// a normal, successful no-op: zero resumed, gateway never touched.
func TestResumePending_NoPendingRows(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	n, err := c.ResumePending(context.Background(), 0, 200)
	if err != nil {
		t.Fatalf("ResumePending: %v", err)
	}
	if n != 0 {
		t.Fatalf("ResumePending resumed = %d, want 0", n)
	}
	if gw.callCount() != 0 {
		t.Fatalf("gateway call count = %d, want 0", gw.callCount())
	}
}

// TestResumePending_GatewayStillFailing asserts that when the gateway is
// still erroring, ResumePending leaves the row pending (not succeeded),
// does not bump refunded_amount, and reports 0 resumed — so the next sweep
// tries again rather than silently losing the row.
func TestResumePending_GatewayStillFailing(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "100.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "100.00", Status: "captured",
	})
	seedGatewayConfig(t, db, tenantID, storeID, "stripe", true)

	wantKey := "refund_" + orderID.String() + "_cancel"
	seedPendingRefundTxn(t, db, tenantID, storeID, orderID, wantKey, "pi_x", "stripe", "100.00", time.Now().Add(-1*time.Hour))

	gw := &fakeGateway{refundErr: errors.New("gateway: still down")}
	c := newCoordinator(db, gw, true)

	n, err := c.ResumePending(context.Background(), 0, 200)
	if err != nil {
		t.Fatalf("ResumePending: %v", err)
	}
	if n != 0 {
		t.Fatalf("ResumePending resumed = %d, want 0", n)
	}

	row := getRefundTxn(t, db, wantKey)
	if row.Status != "pending" {
		t.Fatalf("ledger status = %q, want pending (gateway still failing, retry next sweep)", row.Status)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.IsZero() {
		t.Fatalf("refunded_amount = %s, want 0 (gateway failed, must not bookkeep)", o.RefundedAmount)
	}
}
