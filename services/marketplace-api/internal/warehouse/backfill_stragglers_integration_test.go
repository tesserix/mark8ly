//go:build integration

// Package warehouse_test — coverage for migration 000116, #484's backfill.
//
// #480's write path only sets shipping_carrier_configs.warehouse_id on a
// save through the admin settings handler. A row created (or last edited)
// after migration 000095's one-time backfill ran, but before #480 shipped,
// has its legacy warehouse_* columns populated and warehouse_id still NULL
// — exactly the gap #484's issue names as the precondition for dropping
// those columns. Migration 000116 closes it by re-running 000095's own
// backfill shape, scoped to today's stragglers.
//
// This is the single most important test in the PR: it is what makes
// dropping the legacy columns in a later migration safe, by proving no row
// is left depending on them once this migration has run.
package warehouse_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// runBackfillMigration executes migration 000116's up.sql verbatim, read
// from the same embedded FS the production migrate tool uses — so this
// test exercises the actual migration file, not a hand-copied
// approximation of it that could drift from what actually ships.
func runBackfillMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	sql, err := marketplaceapi.MigrationsFS.ReadFile("migrations/000116_backfill_warehouse_id_stragglers.up.sql")
	require.NoError(t, err, "read migration 000116 from the embedded FS")
	require.NoError(t, db.Exec(string(sql)).Error, "run migration 000116")
}

// seedStraggler inserts a shipping_carrier_configs row shaped exactly like
// the gap this migration closes: legacy warehouse_* columns populated
// (written by #480's expand-half write path or an even older writer),
// warehouse_id still NULL (never touched by a save through the write path
// that maintains it).
func seedStraggler(t *testing.T, db *gorm.DB, tenantID, storeID, name string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active,
		     warehouse_name, warehouse_line1, warehouse_line2, warehouse_city,
		     warehouse_region, warehouse_postal, warehouse_country, warehouse_phone)
		 VALUES (?, ?, ?, 'delhivery', 'legacy-key', 'test', true,
		         ?, '12 Industrial Estate', '', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		uuid.NewString(), tenantID, storeID, name).Error)
}

// TestBackfillMigration_LinksAStragglerRowToANewWarehouse is the core
// assertion: a config with populated legacy columns and NULL warehouse_id
// must, after the migration runs, have a matching warehouses row AND
// warehouse_id pointing at it.
func TestBackfillMigration_LinksAStragglerRowToANewWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "shipping_carrier_configs", "warehouses", "stores")
	tenantID, storeID := seedStore(t, db)
	seedStraggler(t, db, tenantID, storeID, "Main Warehouse")

	runBackfillMigration(t, db)

	var whID, whLine1, whCity string
	require.NoError(t, db.Raw(
		`SELECT id, line1, city FROM warehouses WHERE store_id = ? AND name = 'Main Warehouse'`, storeID).
		Row().Scan(&whID, &whLine1, &whCity))
	require.Equal(t, "12 Industrial Estate", whLine1)
	require.Equal(t, "Mumbai", whCity)

	var linkedID string
	require.NoError(t, db.Raw(
		`SELECT warehouse_id::text FROM shipping_carrier_configs WHERE store_id = ? AND provider = 'delhivery'`,
		storeID).Row().Scan(&linkedID))
	require.Equal(t, whID, linkedID, "the straggler's warehouse_id must now point at the backfilled row")
}

// TestBackfillMigration_SkipsRowsWithABlankWarehouseName mirrors 000095's
// own rule: a blank name was never a usable warehouse (it's what Delhivery
// keys on), so a straggler with no name must be left alone — no warehouse
// row created, warehouse_id still NULL.
func TestBackfillMigration_SkipsRowsWithABlankWarehouseName(t *testing.T) {
	db := testdb.NewDB(t, "shipping_carrier_configs", "warehouses", "stores")
	tenantID, storeID := seedStore(t, db)
	seedStraggler(t, db, tenantID, storeID, "")

	runBackfillMigration(t, db)

	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM warehouses WHERE store_id = ?`, storeID).Scan(&count).Error)
	require.Zero(t, count, "a blank warehouse_name must not create a warehouse row")

	var linkedID *string
	require.NoError(t, db.Raw(
		`SELECT warehouse_id::text FROM shipping_carrier_configs WHERE store_id = ? AND provider = 'delhivery'`,
		storeID).Row().Scan(&linkedID))
	require.Nil(t, linkedID)
}

// TestBackfillMigration_DoesNotTouchAnAlreadyLinkedRow covers a config that
// went through #480's write path normally (warehouse_id already set) — the
// migration's WHERE c.warehouse_id IS NULL guard must leave it untouched,
// not attempt to re-link or duplicate its warehouse.
func TestBackfillMigration_DoesNotTouchAnAlreadyLinkedRow(t *testing.T) {
	db := testdb.NewDB(t, "shipping_carrier_configs", "warehouses", "stores")
	tenantID, storeID := seedStore(t, db)

	whID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone, is_default)
		 VALUES (?, ?, ?, 'Main Warehouse', '12 Industrial Estate', 'Mumbai', 'MH', '400001', 'IN', '+912200000000', true)`,
		whID, tenantID, storeID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active,
		     warehouse_name, warehouse_line1, warehouse_city, warehouse_region,
		     warehouse_postal, warehouse_country, warehouse_phone, warehouse_id)
		 VALUES (?, ?, ?, 'delhivery', 'legacy-key', 'test', true,
		         'Main Warehouse', '12 Industrial Estate', 'Mumbai', 'MH', '400001', 'IN', '+912200000000', ?)`,
		uuid.NewString(), tenantID, storeID, whID).Error)

	runBackfillMigration(t, db)

	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM warehouses WHERE store_id = ?`, storeID).Scan(&count).Error)
	require.EqualValues(t, 1, count, "an already-linked row must not gain a duplicate warehouse")

	var linkedID string
	require.NoError(t, db.Raw(
		`SELECT warehouse_id::text FROM shipping_carrier_configs WHERE store_id = ? AND provider = 'delhivery'`,
		storeID).Row().Scan(&linkedID))
	require.Equal(t, whID, linkedID)
}

// TestBackfillMigration_IsIdempotent runs the migration twice and asserts
// the second run changes nothing: no new warehouses row, no new link. This
// is the same guarantee 000095's own backfill documents for itself, and it
// matters here because a migration tool can, in principle, be asked to
// re-apply an already-applied version.
func TestBackfillMigration_IsIdempotent(t *testing.T) {
	db := testdb.NewDB(t, "shipping_carrier_configs", "warehouses", "stores")
	tenantID, storeID := seedStore(t, db)
	seedStraggler(t, db, tenantID, storeID, "Main Warehouse")

	runBackfillMigration(t, db)

	var whID string
	require.NoError(t, db.Raw(`SELECT id::text FROM warehouses WHERE store_id = ?`, storeID).Row().Scan(&whID))

	runBackfillMigration(t, db)

	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM warehouses WHERE store_id = ?`, storeID).Scan(&count).Error)
	require.EqualValues(t, 1, count, "re-running the migration must not create a second warehouse row")

	var whIDAfter, linkedID string
	require.NoError(t, db.Raw(`SELECT id::text FROM warehouses WHERE store_id = ?`, storeID).Row().Scan(&whIDAfter))
	require.Equal(t, whID, whIDAfter, "the warehouse row's identity must be unchanged by a second run")

	require.NoError(t, db.Raw(
		`SELECT warehouse_id::text FROM shipping_carrier_configs WHERE store_id = ? AND provider = 'delhivery'`,
		storeID).Row().Scan(&linkedID))
	require.Equal(t, whID, linkedID)
}
