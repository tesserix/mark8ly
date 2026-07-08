//go:build integration

package orderrefund_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// coordinatorTruncateTables is the dependency-ordered truncate list for the
// Coordinator integration tests. Children (refund_transactions,
// payment_transactions, payment_gateway_configs) are listed before parents
// (orders, stores); order_events/outbox_events cascade automatically.
var coordinatorTruncateTables = []string{
	"refund_transactions",
	"payment_transactions",
	"payment_gateway_configs",
	"orders",
	"stores",
}

// seedPaidOrder inserts an orders row with payment_status='paid' so
// order.Service.RecordRefund's transition guard (paid -> refunded |
// partially_refunded) is satisfied.
func seedPaidOrder(t *testing.T, db *gorm.DB, orderID, storeID, tenantID uuid.UUID, grandTotal, refundedAmount string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email,
			status, payment_status, fulfillment_status,
			subtotal, tax_total, shipping_total, grand_total, refunded_amount,
			currency_code, placed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'test@example.com',
			'confirmed', 'paid', 'unfulfilled',
			?, '0', '0', ?, ?,
			'EUR', now(), now(), now())`,
		orderID, tenantID, storeID, "RO-"+orderID.String()[:8], "idem-"+orderID.String(),
		grandTotal, grandTotal, refundedAmount,
	).Error
	if err != nil {
		t.Fatalf("seedPaidOrder: %v", err)
	}
}

// fakeGateway implements payment.Gateway. Only RefundPayment does anything
// interesting for these tests; it records every call so tests can assert
// the gateway was (or was not) invoked.
type fakeGateway struct {
	mu    sync.Mutex
	calls []payment.RefundInput
}

func (f *fakeGateway) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeGateway) lastCall() payment.RefundInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func (f *fakeGateway) CreateIntent(ctx context.Context, in payment.CreateIntentInput) (*payment.Intent, error) {
	return nil, errors.New("fakeGateway: CreateIntent not implemented")
}

func (f *fakeGateway) CapturePayment(ctx context.Context, captureID string) (*payment.Capture, error) {
	return nil, errors.New("fakeGateway: CapturePayment not implemented")
}

func (f *fakeGateway) RefundPayment(ctx context.Context, in payment.RefundInput) (*payment.Refund, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	f.mu.Unlock()
	return &payment.Refund{ProviderRefundID: "re_test", Status: "succeeded", Amount: in.Amount}, nil
}

func (f *fakeGateway) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*payment.WebhookEvent, error) {
	return nil, errors.New("fakeGateway: VerifyWebhook not implemented")
}

func (f *fakeGateway) ProviderName() string { return "stripe" }

func (f *fakeGateway) SupportedCountries() []string { return []string{"IE"} }

// fakeResolver embeds the real *orderrefund.Resolver (so PaymentContextForOrder
// reads seeded rows for real) but overrides GatewayFor to hand back a
// fakeGateway instead of constructing a real provider client.
type fakeResolver struct {
	*orderrefund.Resolver
	gw payment.Gateway
}

func (f *fakeResolver) GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error) {
	return f.gw, nil
}

// newCoordinator wires a Coordinator against real payment/order services and
// a fakeResolver, for the given db and enabled flag.
func newCoordinator(db *gorm.DB, gw *fakeGateway, enabled bool) *orderrefund.Coordinator {
	res := &fakeResolver{Resolver: orderrefund.NewResolver(db), gw: gw}
	pay := payment.NewService(payment.NewRepository(db))
	orders := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	return orderrefund.NewCoordinator(db, res, pay, orders, order.NewRepository(), enabled)
}

func getOrder(t *testing.T, db *gorm.DB, orderID uuid.UUID) order.Order {
	t.Helper()
	var o order.Order
	if err := db.Where("id = ?", orderID).First(&o).Error; err != nil {
		t.Fatalf("getOrder: %v", err)
	}
	return o
}

func getRefundTxn(t *testing.T, db *gorm.DB, idempotencyKey string) payment.RefundTransaction {
	t.Helper()
	var row payment.RefundTransaction
	if err := db.Where("idempotency_key = ?", idempotencyKey).First(&row).Error; err != nil {
		t.Fatalf("getRefundTxn: %v", err)
	}
	return row
}

func countRefundTxns(t *testing.T, db *gorm.DB, orderID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&payment.RefundTransaction{}).Where("order_id = ?", orderID).Count(&n).Error; err != nil {
		t.Fatalf("countRefundTxns: %v", err)
	}
	return n
}

// (a) Partial refund: Amount=50 on a 120 order captures a partially_refunded
// order, a succeeded ledger row keyed correctly, and the gateway sees the
// same idempotency key.
func TestRefund_Partial(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("50.00")
	result, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if result.PaymentStatus != order.PaymentStatusPartiallyRefunded {
		t.Fatalf("PaymentStatus = %q, want partially_refunded", result.PaymentStatus)
	}
	if result.AlreadyDone {
		t.Fatalf("AlreadyDone = true, want false")
	}

	wantKey := "refund_" + orderID.String() + "_rr1"
	row := getRefundTxn(t, db, wantKey)
	if row.Status != "succeeded" {
		t.Fatalf("ledger status = %q, want succeeded", row.Status)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("refunded_amount = %s, want 50.00", o.RefundedAmount)
	}

	if gw.callCount() != 1 {
		t.Fatalf("gateway call count = %d, want 1", gw.callCount())
	}
	if gw.lastCall().IdempotencyKey != wantKey {
		t.Fatalf("gateway saw IdempotencyKey = %q, want %q", gw.lastCall().IdempotencyKey, wantKey)
	}
}

// (b) Full refund via nil Amount refunds the entire remaining balance.
func TestRefund_FullNilAmount(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	result, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: nil, Reason: "cancel", Actor: "admin", ScopeID: "cancel",
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if result.PaymentStatus != order.PaymentStatusRefunded {
		t.Fatalf("PaymentStatus = %q, want refunded", result.PaymentStatus)
	}
	if !result.Amount.Equal(decimal.RequireFromString("120.00")) {
		t.Fatalf("Amount = %s, want 120.00", result.Amount)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("120.00")) {
		t.Fatalf("refunded_amount = %s, want 120.00", o.RefundedAmount)
	}
}

// (c) Over cap: requesting more than the captured total is rejected before
// the gateway is ever called.
func TestRefund_OverCap_Rejected(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("200.00")
	_, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	})
	if !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("err = %v, want ErrRefundExceedsTotal", err)
	}
	if gw.callCount() != 0 {
		t.Fatalf("gateway call count = %d, want 0", gw.callCount())
	}
	if n := countRefundTxns(t, db, orderID); n != 0 {
		t.Fatalf("refund_transactions rows = %d, want 0", n)
	}
}

// (d) No captured transaction (only a pending row) → RefundUnavailable,
// gateway never called.
func TestRefund_NoCapturedTxn_Unavailable(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "pending",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("50.00")
	_, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	})
	if !errors.Is(err, apperrors.ErrRefundUnavailable) {
		t.Fatalf("err = %v, want ErrRefundUnavailable", err)
	}
	if gw.callCount() != 0 {
		t.Fatalf("gateway call count = %d, want 0", gw.callCount())
	}
}

// (e) Disabled coordinator returns an error and never touches the gateway
// or the ledger.
func TestRefund_Disabled_SkipsGateway(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, false)

	amount := decimal.RequireFromString("50.00")
	_, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	})
	if err == nil {
		t.Fatal("Refund: err = nil, want non-nil (coordinator disabled)")
	}
	if gw.callCount() != 0 {
		t.Fatalf("gateway call count = %d, want 0", gw.callCount())
	}
	if n := countRefundTxns(t, db, orderID); n != 0 {
		t.Fatalf("refund_transactions rows = %d, want 0", n)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.IsZero() {
		t.Fatalf("refunded_amount = %s, want 0 (disabled path must not bookkeep)", o.RefundedAmount)
	}
}

// (f) Idempotent replay: calling Refund twice with the same ScopeID must
// not double-charge — the second call sees the already-succeeded ledger row
// and returns AlreadyDone without calling the gateway or bumping
// refunded_amount again.
func TestRefund_IdempotentReplay(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	amount := decimal.RequireFromString("50.00")
	cmd := orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amount, Reason: "customer request", Actor: "admin", ScopeID: "rr1",
	}

	first, err := c.Refund(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first Refund: %v", err)
	}
	if first.AlreadyDone {
		t.Fatalf("first Refund: AlreadyDone = true, want false")
	}

	second, err := c.Refund(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second Refund: %v", err)
	}
	if !second.AlreadyDone {
		t.Fatalf("second Refund: AlreadyDone = false, want true (idempotent replay)")
	}

	if gw.callCount() != 1 {
		t.Fatalf("gateway call count = %d, want 1 (second call must not re-charge)", gw.callCount())
	}
	if n := countRefundTxns(t, db, orderID); n != 1 {
		t.Fatalf("refund_transactions rows = %d, want 1 (no duplicate ledger row)", n)
	}

	o := getOrder(t, db, orderID)
	if !o.RefundedAmount.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("refunded_amount = %s, want 50.00 (bumped once, not twice)", o.RefundedAmount)
	}
}
