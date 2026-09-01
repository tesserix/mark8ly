//go:build integration

// #177 PR 6 — the sentinel backfill (migration 000123).
//
// Tested by executing the migration's own SQL inside a rolled-back
// transaction, rather than by asserting on a pre-migrated fixture: the
// thing that can be wrong here is the SQL, and a test that re-implements
// it would agree with itself.
//
// The case that drove these tests is the one the spec gets wrong. It says
// "production has zero warehouses", so the backfill may simply CREATE a
// 'Main Warehouse' per store. the-bondi-store has had one — with a real
// address — since 2026-09-01, and warehouses is keyed (store_id, name).
package marketplaceapi_test

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

const sentinel = "00000000-0000-0000-0000-000000000001"

// runBackfill executes migrations/000123 statement by statement.
func runBackfill(t *testing.T, tx *gorm.DB) {
	t.Helper()
	raw, err := os.ReadFile("migrations/000123_backfill_sentinel_stock.up.sql")
	require.NoError(t, err)

	// Strip comments BEFORE splitting. A prose semicolon inside a comment
	// ("safe in either order; the reverse is not") otherwise splits a
	// statement in half, and the fragment fails with a syntax error that
	// looks nothing like the real cause.
	body := make([]string, 0)
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "--") {
			continue
		}
		body = append(body, l)
	}

	for _, stmt := range strings.Split(strings.Join(body, "\n"), ";") {
		cleaned := strings.TrimSpace(stmt)
		if cleaned == "" {
			continue
		}
		require.NoError(t, tx.Exec(cleaned).Error, "statement: %s", cleaned)
	}
}

// seedStoreWithSentinelStock creates a store, a product, one variant and
// qty units on the sentinel — the exact shape of every product in
// production today.
func seedStoreWithSentinelStock(t *testing.T, tx *gorm.DB, qty int) (storeID, tenantID, variantID string) {
	t.Helper()
	tenantID, storeID = uuid.NewString(), uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code,
		                     timezone, storefront_customer_portal_secret)
		 VALUES (?, ?, 'Backfill Store', ?, 'active', 'AU', 'AUD', 'Australia/Sydney', ?)`,
		storeID, tenantID, "bf-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Backfill Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "bf-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID = uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'AUD')`,
		variantID, productID, storeID, "BF-"+uuid.NewString()[:8]).Error)
	require.NoError(t, tx.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, sentinel, qty).Error)
	return storeID, tenantID, variantID
}

func seedNamedWarehouse(t *testing.T, tx *gorm.DB, tenantID, storeID, name, line1 string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone, is_default)
		 VALUES (?, ?, ?, ?, ?, 'Bondi Beach', 'NSW', '2026', 'AU', '+61255500000', true)`,
		id, tenantID, storeID, name, line1).Error)
	return id
}

func stockAt(t *testing.T, tx *gorm.DB, variantID, locationID string) int {
	t.Helper()
	var q int
	require.NoError(t, tx.Raw(
		`SELECT COALESCE((SELECT quantity FROM variant_stock
		                   WHERE variant_id = ? AND location_id = ?), -1)`,
		variantID, locationID).Scan(&q).Error)
	return q
}

func liveWarehouses(t *testing.T, tx *gorm.DB, storeID string) []string {
	t.Helper()
	var names []string
	require.NoError(t, tx.Raw(
		`SELECT name FROM warehouses WHERE store_id = ? AND archived_at IS NULL ORDER BY name`,
		storeID).Scan(&names).Error)
	return names
}

// A store with no warehouse gets one, and its stock moves onto it.
func TestBackfill_StoreWithNoWarehouseGetsOneAndItsStockMoves(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, _, variantID := seedStoreWithSentinelStock(t, tx, 16)

	runBackfill(t, tx)

	require.Equal(t, []string{"Main Warehouse"}, liveWarehouses(t, tx, storeID))
	require.Equal(t, -1, stockAt(t, tx, variantID, sentinel), "the sentinel row must be gone")

	var moved int
	require.NoError(t, tx.Raw(
		`SELECT vs.quantity FROM variant_stock vs
		   JOIN warehouses w ON w.id = vs.location_id
		  WHERE vs.variant_id = ? AND w.store_id = ?`, variantID, storeID).Scan(&moved).Error)
	require.Equal(t, 16, moved)
}

// THE test #515 asked for. bondi already has a 'Main Warehouse' with a real
// address. A naive INSERT violates (store_id, name); an ON CONFLICT DO
// NOTHING leaves the stock behind while the code believes it migrated.
func TestBackfill_ExistingMainWarehouseIsReusedNotDuplicated(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 16354)
	existing := seedNamedWarehouse(t, tx, tenantID, storeID, "Main Warehouse", "1 Campbell Parade")

	runBackfill(t, tx)

	require.Equal(t, []string{"Main Warehouse"}, liveWarehouses(t, tx, storeID),
		"a second 'Main Warehouse' must not be created")
	require.Equal(t, 16354, stockAt(t, tx, variantID, existing),
		"the stock must land on the warehouse that already existed")
	require.Equal(t, -1, stockAt(t, tx, variantID, sentinel))

	// The merchant's address must survive untouched — the backfill's blank
	// address is for stores that never had one.
	var line1 string
	require.NoError(t, tx.Raw(`SELECT line1 FROM warehouses WHERE id = ?`, existing).Scan(&line1).Error)
	require.Equal(t, "1 Campbell Parade", line1)
}

// A store whose warehouse is named something else must use THAT one, not
// gain a second called 'Main Warehouse' beside it.
func TestBackfill_DifferentlyNamedWarehouseIsUsedAsIs(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 40)
	existing := seedNamedWarehouse(t, tx, tenantID, storeID, "Bondi Depot", "1 Campbell Parade")

	runBackfill(t, tx)

	require.Equal(t, []string{"Bondi Depot"}, liveWarehouses(t, tx, storeID))
	require.Equal(t, 40, stockAt(t, tx, variantID, existing))
}

// A variant can carry BOTH a sentinel row and a real row for the same
// warehouse — 5e clears the sentinel, but the multi-variant product save
// still writes it. Availability adds them today, so the backfill must sum:
// overwriting would silently delete stock the merchant is selling.
func TestBackfill_SentinelAndRealRowAreSummedNotOverwritten(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 10)
	existing := seedNamedWarehouse(t, tx, tenantID, storeID, "Main Warehouse", "1 Campbell Parade")
	require.NoError(t, tx.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, existing).Error)

	runBackfill(t, tx)

	require.Equal(t, 15, stockAt(t, tx, variantID, existing),
		"10 on the sentinel + 5 already there = 15, the total availability reports today")
	require.Equal(t, -1, stockAt(t, tx, variantID, sentinel))
	_ = storeID
}

// A live cart hold on the sentinel must MOVE, not vanish: deleting it
// releases a shopper's reservation mid-checkout and lets someone else take
// the units they are paying for.
func TestBackfill_LiveHoldsMoveWithTheStock(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 10)
	existing := seedNamedWarehouse(t, tx, tenantID, storeID, "Main Warehouse", "1 Campbell Parade")
	cart := uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, 3, now() + interval '10 minutes', 'held')`,
		variantID, sentinel, cart).Error)

	runBackfill(t, tx)

	var qty int
	require.NoError(t, tx.Raw(
		`SELECT COALESCE((SELECT qty FROM stock_holds
		                   WHERE cart_token = ? AND variant_id = ? AND location_id = ?), -1)`,
		cart, variantID, existing).Scan(&qty).Error)
	require.Equal(t, 3, qty, "the hold must have moved to the real warehouse")

	var onSentinel int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM stock_holds WHERE variant_id = ? AND location_id = ?`,
		variantID, sentinel).Scan(&onSentinel).Error)
	require.Zero(t, onSentinel)
}

// Migrations get re-run in recovery and in fresh environments. A second
// pass must be a no-op, not a doubling.
func TestBackfill_IsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 16)
	existing := seedNamedWarehouse(t, tx, tenantID, storeID, "Main Warehouse", "1 Campbell Parade")

	runBackfill(t, tx)
	runBackfill(t, tx)

	require.Equal(t, 16, stockAt(t, tx, variantID, existing),
		"a second run must not double the stock")
	require.Equal(t, []string{"Main Warehouse"}, liveWarehouses(t, tx, storeID))
}

// An ARCHIVED warehouse is removed as far as the merchant is concerned, and
// the allocator refuses to fill from it (#528). Backfilling onto one would
// park every unit somewhere unsellable.
func TestBackfill_ArchivedWarehouseIsNotATarget(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 10)
	archived := seedNamedWarehouse(t, tx, tenantID, storeID, "Gone", "1 Old Road")
	require.NoError(t, tx.Exec(
		`UPDATE warehouses SET archived_at = now(), is_default = false WHERE id = ?`, archived).Error)

	runBackfill(t, tx)

	require.Equal(t, []string{"Main Warehouse"}, liveWarehouses(t, tx, storeID),
		"a store whose only warehouse is archived must get a live one")
	require.Equal(t, -1, stockAt(t, tx, variantID, archived),
		"no stock may be parked on an archived warehouse")
	require.Equal(t, -1, stockAt(t, tx, variantID, sentinel))
}

// The case above cannot catch a missing archived_at filter on its own: the
// warehouse the backfill creates is is_default = true, so it wins the
// ordering regardless. The filter only earns its keep when an archived row
// would otherwise SORT FIRST — an older one, at a store that already has a
// live warehouse so nothing new is created. A mutation proved the gap.
func TestBackfill_OlderArchivedWarehouseDoesNotWinOverALiveOne(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, variantID := seedStoreWithSentinelStock(t, tx, 25)

	archived := seedNamedWarehouse(t, tx, tenantID, storeID, "Old Depot", "1 Old Road")
	live := seedNamedWarehouse(t, tx, tenantID, storeID, "Current Depot", "2 New Road")
	// Neither is the default, so the tie-break falls to created_at — and the
	// archived one is older.
	require.NoError(t, tx.Exec(
		`UPDATE warehouses SET is_default = false WHERE store_id = ?`, storeID).Error)
	require.NoError(t, tx.Exec(
		`UPDATE warehouses SET archived_at = now(), created_at = now() - interval '30 days'
		  WHERE id = ?`, archived).Error)

	runBackfill(t, tx)

	require.Equal(t, 25, stockAt(t, tx, variantID, live),
		"the stock must land on the LIVE warehouse")
	require.Equal(t, -1, stockAt(t, tx, variantID, archived),
		"an archived warehouse must never be the backfill target, however it sorts")
}

// A store with no sentinel stock must not gain a warehouse it never asked
// for — the backfill is a migration, not a provisioning step.
func TestBackfill_StoreWithoutSentinelStockIsUntouched(t *testing.T) {
	tx := testdb.NewTx(t)
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code,
		                     timezone, storefront_customer_portal_secret)
		 VALUES (?, ?, 'Quiet Store', ?, 'active', 'AU', 'AUD', 'Australia/Sydney', ?)`,
		storeID, tenantID, "q-"+uuid.NewString()[:8], uuid.NewString()).Error)

	runBackfill(t, tx)

	require.Empty(t, liveWarehouses(t, tx, storeID))
}
