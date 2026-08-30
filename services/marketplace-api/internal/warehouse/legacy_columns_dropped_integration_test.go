//go:build integration

// Package warehouse_test — coverage for migration 000117, #484's contract half.
//
// 000117 drops the 8 legacy warehouse_* columns from
// shipping_carrier_configs, closing the expand/contract migration 000095
// opened. What these tests guard is the shape of the schema afterwards,
// against a database that has every migration applied:
//
//   - the 8 columns are actually gone (the drop ran, and nothing re-adds
//     them)
//   - warehouse_id and its index are NOT gone — they are the whole point of
//     the migration, and a fat-fingered column list in the DROP would take
//     out the read path with no compile error anywhere
//
// This file replaces backfill_stragglers_integration_test.go, which covered
// 000116 by seeding rows into the legacy columns. That coverage cannot
// outlive the columns: once 000117 has run, the fixture it needs is
// unwritable. 000116 itself still runs, ahead of 000117, on any fresh
// replay of the migration history.
package warehouse_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// legacyWarehouseColumns is the exact set 000008 created and 000117 drops.
var legacyWarehouseColumns = []string{
	"warehouse_name",
	"warehouse_line1",
	"warehouse_line2",
	"warehouse_city",
	"warehouse_region",
	"warehouse_postal",
	"warehouse_country",
	"warehouse_phone",
}

func TestMigration000117_LegacyWarehouseColumnsAreDropped(t *testing.T) {
	db := testdb.NewTx(t)

	for _, col := range legacyWarehouseColumns {
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM information_schema.columns
			  WHERE table_name = 'shipping_carrier_configs' AND column_name = ?`, col,
		).Scan(&n).Error)
		require.Zerof(t, n, "shipping_carrier_configs.%s must be dropped by migration 000117", col)
	}
}

// TestMigration000117_WarehouseIDSurvives is the mutation guard on the test
// above: an assertion that only checks columns are absent would still pass
// if the DROP had taken warehouse_id with them, which would silently sever
// every reader from its address.
func TestMigration000117_WarehouseIDSurvives(t *testing.T) {
	db := testdb.NewTx(t)

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name = 'shipping_carrier_configs' AND column_name = 'warehouse_id'`,
	).Scan(&n).Error)
	require.EqualValues(t, 1, n, "warehouse_id is the replacement for the dropped columns — it must still exist")

	require.NoError(t, db.Raw(
		`SELECT count(*) FROM pg_indexes
		  WHERE tablename = 'shipping_carrier_configs'
		    AND indexname = 'shipping_carrier_configs_warehouse_idx'`,
	).Scan(&n).Error)
	require.EqualValues(t, 1, n, "the warehouse_id index from 000095 must survive the drop")
}

// TestMigration000117_ConfigStillResolvesItsWarehouse proves the surviving
// column is usable, not merely present: a config written with a
// warehouse_id still joins to the address every reader now goes through.
func TestMigration000117_ConfigStillResolvesItsWarehouse(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := seedStore(t, db)

	warehouseID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone)
		 VALUES (?, ?, ?, 'Main Warehouse', '12 Industrial Estate', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		warehouseID, tenantID, storeID).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active, warehouse_id)
		 VALUES (?, ?, ?, 'delhivery', 'key', 'test', true, ?)`,
		uuid.NewString(), tenantID, storeID, warehouseID).Error)

	var got struct {
		Name string
		City string
	}
	require.NoError(t, db.Raw(
		`SELECT w.name, w.city
		   FROM shipping_carrier_configs c
		   JOIN warehouses w ON w.id = c.warehouse_id
		  WHERE c.store_id = ? AND c.provider = 'delhivery'`, storeID,
	).Scan(&got).Error)
	require.Equal(t, "Main Warehouse", got.Name)
	require.Equal(t, "Mumbai", got.City)
}
