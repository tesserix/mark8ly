//go:build integration

// Package wishlist_test covers the raw-SQL read projection in
// wishlist.Repository.List — the only place in the service where a customer's
// saved products get a price range and an availability flag.
//
// These are the first tests the wishlist package has ever had. They exist
// because the two values that query computes are customer-visible promises:
// "from $X" is a price a shopper expects checkout to honour, and in_stock is a
// claim they can add the thing to a basket. Both were computed from data that
// included WITHDRAWN variants (#420), so both could lie in the direction that
// costs the merchant — advertising a price that has been retracted, and
// advertising availability that does not exist.
//
// The query is raw SQL, so GORM's soft-delete predicate from #395 does not
// reach it. That is precisely why it needs tests at this level: nothing in the
// type system or the ORM will notice if `AND deleted_at IS NULL` goes missing
// again.
package wishlist_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/wishlist"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// --------------- fixtures ---------------

// fixture is one customer with one wishlisted product, ready for variants.
type fixture struct {
	tenantID   uuid.UUID
	storeID    uuid.UUID
	productID  uuid.UUID
	customerID uuid.UUID
}

// seedWishlistedProduct builds the smallest graph the List query joins over:
// store -> vendor -> product -> (customer, wishlist entry). Variants are left
// to the caller, since which variants exist — and which are soft-deleted — is
// the entire subject of these tests.
//
// status is a products.status value. 'active' additionally requires
// published_at (products_published_requires_active), so it is set in lockstep
// rather than left to the caller to remember.
func seedWishlistedProduct(t *testing.T, db *gorm.DB, status string) fixture {
	t.Helper()

	f := fixture{
		tenantID:   uuid.New(),
		storeID:    uuid.New(),
		productID:  uuid.New(),
		customerID: uuid.New(),
	}

	testdb.SeedStore(t, db, f.tenantID, f.storeID)
	vendorID := testdb.SeedVendor(t, db, f.tenantID)

	publishedAt := "now()"
	if status != "active" {
		publishedAt = "NULL"
	}
	err := db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, status, published_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, `+publishedAt+`)`,
		f.productID, f.tenantID, f.storeID, vendorID,
		"wl-"+f.productID.String()[:8], "Wishlisted Product", status,
	).Error
	require.NoError(t, err, "seed products row")

	err = db.Exec(`
		INSERT INTO customer_profiles (id, tenant_id, store_id, email)
		VALUES (?, ?, ?, ?)`,
		f.customerID, f.tenantID, f.storeID, f.customerID.String()+"@example.test",
	).Error
	require.NoError(t, err, "seed customer_profiles row")

	err = db.Exec(`
		INSERT INTO wishlists (tenant_id, store_id, customer_id, product_id)
		VALUES (?, ?, ?, ?)`,
		f.tenantID, f.storeID, f.customerID, f.productID,
	).Error
	require.NoError(t, err, "seed wishlists row")

	return f
}

// seedVariant inserts one product_variants row.
//
// deleted models the SOFT delete the admin path performs
// (product/repository_variants.go): the row stays, deleted_at is stamped. That
// surviving row is the whole hazard — a hard DELETE here would make every one
// of these tests pass against the buggy query and prove nothing.
//
// The column is inventory_quantity, not stock_quantity. It is denormalised
// from variant_stock by trigger in production, but writing it directly is the
// established fixture shape in this repo and is what the List query reads.
func seedVariant(t *testing.T, db *gorm.DB, f fixture, sku, price string, qty int, deleted bool) uuid.UUID {
	t.Helper()

	id := uuid.New()
	deletedAt := "NULL"
	if deleted {
		deletedAt = "now()"
	}
	err := db.Exec(`
		INSERT INTO product_variants
			(id, product_id, store_id, sku, price, currency_code, inventory_quantity, deleted_at)
		VALUES (?, ?, ?, ?, ?, 'EUR', ?, `+deletedAt+`)`,
		id, f.productID, f.storeID, sku, price, qty,
	).Error
	require.NoError(t, err, "seed product_variants row (sku %s)", sku)
	return id
}

// listOne runs the projection under test and asserts the customer sees exactly
// their one wishlisted product.
func listOne(t *testing.T, db *gorm.DB, f fixture) wishlist.WishlistItem {
	t.Helper()

	items, total, err := wishlist.NewRepository(db).List(context.Background(), f.customerID.String(), 1, 20)
	require.NoError(t, err, "List must not error")
	require.EqualValues(t, 1, total, "the fixture wishlisted exactly one product")
	require.Len(t, items, 1, "the fixture wishlisted exactly one product")
	return items[0]
}

// --------------- price range ---------------

// The headline case for #420. A merchant lists a product at $10 and $30, then
// withdraws the $10 variant. The wishlist must stop advertising "from $10":
// that price is retracted and checkout will not honour it.
func TestIntegration_WishlistList_MinPrice_IgnoresSoftDeletedVariant(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-CHEAP", "10.00", 5, true) // withdrawn by the merchant
	seedVariant(t, db, f, "WL-DEAR", "30.00", 5, false) // the cheapest one still purchasable

	got := listOne(t, db, f)

	require.Equal(t, "30.00", got.ProductMinPrice,
		"the wishlist advertised a price the merchant has WITHDRAWN: it quoted the "+
			"soft-deleted $10 variant, so a shopper is shown 'from $10' for a product "+
			"whose cheapest purchasable variant is $30 and whose checkout will charge $30")
	require.Equal(t, "30.00", got.ProductMaxPrice,
		"max price must come from live variants only")
}

// The soft-deleted row must not win the top of the range either — the same
// defect in the other direction, where a withdrawn premium variant inflates
// the advertised ceiling.
func TestIntegration_WishlistList_MaxPrice_IgnoresSoftDeletedVariant(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-LIVE", "20.00", 5, false)
	seedVariant(t, db, f, "WL-GONE", "99.00", 5, true)

	got := listOne(t, db, f)

	require.Equal(t, "20.00", got.ProductMinPrice)
	require.Equal(t, "20.00", got.ProductMaxPrice,
		"a withdrawn $99 variant must not set the top of the advertised range")
}

// With every variant withdrawn there is no live price at all. The COALESCE
// fallback must take over: '0' is a visibly empty range the storefront can
// render, whereas quoting the withdrawn price would be the #420 bug at its
// worst — a product with nothing purchasable still showing a price tag.
func TestIntegration_WishlistList_Price_FallsBackToZeroWhenEveryVariantIsSoftDeleted(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-D1", "10.00", 5, true)
	seedVariant(t, db, f, "WL-D2", "30.00", 5, true)

	got := listOne(t, db, f)

	require.Equal(t, "0", got.ProductMinPrice,
		"no live variant means no live price; the COALESCE fallback must apply "+
			"rather than the query erroring or quoting a withdrawn amount")
	require.Equal(t, "0", got.ProductMaxPrice)
}

// --------------- in_stock ---------------

// The happy path, asserted first-class so the flag cannot be satisfied by
// simply always being false — which every other in_stock test here would
// otherwise accept.
func TestIntegration_WishlistList_InStock_TrueForActiveProductWithLiveStockedVariant(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-OK", "15.00", 3, false)

	got := listOne(t, db, f)

	require.True(t, got.InStock,
		"an active product with a live variant holding 3 units IS purchasable; "+
			"reporting it out of stock loses the merchant a sale")
}

// in_stock was a bare `p.status = 'active'`, so this product — active, but
// with nothing on the shelf — advertised itself as in stock, contradicting its
// own product page, which derives the flag from InventoryQuantity > 0
// (handlers/storefront/dto.go:139).
func TestIntegration_WishlistList_InStock_FalseWhenEveryLiveVariantHasZeroInventory(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-EMPTY-1", "15.00", 0, false)
	seedVariant(t, db, f, "WL-EMPTY-2", "25.00", 0, false)

	got := listOne(t, db, f)

	require.False(t, got.InStock,
		"every live variant holds zero units, so nothing can be bought; the "+
			"wishlist claimed in_stock anyway and the product's own page said the "+
			"opposite — two surfaces, one word, contradictory answers")
}

// A withdrawn variant's stock must not keep a product looking available: the
// units exist in the table but are not for sale.
func TestIntegration_WishlistList_InStock_FalseWhenTheOnlyStockedVariantIsSoftDeleted(t *testing.T) {
	db := testdb.NewTx(t)
	f := seedWishlistedProduct(t, db, "active")

	seedVariant(t, db, f, "WL-GONE-STOCK", "15.00", 50, true) // withdrawn, still holds 50
	seedVariant(t, db, f, "WL-LIVE-EMPTY", "15.00", 0, false)

	got := listOne(t, db, f)

	require.False(t, got.InStock,
		"the 50 units sit on a WITHDRAWN variant, so none of them are purchasable; "+
			"counting them keeps an unbuyable product advertised as available")
}

// The status half of the condition still has to hold: stock on the shelf does
// not make a draft or archived product purchasable.
func TestIntegration_WishlistList_InStock_FalseForInactiveProductDespiteStock(t *testing.T) {
	for _, status := range []string{"draft", "archived"} {
		t.Run(status, func(t *testing.T) {
			db := testdb.NewTx(t)
			f := seedWishlistedProduct(t, db, status)

			seedVariant(t, db, f, "WL-"+status, "15.00", 99, false)

			got := listOne(t, db, f)

			require.False(t, got.InStock,
				"a %s product is not on sale however much stock it holds", status)
		})
	}
}
