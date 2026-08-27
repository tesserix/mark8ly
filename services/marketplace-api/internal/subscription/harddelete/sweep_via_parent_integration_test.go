//go:build integration

// Package harddelete_test covers the §15.2 hard-delete sweep (issue #360):
// nine tables in the Sweep list have no store_id column of their own, so
// `DELETE FROM <table> WHERE store_id = ?` raised SQLSTATE 42703 on the
// first one, aborted the transaction, and the 150-day GDPR hard-delete
// never completed for any store. These tests seed rows in all nine tables
// (plus the parent each is reached through), run Sweep, and assert every
// row disappears — for the swept store only.
package harddelete_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription/harddelete"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// nineTableFixture is the id of every row seeded by seedNineTableFixture,
// keyed by table name, so tests can assert on exact rows rather than just
// counts.
type nineTableFixture struct {
	reviewID   uuid.UUID
	loyaltyID  uuid.UUID
	campaignID uuid.UUID
	giftCardID uuid.UUID
	couponID   uuid.UUID
	ticketID   uuid.UUID
	productID  uuid.UUID
	categoryID uuid.UUID
	orderID    uuid.UUID
}

// swweptTables lists every table (parents + the nine store_id-less
// children) seedNineTableFixture populates, in an order safe for cleanup
// (children before parents; TRUNCATE ... CASCADE makes strict order
// unnecessary but this keeps the list self-documenting).
var sweptTables = []string{
	"review_reactions", "review_replies", "review_media", "reviews",
	"loyalty_transactions", "customer_loyalties",
	"campaign_recipients", "campaigns",
	"gift_card_transactions", "gift_cards",
	"coupon_usage", "coupons",
	"ticket_replies", "tickets",
	"product_categories", "products", "categories", "vendors",
	"customer_profiles",
	"orders",
	"audit_logs",
	"stores",
}

// seedNineTableFixture inserts a store plus one row in each of the nine
// store_id-less tables named in issue #360 (and the parent each is FK'd
// to). It returns the ids needed to assert on specific rows.
func seedNineTableFixture(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) nineTableFixture {
	t.Helper()

	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error, "seed stores")

	// vendors (products.vendor_id is NOT NULL as of migration 000028).
	vendorID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO vendors (id, tenant_id, name, slug) VALUES (?, ?, ?, ?)`,
		vendorID, tenantID, "Test Vendor", "vendor-"+vendorID.String()[:20],
	).Error, "seed vendors")

	// products + categories (parents of product_categories).
	productID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, vendor_id) VALUES (?, ?, ?, ?, ?, ?)`,
		productID, tenantID, storeID, "prod-"+productID.String()[:8], "Test Product", vendorID,
	).Error, "seed products")

	categoryID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO categories (id, tenant_id, store_id, name, slug) VALUES (?, ?, ?, ?, ?)`,
		categoryID, tenantID, storeID, "Test Category", "cat-"+categoryID.String()[:8],
	).Error, "seed categories")

	require.NoError(t, db.Exec(
		`INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`,
		productID, categoryID,
	).Error, "seed product_categories")

	// customer_profiles (parent of review_reactions).
	customerProfileID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO customer_profiles (id, tenant_id, store_id, email) VALUES (?, ?, ?, ?)`,
		customerProfileID, tenantID, storeID, "cust-"+customerProfileID.String()[:8]+"@example.com",
	).Error, "seed customer_profiles")

	// reviews (parent of review_reactions/review_replies/review_media).
	reviewID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO reviews (id, tenant_id, store_id, product_id, customer_name, customer_email, rating, content)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		reviewID, tenantID, storeID, productID, "Test Customer", "reviewer-"+reviewID.String()[:8]+"@example.com", 5, "Great product",
	).Error, "seed reviews")

	require.NoError(t, db.Exec(
		`INSERT INTO review_reactions (id, review_id, customer_profile_id, reaction) VALUES (?, ?, ?, ?)`,
		uuid.New(), reviewID, customerProfileID, "helpful",
	).Error, "seed review_reactions")
	require.NoError(t, db.Exec(
		`INSERT INTO review_replies (id, review_id, author_type, author_name, content) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), reviewID, "merchant", "Store Owner", "Thanks!",
	).Error, "seed review_replies")
	require.NoError(t, db.Exec(
		`INSERT INTO review_media (id, review_id, url) VALUES (?, ?, ?)`,
		uuid.New(), reviewID, "https://example.com/photo.jpg",
	).Error, "seed review_media")

	// customer_loyalties (parent of loyalty_transactions).
	loyaltyID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO customer_loyalties (id, tenant_id, store_id, customer_email, referral_code)
		 VALUES (?, ?, ?, ?, ?)`,
		loyaltyID, tenantID, storeID, "loyal-"+loyaltyID.String()[:8]+"@example.com", loyaltyID.String()[:8],
	).Error, "seed customer_loyalties")
	require.NoError(t, db.Exec(
		`INSERT INTO loyalty_transactions (id, tenant_id, loyalty_id, type, points, balance_after)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, loyaltyID, "earn", 10, 10,
	).Error, "seed loyalty_transactions")

	// campaigns (parent of campaign_recipients).
	campaignID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO campaigns (id, tenant_id, store_id, name) VALUES (?, ?, ?, ?)`,
		campaignID, tenantID, storeID, "Test Campaign",
	).Error, "seed campaigns")
	require.NoError(t, db.Exec(
		`INSERT INTO campaign_recipients (id, tenant_id, campaign_id, customer_email) VALUES (?, ?, ?, ?)`,
		uuid.New(), tenantID, campaignID, "recipient-"+campaignID.String()[:8]+"@example.com",
	).Error, "seed campaign_recipients")

	// gift_cards (parent of gift_card_transactions).
	giftCardID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO gift_cards (id, tenant_id, store_id, code, initial_balance, current_balance, currency_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		giftCardID, tenantID, storeID, "GC-"+giftCardID.String()[:8], 100, 100, "EUR",
	).Error, "seed gift_cards")
	require.NoError(t, db.Exec(
		`INSERT INTO gift_card_transactions (id, tenant_id, gift_card_id, type, amount, balance_after)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, giftCardID, "redeem", 10, 90,
	).Error, "seed gift_card_transactions")

	// orders (needed for coupon_usage.order_id NOT NULL FK — no cascade, so
	// coupon_usage must be swept before orders; see the ordering note in
	// Sweep's comment).
	orderID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email, subtotal, grand_total, currency_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orderID, tenantID, storeID, "ORD-"+orderID.String()[:8], "idem-"+orderID.String(), "buyer-"+orderID.String()[:8]+"@example.com", 10, 10, "EUR",
	).Error, "seed orders")

	// coupons (parent of coupon_usage).
	couponID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO coupons (id, tenant_id, store_id, code, title, type, value) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		couponID, tenantID, storeID, "CODE-"+couponID.String()[:8], "Test Coupon", "percentage", 10,
	).Error, "seed coupons")
	require.NoError(t, db.Exec(
		`INSERT INTO coupon_usage (id, tenant_id, coupon_id, order_id, customer_email, discount_amount, currency_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, couponID, orderID, "buyer-"+couponID.String()[:8]+"@example.com", 1, "EUR",
	).Error, "seed coupon_usage")

	// tickets (parent of ticket_replies).
	ticketID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO tickets (id, tenant_id, store_id, ticket_number, subject, description, submitted_by_name, submitted_by_email)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, tenantID, storeID, "TKT-"+ticketID.String()[:8], "Help", "I need help", "Test Customer", "ticket-"+ticketID.String()[:8]+"@example.com",
	).Error, "seed tickets")
	require.NoError(t, db.Exec(
		`INSERT INTO ticket_replies (id, ticket_id, author_type, author_name, content) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), ticketID, "customer", "Test Customer", "Still broken",
	).Error, "seed ticket_replies")

	return nineTableFixture{
		reviewID:   reviewID,
		loyaltyID:  loyaltyID,
		campaignID: campaignID,
		giftCardID: giftCardID,
		couponID:   couponID,
		ticketID:   ticketID,
		productID:  productID,
		categoryID: categoryID,
		orderID:    orderID,
	}
}

// countRows returns the number of rows in table matching whereCol = id.
func countRows(t *testing.T, db *gorm.DB, table, whereCol string, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM "+table+" WHERE "+whereCol+" = ?", id, //nolint:gosec // table/col are test constants
	).Scan(&n).Error)
	return n
}

// assertNineTablesEmpty asserts every store_id-less table from issue #360
// has zero rows for the given fixture's parent ids.
func assertNineTablesEmpty(t *testing.T, db *gorm.DB, f nineTableFixture) {
	t.Helper()
	require.Zero(t, countRows(t, db, "review_reactions", "review_id", f.reviewID), "review_reactions")
	require.Zero(t, countRows(t, db, "review_replies", "review_id", f.reviewID), "review_replies")
	require.Zero(t, countRows(t, db, "review_media", "review_id", f.reviewID), "review_media")
	require.Zero(t, countRows(t, db, "loyalty_transactions", "loyalty_id", f.loyaltyID), "loyalty_transactions")
	require.Zero(t, countRows(t, db, "campaign_recipients", "campaign_id", f.campaignID), "campaign_recipients")
	require.Zero(t, countRows(t, db, "gift_card_transactions", "gift_card_id", f.giftCardID), "gift_card_transactions")
	require.Zero(t, countRows(t, db, "coupon_usage", "coupon_id", f.couponID), "coupon_usage")
	require.Zero(t, countRows(t, db, "ticket_replies", "ticket_id", f.ticketID), "ticket_replies")
	require.Zero(t, countRows(t, db, "product_categories", "product_id", f.productID), "product_categories")
}

func newEmitter(db *gorm.DB) *audit.Emitter {
	return audit.NewEmitter(audit.EmitterConfig{
		DB:     db,
		Repo:   audit.NewRepository(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// TestSweep_NineOrphanTables_DeletesViaParent is the test that fails on
// unfixed main: Sweep hits `column "store_id" does not exist`
// (SQLSTATE 42703) on the first store_id-less table it reaches and returns
// an error before deleting anything. On the fix, Sweep returns nil and
// every row — including the nine tables that have no store_id column — is
// gone.
func TestSweep_NineOrphanTables_DeletesViaParent(t *testing.T) {
	db := testdb.NewDB(t, sweptTables...)

	tenantID := uuid.New()
	storeID := uuid.New()
	fixture := seedNineTableFixture(t, db, tenantID, storeID)

	emitter := newEmitter(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tx := db.Begin()
	require.NoError(t, tx.Error)

	err := harddelete.Sweep(context.Background(), tx, emitter, logger, storeID, tenantID)
	require.NoError(t, err, "Sweep must succeed now that store_id-less tables are reached via their parent")
	require.NoError(t, tx.Commit().Error)

	assertNineTablesEmpty(t, db, fixture)

	// The store row itself (last in the sweep list) must also be gone.
	require.Zero(t, countRows(t, db, "stores", "id", storeID), "stores")

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	emitter.Stop(stopCtx)
}

// TestSweep_CrossStoreIsolation is the important test: a subquery on the
// parent table that forgets `WHERE store_id = ?` (e.g.
// "DELETE FROM review_reactions WHERE review_id IN (SELECT id FROM
// reviews)") would delete every store's rows through that parent, not just
// the swept one. This activates a destructive cron that has never
// successfully completed a run, so cross-store leakage here is the
// highest-severity failure mode this fix can introduce.
func TestSweep_CrossStoreIsolation(t *testing.T) {
	db := testdb.NewDB(t, sweptTables...)

	sweptTenantID := uuid.New()
	sweptStoreID := uuid.New()
	sweptFixture := seedNineTableFixture(t, db, sweptTenantID, sweptStoreID)

	survivorTenantID := uuid.New()
	survivorStoreID := uuid.New()
	survivorFixture := seedNineTableFixture(t, db, survivorTenantID, survivorStoreID)

	emitter := newEmitter(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tx := db.Begin()
	require.NoError(t, tx.Error)

	err := harddelete.Sweep(context.Background(), tx, emitter, logger, sweptStoreID, sweptTenantID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit().Error)

	// Swept store: every one of the nine tables is empty.
	assertNineTablesEmpty(t, db, sweptFixture)
	require.Zero(t, countRows(t, db, "stores", "id", sweptStoreID), "swept store row")

	// Survivor store: every one of the nine tables still has its row.
	require.EqualValues(t, 1, countRows(t, db, "review_reactions", "review_id", survivorFixture.reviewID), "survivor review_reactions")
	require.EqualValues(t, 1, countRows(t, db, "review_replies", "review_id", survivorFixture.reviewID), "survivor review_replies")
	require.EqualValues(t, 1, countRows(t, db, "review_media", "review_id", survivorFixture.reviewID), "survivor review_media")
	require.EqualValues(t, 1, countRows(t, db, "loyalty_transactions", "loyalty_id", survivorFixture.loyaltyID), "survivor loyalty_transactions")
	require.EqualValues(t, 1, countRows(t, db, "campaign_recipients", "campaign_id", survivorFixture.campaignID), "survivor campaign_recipients")
	require.EqualValues(t, 1, countRows(t, db, "gift_card_transactions", "gift_card_id", survivorFixture.giftCardID), "survivor gift_card_transactions")
	require.EqualValues(t, 1, countRows(t, db, "coupon_usage", "coupon_id", survivorFixture.couponID), "survivor coupon_usage")
	require.EqualValues(t, 1, countRows(t, db, "ticket_replies", "ticket_id", survivorFixture.ticketID), "survivor ticket_replies")
	require.EqualValues(t, 1, countRows(t, db, "product_categories", "product_id", survivorFixture.productID), "survivor product_categories")
	require.EqualValues(t, 1, countRows(t, db, "stores", "id", survivorStoreID), "survivor store row")

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	emitter.Stop(stopCtx)
}

// TestSweep_EmitsAuditEventPerTable pins the compliance trail the design
// explicitly chose to preserve: sweepTable emits a
// "subscription.hard_delete_sweep" audit event per table, including the
// nine reached via their parent. Dropping those nine from the sweep list
// (and relying on cascade instead) was considered and rejected because it
// would leave those tables with no deletion evidence in a GDPR path — this
// test is what would catch that regression.
func TestSweep_EmitsAuditEventPerTable(t *testing.T) {
	db := testdb.NewDB(t, append(sweptTables, "audit_logs")...)

	tenantID := uuid.New()
	storeID := uuid.New()
	seedNineTableFixture(t, db, tenantID, storeID)

	emitter := newEmitter(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tx := db.Begin()
	require.NoError(t, tx.Error)

	err := harddelete.Sweep(context.Background(), tx, emitter, logger, storeID, tenantID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit().Error)

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	emitter.Stop(stopCtx)

	for _, table := range []string{
		"review_reactions", "review_replies", "review_media",
		"loyalty_transactions", "campaign_recipients",
		"gift_card_transactions", "coupon_usage", "ticket_replies",
		"product_categories",
	} {
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM audit_logs
			 WHERE action = 'subscription.hard_delete_sweep' AND resource_type = ? AND store_id = ?`,
			table, storeID,
		).Scan(&n).Error)
		require.EqualValues(t, 1, n, "expected one hard_delete_sweep audit event for table %s", table)
	}
}
