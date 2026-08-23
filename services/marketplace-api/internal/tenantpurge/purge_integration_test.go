//go:build integration

// This file requires TEST_DATABASE_URL pointed at a real Postgres with all
// migrations applied (see pkg/testdb doc comment and `make dev` / `make
// test-int`). In this environment no such database is reachable, so this
// file is verified to COMPILE (`GOWORK=off go vet -tags=integration ./...`)
// but every test in it SKIPS at runtime via testdb.NewDB -> openOrSkip.
//
// The seed below covers a representative table from every ordering group in
// purgePlan (financial leaves, order children, product/review subtree,
// vendors, tenant-only tables not reachable by the stores CASCADE, and a
// stores-CASCADE-swept config table) rather than literally all ~90 domain
// tables — extend seedTenant as new groups are added to purgePlan.
package tenantpurge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seededTenant holds the ids used to seed one tenant's worth of rows across
// every purgePlan group, so assertions can address them by name.
type seededTenant struct {
	tenantID  string
	storeID   string
	orderID   string
	returnID  string
	productID string
	reviewID  string
	vendorID  string
}

// domainTablesToCleanup lists every table this test writes to, in
// child-before-parent order, for testdb.NewDB's post-test TRUNCATE CASCADE.
var domainTablesToCleanup = []string{
	// group 1 — financial leaves
	"refund_transactions", "coupon_usage", "payment_transactions", "shipments",
	// group 2 — order children -> orders
	"order_items", "returns", "orders", "abandoned_carts",
	// group 3 — product/review subtree
	"reviews", "wishlists", "products", "categories",
	// group 4 — vendors
	"vendors",
	// group 5 — tenant-only, not reached by stores CASCADE
	"warehouses", "notifications", "pages", "customer_loyalties", "referrals",
	// group 6 — stores (CASCADEs coupons, campaigns, customer_profiles, ...)
	"coupons", "stores",
	// global tables touched by the test (seeded, must survive)
	"fx_rates",
}

func seedTenant(t *testing.T, db *gorm.DB, tenantID string) seededTenant {
	t.Helper()

	s := seededTenant{
		tenantID:  tenantID,
		storeID:   uuid.NewString(),
		orderID:   uuid.NewString(),
		returnID:  uuid.NewString(),
		productID: uuid.NewString(),
		reviewID:  uuid.NewString(),
		vendorID:  uuid.NewString(),
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed: %s: %v", sql, err)
		}
	}

	// stores (group 6 root)
	exec(`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
	      VALUES (?, ?, ?, 'Seed Store', 'US', 'USD', 'UTC', 'active', encode(gen_random_bytes(32), 'hex'))`,
		s.storeID, s.tenantID, "seed-store-"+s.storeID[:8])

	// group 6 — CASCADE-swept config table (proves stores CASCADE reaches it)
	exec(`INSERT INTO coupons (id, tenant_id, store_id, code, title, type, value, per_customer, target_type, stackable, status)
	      VALUES (?, ?, ?, ?, 'Seed Coupon', 'fixed_amount', 10, 1, 'all', false, 'active')`,
		uuid.NewString(), s.tenantID, s.storeID, "SEED"+s.storeID[:6])

	// group 4 — vendors (tenant_id only, no store_id/FK to stores)
	exec(`INSERT INTO vendors (id, tenant_id, name, slug, status, is_self)
	      VALUES (?, ?, 'Seed Vendor', ?, 'active', true)`,
		s.vendorID, s.tenantID, "seed-vendor-"+s.vendorID[:8])

	// group 3 — product/review subtree
	exec(`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id, tags)
	      VALUES (?, ?, ?, ?, 'Seed Product', 'draft', ?, '{}')`,
		s.productID, s.tenantID, s.storeID, "seed-product-"+s.productID[:8], s.vendorID)
	exec(`INSERT INTO categories (id, tenant_id, store_id, name, slug)
	      VALUES (?, ?, ?, 'Seed Category', ?)`,
		uuid.NewString(), s.tenantID, s.storeID, "seed-category-"+s.tenantID[:8])
	exec(`INSERT INTO reviews (id, tenant_id, store_id, product_id, customer_name, customer_email, rating, content, status)
	      VALUES (?, ?, ?, ?, 'Seed Customer', 'seed@example.com', 5, 'Great!', 'published')`,
		s.reviewID, s.tenantID, s.storeID, s.productID)
	// wishlists.customer_id requires a customer_profiles row; omitted here to
	// keep the seed focused on the tables actually asserted on below (see
	// domainTablesToCleanup) — extend if wishlists purging needs coverage.

	// group 2 — order children -> orders
	exec(`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email, subtotal, grand_total, currency_code)
	      VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 100, 100, 'USD')`,
		s.orderID, s.tenantID, s.storeID, "SEED-"+s.orderID[:8], "idem-"+s.orderID)
	exec(`INSERT INTO order_items (id, order_id, title_snapshot, sku_snapshot, unit_price, quantity, line_total, currency_code)
	      VALUES (?, ?, 'Seed Item', 'SEED-SKU', 100, 1, 100, 'USD')`,
		uuid.NewString(), s.orderID)
	exec(`INSERT INTO returns (id, tenant_id, store_id, order_id, return_number, currency_code)
	      VALUES (?, ?, ?, ?, ?, 'USD')`,
		s.returnID, s.tenantID, s.storeID, s.orderID, "SEED-R-"+s.returnID[:8])
	exec(`INSERT INTO abandoned_carts (id, tenant_id, store_id, cart_session_id, item_count, subtotal, currency_code, items_snapshot, last_active_at)
	      VALUES (?, ?, ?, ?, 1, 50, 'USD', '[]'::jsonb, now())`,
		uuid.NewString(), s.tenantID, s.storeID, "sess-"+s.tenantID[:8])

	// group 1 — financial leaves
	exec(`INSERT INTO payment_transactions (id, tenant_id, store_id, order_id, provider, amount, currency_code, status)
	      VALUES (?, ?, ?, ?, 'stripe', 100, 'USD', 'succeeded')`,
		uuid.NewString(), s.tenantID, s.storeID, s.orderID)
	exec(`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, ship_from, ship_to, currency_code)
	      VALUES (?, ?, ?, ?, 'shipengine', '{}'::jsonb, '{}'::jsonb, 'USD')`,
		uuid.NewString(), s.tenantID, s.storeID, s.orderID)
	exec(`INSERT INTO coupon_usage (id, tenant_id, coupon_id, order_id, customer_email, discount_amount, currency_code)
	      SELECT ?, ?, id, ?, 'buyer@example.com', 10, 'USD' FROM coupons WHERE tenant_id = ? LIMIT 1`,
		uuid.NewString(), s.tenantID, s.orderID, s.tenantID)

	// group 5 — tenant-only, no FK to stores at all
	exec(`INSERT INTO warehouses (id, tenant_id, store_id, name)
	      VALUES (?, ?, ?, 'Seed Warehouse')`,
		uuid.NewString(), s.tenantID, s.storeID)
	exec(`INSERT INTO notifications (id, tenant_id, store_id, type, title)
	      VALUES (?, ?, ?, 'new_order', 'Seed Notification')`,
		uuid.NewString(), s.tenantID, s.storeID)
	exec(`INSERT INTO pages (id, tenant_id, store_id, slug, title)
	      VALUES (?, ?, ?, ?, 'Seed Page')`,
		uuid.NewString(), s.tenantID, s.storeID, "seed-page-"+s.tenantID[:8])
	custLoyaltyID := uuid.NewString()
	exec(`INSERT INTO customer_loyalties (id, tenant_id, store_id, customer_email, referral_code)
	      VALUES (?, ?, ?, 'loyal@example.com', ?)`,
		custLoyaltyID, s.tenantID, s.storeID, "REF-"+custLoyaltyID[:8])

	return s
}

func countRows(t *testing.T, db *gorm.DB, table, whereCol, id string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM "+table+" WHERE "+whereCol+" = ?", id).Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestIntegration_Purge_DeletesTenantLeavesGlobalAndOtherTenantIntact(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	ctx := context.Background()

	// Ensure at least one row exists in a global table so we can assert it
	// survives the purge untouched.
	//
	// XTS is the ISO 4217 code officially reserved for testing purposes
	// (no real currency will ever use it), and fx_rates.currency is
	// character(3), so it must stay exactly 3 characters — do not swap
	// this for 'USD' or any other real code, that would risk colliding
	// with a currency another test cares about.
	if err := db.Exec(`INSERT INTO fx_rates (currency, usd_mid_rate, source) VALUES ('XTS', 1.0, 'test') ON CONFLICT (currency) DO NOTHING`).Error; err != nil {
		t.Fatalf("seed fx_rates: %v", err)
	}

	tenant1 := seedTenant(t, db, uuid.NewString())
	tenant2 := seedTenant(t, db, uuid.NewString())

	if err := tenantpurge.Purge(ctx, db, tenant1.tenantID, []string{tenant1.storeID}); err != nil {
		t.Fatalf("Purge tenant1: %v", err)
	}

	// tenant1 rows are gone across every seeded domain table.
	for _, table := range []string{"stores", "vendors", "products", "categories", "reviews", "orders",
		"returns", "abandoned_carts", "payment_transactions", "shipments", "coupon_usage", "coupons",
		"warehouses", "notifications", "pages", "customer_loyalties"} {
		if n := countRows(t, db, table, "tenant_id", tenant1.tenantID); n != 0 {
			t.Errorf("expected 0 rows in %s for purged tenant1, got %d", table, n)
		}
	}
	if n := countRows(t, db, "order_items", "order_id", tenant1.orderID); n != 0 {
		t.Errorf("expected 0 order_items for purged tenant1's order, got %d", n)
	}

	// global table untouched.
	var fxCount int64
	if err := db.Raw(`SELECT count(*) FROM fx_rates WHERE currency = 'XTS'`).Scan(&fxCount).Error; err != nil {
		t.Fatalf("count fx_rates: %v", err)
	}
	if fxCount != 1 {
		t.Errorf("expected the seeded global fx_rates row to survive, got count=%d", fxCount)
	}

	// tenant2's rows are fully intact.
	for _, table := range []string{"stores", "vendors", "products", "categories", "reviews", "orders",
		"returns", "abandoned_carts", "payment_transactions", "shipments", "coupons",
		"warehouses", "notifications", "pages", "customer_loyalties"} {
		if n := countRows(t, db, table, "tenant_id", tenant2.tenantID); n == 0 {
			t.Errorf("expected tenant2's rows in %s to remain, got 0", table)
		}
	}

	// idempotent: purging an already-purged tenant is a nil-error no-op.
	if err := tenantpurge.Purge(ctx, db, tenant1.tenantID, []string{tenant1.storeID}); err != nil {
		t.Fatalf("second Purge (idempotency check) returned error: %v", err)
	}
	if n := countRows(t, db, "stores", "tenant_id", tenant1.tenantID); n != 0 {
		t.Errorf("expected tenant1 to still have 0 stores after the idempotent re-run, got %d", n)
	}
}
