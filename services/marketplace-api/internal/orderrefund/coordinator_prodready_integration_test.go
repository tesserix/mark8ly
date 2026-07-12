//go:build integration

package orderrefund_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Two sequential partial refunds on one order must BOTH succeed and leave the
// order partially_refunded. Regression for the missing
// partially_refunded->partially_refunded transition, which previously moved
// money on the gateway for the second refund and then failed bookkeeping,
// stranding the ledger row as pending forever.
func TestRefund_TwoSequentialPartials_BothSucceed(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	first := decimal.RequireFromString("40.00")
	r1, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &first, Reason: "return A", Actor: "admin", ScopeID: "retA",
	})
	if err != nil {
		t.Fatalf("first partial: %v", err)
	}
	if r1.PaymentStatus != order.PaymentStatusPartiallyRefunded {
		t.Fatalf("first status = %q, want partially_refunded", r1.PaymentStatus)
	}

	second := decimal.RequireFromString("30.00")
	r2, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &second, Reason: "return B", Actor: "admin", ScopeID: "retB",
	})
	if err != nil {
		t.Fatalf("second partial: %v (regression: partial->partial transition)", err)
	}
	if r2.PaymentStatus != order.PaymentStatusPartiallyRefunded {
		t.Fatalf("second status = %q, want partially_refunded", r2.PaymentStatus)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("70.00")) {
		t.Fatalf("refunded_amount = %s, want 70.00", o.RefundedAmount)
	}
	for _, key := range []string{"retA", "retB"} {
		row := getRefundTxn(t, db, "refund_"+orderID.String()+"_"+key)
		if row.Status != "succeeded" {
			t.Fatalf("ledger %s status = %q, want succeeded", key, row.Status)
		}
	}
	if gw.callCount() != 2 {
		t.Fatalf("gateway call count = %d, want 2", gw.callCount())
	}
}

// Two concurrent refunds with DIFFERENT scope IDs that together exceed the
// cap must NOT both move money. The order-row lock + pending-sum cap check
// serialise them: exactly one succeeds, the other is rejected with
// ErrRefundExceedsTotal, and refunded_amount never exceeds the cap.
func TestRefund_ConcurrentDistinctScopes_NeverExceedCap(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "100.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "100.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("70.00") // 70 + 70 = 140 > 100 cap
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, scope := range []string{"scopeA", "scopeB"} {
		wg.Add(1)
		go func(idx int, sc string) {
			defer wg.Done()
			amt := amount
			_, errs[idx] = c.Refund(context.Background(), orderrefund.RefundCommand{
				OrderID: orderID, Amount: &amt, Reason: "concurrent", Actor: "admin", ScopeID: sc,
			})
		}(i, scope)
	}
	wg.Wait()

	successes, overCap := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, apperrors.ErrRefundExceedsTotal):
			overCap++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || overCap != 1 {
		t.Fatalf("outcomes: %d success, %d over-cap; want exactly 1 each", successes, overCap)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("70.00")) {
		t.Fatalf("refunded_amount = %s, want 70.00 (never both)", o.RefundedAmount)
	}
	if gw.callCount() != 1 {
		t.Fatalf("gateway call count = %d, want 1 (loser must be rejected before the gateway)", gw.callCount())
	}
	if n := countRefundTxns(t, db, orderID); n != 1 {
		t.Fatalf("refund_transactions rows = %d, want 1 (rejected reservation rolled back)", n)
	}
}

// A permanent gateway error (4xx) must move the ledger row to 'failed' so the
// sweeper stops re-driving it forever, and must not bump refunded_amount.
func TestRefund_PermanentGatewayError_MarksFailed(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{refundErr: &payment.GatewayError{Provider: "stripe", StatusCode: 400, Body: "no such payment_intent"}}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("50.00")
	_, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	})
	if err == nil {
		t.Fatal("Refund: err = nil, want permanent gateway error")
	}

	row := getRefundTxn(t, db, "refund_"+orderID.String()+"_rr1")
	if row.Status != "failed" {
		t.Fatalf("ledger status = %q, want failed (permanent error must not be re-driven)", row.Status)
	}
	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.IsZero() {
		t.Fatalf("refunded_amount = %s, want 0", o.RefundedAmount)
	}
}
