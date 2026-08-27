//go:build integration

package orderrefund_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// resolverTruncateTables is the dependency-ordered truncate list for the
// orderrefund resolver tests. payment_transactions FKs to orders, and
// payment_gateway_configs FKs to stores, so children are listed before
// parents; TRUNCATE ... CASCADE (see pkg/testdb) handles anything missed.
var resolverTruncateTables = []string{
	"payment_transactions",
	"payment_gateway_configs",
	"orders",
	"stores",
}

// seedStore inserts a minimal stores row satisfying all NOT NULL columns,
// including storefront_customer_portal_secret which has no default
// (migration 000058 drops the default after backfilling existing rows).
func seedStore(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, "rs-"+storeID.String()[:8], "Resolver Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedStore: %v", err)
	}
}

// seedOrder inserts a minimal orders row so payment_transactions rows (which
// FK to orders.id ON DELETE RESTRICT) can be seeded against it.
func seedOrder(t *testing.T, db *gorm.DB, orderID, storeID, tenantID uuid.UUID) {
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
		orderID, tenantID, storeID, "RO-"+orderID.String()[:8], "idem-"+orderID.String(),
	).Error
	if err != nil {
		t.Fatalf("seedOrder: %v", err)
	}
}

// seedPaymentTxnOpts controls the optional fields of a seeded
// payment_transactions row so tests can exercise each resolver branch.
type seedPaymentTxnOpts struct {
	Provider          string
	ProviderPaymentID string // may be "" to leave the column NULL
	ProviderIntentID  string // may be "" to leave the column NULL
	Amount            string
	Status            string
	CreatedAt         time.Time
}

func seedPaymentTxn(t *testing.T, db *gorm.DB, tenantID, storeID, orderID uuid.UUID, opts seedPaymentTxnOpts) {
	t.Helper()
	var providerPaymentID, providerIntentID any
	if opts.ProviderPaymentID != "" {
		providerPaymentID = opts.ProviderPaymentID
	}
	if opts.ProviderIntentID != "" {
		providerIntentID = opts.ProviderIntentID
	}
	err := db.Exec(
		`INSERT INTO payment_transactions
			(id, tenant_id, store_id, order_id, provider, provider_intent_id, provider_payment_id, amount, currency_code, status, metadata, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, ?, ?, 'EUR', ?, '{}'::jsonb, ?, ?)`,
		tenantID, storeID, orderID, opts.Provider, providerIntentID, providerPaymentID, opts.Amount, opts.Status, opts.CreatedAt, opts.CreatedAt,
	).Error
	if err != nil {
		t.Fatalf("seedPaymentTxn: %v", err)
	}
}

func seedGatewayConfig(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, provider string, isActive bool) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_gateway_configs (id, tenant_id, store_id, provider, api_key_encrypted, secret_key_encrypted, mode, is_active)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'test-api-key', 'test-secret-key', 'test', ?)`,
		tenantID, storeID, provider, isActive,
	).Error
	if err != nil {
		t.Fatalf("seedGatewayConfig: %v", err)
	}
}

func TestPaymentContextForOrder_CapturedTotal(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedOrder(t, db, orderID, storeID, tenantID)

	base := time.Now().Add(-1 * time.Hour)
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider:          "stripe",
		ProviderPaymentID: "pi_earliest",
		Amount:            "60.00",
		Status:            "captured",
		CreatedAt:         base,
	})
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider:          "stripe",
		ProviderPaymentID: "pi_second",
		Amount:            "40.00",
		Status:            "captured",
		CreatedAt:         base.Add(1 * time.Minute),
	})

	r := orderrefund.NewResolver(db)
	pc, err := r.PaymentContextForOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("PaymentContextForOrder: %v", err)
	}
	if !pc.Found {
		t.Fatalf("Found = false, want true")
	}
	if pc.Provider != "stripe" {
		t.Fatalf("Provider = %q, want stripe", pc.Provider)
	}
	if pc.ProviderPaymentID != "pi_earliest" {
		t.Fatalf("ProviderPaymentID = %q, want pi_earliest (earliest row)", pc.ProviderPaymentID)
	}
	if !pc.CapturedTotal.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("CapturedTotal = %s, want 100.00", pc.CapturedTotal)
	}
}

func TestPaymentContextForOrder_PrefersProviderPaymentIDOverIntentID(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedOrder(t, db, orderID, storeID, tenantID)

	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider:          "razorpay",
		ProviderPaymentID: "pay_captured123",
		ProviderIntentID:  "order_intent456",
		Amount:            "75.00",
		Status:            "captured",
		CreatedAt:         time.Now(),
	})

	r := orderrefund.NewResolver(db)
	pc, err := r.PaymentContextForOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("PaymentContextForOrder: %v", err)
	}
	if !pc.Found {
		t.Fatalf("Found = false, want true")
	}
	if pc.ProviderPaymentID != "pay_captured123" {
		t.Fatalf("ProviderPaymentID = %q, want pay_captured123 (must prefer provider_payment_id)", pc.ProviderPaymentID)
	}
}

func TestPaymentContextForOrder_FallsBackToProviderIntentID(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedOrder(t, db, orderID, storeID, tenantID)

	// Legacy row captured before the Step 0 fix: only provider_intent_id was
	// ever persisted. Stripe's intent IS the refund target, so this must
	// still resolve to a usable ProviderPaymentID.
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider:         "stripe",
		ProviderIntentID: "pi_legacy_only",
		Amount:           "30.00",
		Status:           "captured",
		CreatedAt:        time.Now(),
	})

	r := orderrefund.NewResolver(db)
	pc, err := r.PaymentContextForOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("PaymentContextForOrder: %v", err)
	}
	if !pc.Found {
		t.Fatalf("Found = false, want true")
	}
	if pc.ProviderPaymentID != "pi_legacy_only" {
		t.Fatalf("ProviderPaymentID = %q, want pi_legacy_only (fallback to provider_intent_id)", pc.ProviderPaymentID)
	}
}

func TestPaymentContextForOrder_NotCaptured(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedOrder(t, db, orderID, storeID, tenantID)

	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider:          "stripe",
		ProviderPaymentID: "pi_pending",
		Amount:            "50.00",
		Status:            "pending",
		CreatedAt:         time.Now(),
	})

	r := orderrefund.NewResolver(db)
	pc, err := r.PaymentContextForOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("PaymentContextForOrder: %v", err)
	}
	if pc.Found {
		t.Fatalf("Found = true, want false (no captured rows)")
	}
}

func TestGatewayFor_ActiveConfig(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedGatewayConfig(t, db, tenantID, storeID, "stripe", true)

	r := orderrefund.NewResolver(db).WithEncryptor(crypto.NewNoopEncryptor())
	gw, err := r.GatewayFor(context.Background(), storeID, "stripe")
	if err != nil {
		t.Fatalf("GatewayFor: %v", err)
	}
	if gw == nil {
		t.Fatalf("gateway is nil")
	}
	if gw.ProviderName() != "stripe" {
		t.Fatalf("ProviderName() = %q, want stripe", gw.ProviderName())
	}
}

func TestGatewayFor_InactiveConfig(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedGatewayConfig(t, db, tenantID, storeID, "stripe", false)

	r := orderrefund.NewResolver(db)
	if _, err := r.GatewayFor(context.Background(), storeID, "stripe"); err == nil {
		t.Fatalf("expected error for inactive gateway config, got nil")
	}
}

func TestGatewayFor_MissingConfig(t *testing.T) {
	db := testdb.NewDB(t, resolverTruncateTables...)
	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)

	r := orderrefund.NewResolver(db)
	if _, err := r.GatewayFor(context.Background(), storeID, "stripe"); err == nil {
		t.Fatalf("expected error for missing gateway config, got nil")
	}
}
