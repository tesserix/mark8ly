package testdb

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
)

// SeedStore inserts a minimal stores row for (tenantID, storeID), satisfying
// every NOT NULL column the schema requires — including
// storefront_customer_portal_secret, which a later migration added and most
// fixtures never caught up with.
//
// tenantID is supplied by the caller rather than generated here. Callers
// already mint a tenant for the rows they seed, and the store must carry that
// same tenant: otherwise the store and whatever references it disagree about
// tenancy, and every tenant-scoped assertion passes while testing nothing.
//
// Registers a t.Cleanup deleting the row AND dropping the two per-store
// sequences the insert created, so packages using NewDB (which truncates
// only the tables it was told about) still start clean.
//
// The sequences are not an incidental detail. Migration 000004's
// stores_after_insert_create_sequences trigger creates mk_seq_order_<id>
// and mk_seq_return_<id> for every row inserted into stores, and a sequence
// is a catalog relation, not a row: neither the DELETE below nor NewDB's
// TRUNCATE ... CASCADE reclaims it. Without an explicit DROP every NewDB
// run left two sequences behind forever, which is how the shared dev
// database reached ~28,000 orphaned sequences against zero stores (#436).
//
// Order: sequences first, store row second. The trigger fires on INSERT
// only, so there is no ON DELETE counterpart that would recreate them, and
// dropping first means a failure to delete the store (see below) still
// reclaims the catalog objects — the direction that leaks.
//
// The cleanup is best-effort and never fails the test: under NewTx it's a
// no-op, since both the row and the DDL are rolled back before this runs.
// Under NewDB the DELETE can fail — products.store_id and categories.store_id
// are ON DELETE RESTRICT, so if the caller's package left rows referencing
// this store, the DELETE errors out (logged, not fatal) unless the package
// also truncates stores itself.
func SeedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	// Derived from the store id to dodge stores_slug_unique when one test
	// seeds several stores.
	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("testdb.SeedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		// A sequence name is an identifier, not a bind value, so DROP
		// SEQUENCE cannot be parameterized. order.SequenceName derives the
		// name from a uuid and a literal kind, so the character set is
		// [a-z0-9_] only and interpolation is safe.
		for _, kind := range []string{"order", "return"} {
			seq := order.SequenceName(storeID, kind)
			if err := db.Exec("DROP SEQUENCE IF EXISTS " + seq).Error; err != nil {
				t.Logf("testdb.SeedStore: cleanup DROP SEQUENCE %s: %v", seq, err)
			}
		}
		if err := db.Exec("DELETE FROM stores WHERE id = ?", storeID).Error; err != nil {
			t.Logf("testdb.SeedStore: cleanup DELETE stores row %s: %v", storeID, err)
		}
	})
}

// SeedVendor inserts a vendors row owned by tenantID and returns its id, for
// use as products.vendor_id — NOT NULL since migration 000028.
//
// A real row is inserted (not a synthetic UUID) even though there is no FK
// from products.vendor_id to vendors(id) — migration 000027 creates the
// vendors table and 000028 only adds a NOT NULL constraint on vendor_id, no
// foreign key — so the database will not catch a bogus id. The real row
// exists purely so vendor-scoped filtering and any products→vendors join
// stay meaningful for assertions; a synthetic id would make those assertions
// vacuous in exactly the way this plan is trying to stop.
//
// is_self is true because product.Create defaults a new product to the
// tenant's self-vendor, so that is the vendor a fixture should stand in for.
//
// SeedVendor is idempotent per tenant. The vendors_tenant_self_idx unique
// constraint (UNIQUE (tenant_id) WHERE (is_self = true)) enforces that only
// one self-vendor can exist per tenant. This mirrors production's
// EnsureSelfVendor (migration 000028), which creates it once per tenant
// lifecycle. Repeat calls for the same tenantID return the existing self-vendor
// without inserting or registering cleanup, so multiple tests can safely use
// the same tenant.
func SeedVendor(t *testing.T, db *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()

	// Check if a self-vendor already exists for this tenant.
	var existing struct {
		ID uuid.UUID `db:"id"`
	}
	result := db.Raw(
		`SELECT id FROM vendors WHERE tenant_id = ? AND is_self = true`,
		tenantID,
	).Scan(&existing)
	if result.Error != nil {
		// Query failed; this is a real error.
		t.Fatalf("testdb.SeedVendor: check for existing self-vendor: %v", result.Error)
	}
	if result.RowsAffected > 0 {
		// Row found; return it without cleanup.
		return existing.ID
	}

	// No existing self-vendor; insert one.
	vendorID := uuid.New()
	slug := "vnd-" + strings.ReplaceAll(vendorID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO vendors (id, tenant_id, name, slug, status, is_self)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		vendorID, tenantID, "Test Vendor", slug, "active", true,
	).Error
	if err != nil {
		t.Fatalf("testdb.SeedVendor: insert vendors row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM vendors WHERE id = ?", vendorID)
	})

	return vendorID
}
