//go:build integration

// White-box (in-package) test for handlePaymentSucceeded's persistence of
// provider_payment_id. This must live in package storefront (not
// storefront_test) because handlePaymentSucceeded is unexported.
//
// The WebhookHandler is constructed directly with orderSvc left nil, which
// is safe: handlePaymentSucceeded only invokes the order-confirm path when
// both h.orderSvc != nil AND evt.OrderID != "". Leaving orderSvc nil lets
// this test exercise the payment_transactions UPDATE in isolation without
// standing up the full order.Service + notification/loyalty/docmailer
// dependency graph exercised by webhooks.go's other collaborators.
package storefront

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var captureTestTruncateTables = []string{
	"payment_transactions",
	"orders",
	"stores",
}

func seedCaptureTestStore(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, "wh-"+storeID.String()[:8], "Webhook Capture Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedCaptureTestStore: %v", err)
	}
}

func seedCaptureTestOrder(t *testing.T, db *gorm.DB, orderID, storeID, tenantID uuid.UUID) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email,
			status, payment_status, fulfillment_status,
			subtotal, tax_total, shipping_total, grand_total, refunded_amount,
			currency_code, placed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'test@example.com',
			'pending', 'pending', 'unfulfilled',
			'100.00', '0', '0', '100.00', '0',
			'EUR', now(), now(), now())`,
		orderID, tenantID, storeID, "WH-"+orderID.String()[:8], "idem-"+orderID.String(),
	).Error
	if err != nil {
		t.Fatalf("seedCaptureTestOrder: %v", err)
	}
}

func seedCaptureTestPaymentTxn(t *testing.T, db *gorm.DB, tenantID, storeID, orderID uuid.UUID, provider string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_transactions
			(id, tenant_id, store_id, order_id, provider, provider_intent_id, amount, currency_code, status, metadata)
		 VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, '100.00', 'EUR', 'pending', '{}'::jsonb)`,
		tenantID, storeID, orderID, provider, "intent_"+orderID.String(),
	).Error
	if err != nil {
		t.Fatalf("seedCaptureTestPaymentTxn: %v", err)
	}
}

func fetchCaptureTestRow(t *testing.T, db *gorm.DB, orderID uuid.UUID) (status, providerPaymentID string) {
	t.Helper()
	var row struct {
		Status            string `gorm:"column:status"`
		ProviderPaymentID string `gorm:"column:provider_payment_id"`
	}
	if err := db.Table("payment_transactions").
		Select("status, provider_payment_id").
		Where("order_id = ?", orderID).
		Take(&row).Error; err != nil {
		t.Fatalf("fetchCaptureTestRow: %v", err)
	}
	return row.Status, row.ProviderPaymentID
}

// TestHandlePaymentSucceeded_PersistsProviderPaymentID is the Step 0
// regression test: Razorpay/PayPal refund targets live in
// provider_payment_id, which was previously discarded by
// handlePaymentSucceeded. This exercises the exact SQL path both the
// webhook route and the Razorpay client-verify route
// (payment_verify.go) funnel through.
func TestHandlePaymentSucceeded_PersistsProviderPaymentID(t *testing.T) {
	db := testdb.NewDB(t, captureTestTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedCaptureTestStore(t, db, storeID, tenantID)
	seedCaptureTestOrder(t, db, orderID, storeID, tenantID)
	seedCaptureTestPaymentTxn(t, db, tenantID, storeID, orderID, "razorpay")

	h := &WebhookHandler{db: db}
	evt := &payment.WebhookEvent{
		OrderID:           orderID.String(),
		PaymentMethod:     "card",
		ProviderPaymentID: "pay_ABC123",
	}
	h.handlePaymentSucceeded(context.Background(), "razorpay", evt)

	status, providerPaymentID := fetchCaptureTestRow(t, db, orderID)
	if status != "captured" {
		t.Fatalf("status = %q, want captured", status)
	}
	if providerPaymentID != "pay_ABC123" {
		t.Fatalf("provider_payment_id = %q, want pay_ABC123", providerPaymentID)
	}
}

// TestHandlePaymentSucceeded_PreservesExistingProviderPaymentIDWhenEventLacksOne
// covers the COALESCE(NULLIF(?, ''), provider_payment_id) branch: an event
// with an empty ProviderPaymentID (e.g. a retried/duplicate webhook, or a
// provider that doesn't carry one on every event) must not clobber a value
// already persisted from an earlier capture.
func TestHandlePaymentSucceeded_PreservesExistingProviderPaymentIDWhenEventLacksOne(t *testing.T) {
	db := testdb.NewDB(t, captureTestTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedCaptureTestStore(t, db, storeID, tenantID)
	seedCaptureTestOrder(t, db, orderID, storeID, tenantID)
	seedCaptureTestPaymentTxn(t, db, tenantID, storeID, orderID, "razorpay")

	h := &WebhookHandler{db: db}

	// First event carries the captured payment id.
	h.handlePaymentSucceeded(context.Background(), "razorpay", &payment.WebhookEvent{
		OrderID:           orderID.String(),
		PaymentMethod:     "card",
		ProviderPaymentID: "pay_first",
	})
	_, providerPaymentID := fetchCaptureTestRow(t, db, orderID)
	if providerPaymentID != "pay_first" {
		t.Fatalf("after first event: provider_payment_id = %q, want pay_first", providerPaymentID)
	}

	// Second event (e.g. a retry) carries no ProviderPaymentID — must
	// preserve the value already persisted, not overwrite it with empty.
	h.handlePaymentSucceeded(context.Background(), "razorpay", &payment.WebhookEvent{
		OrderID:           orderID.String(),
		PaymentMethod:     "card",
		ProviderPaymentID: "",
	})
	_, providerPaymentID = fetchCaptureTestRow(t, db, orderID)
	if providerPaymentID != "pay_first" {
		t.Fatalf("after empty-id event: provider_payment_id = %q, want preserved pay_first", providerPaymentID)
	}
}

// TestHandlePaymentSucceeded_NilOrderServiceSkipsConfirm is a sanity check
// that the test harness pattern used above (orderSvc left nil) is itself
// safe — handlePaymentSucceeded must not panic when orderSvc is nil even
// though evt.OrderID is set.
func TestHandlePaymentSucceeded_NilOrderServiceSkipsConfirm(t *testing.T) {
	db := testdb.NewDB(t, captureTestTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedCaptureTestStore(t, db, storeID, tenantID)
	seedCaptureTestOrder(t, db, orderID, storeID, tenantID)
	seedCaptureTestPaymentTxn(t, db, tenantID, storeID, orderID, "stripe")

	h := &WebhookHandler{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.handlePaymentSucceeded(ctx, "stripe", &payment.WebhookEvent{
		OrderID:           orderID.String(),
		PaymentMethod:     "card",
		ProviderPaymentID: "pi_stripe123",
	})

	status, providerPaymentID := fetchCaptureTestRow(t, db, orderID)
	if status != "captured" || providerPaymentID != "pi_stripe123" {
		t.Fatalf("status=%q provider_payment_id=%q, want captured/pi_stripe123", status, providerPaymentID)
	}
}
