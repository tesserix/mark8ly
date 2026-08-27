package testdb

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
// Registers a t.Cleanup deleting the row, so packages using NewDB (which
// truncates only the tables it was told about) still start clean.
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
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// SeedVendor inserts a vendors row owned by tenantID and returns its id, for
// use as products.vendor_id — NOT NULL since migration 000028.
//
// The column has no foreign key, so any UUID would satisfy the constraint.
// A real row is inserted anyway so that vendor-scoped filtering and any
// products→vendors join stay meaningful; a synthetic id would make those
// assertions vacuous in exactly the way this plan is trying to stop.
//
// is_self is true because product.Create defaults a new product to the
// tenant's self-vendor, so that is the vendor a fixture should stand in for.
func SeedVendor(t *testing.T, db *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()

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
