//go:build integration

package order_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedStore inserts a minimal stores row with the given id so that the
// AFTER INSERT trigger (see migration 000004_orders_seq_eager) fires and
// creates the per-store mk_seq_order_* and mk_seq_return_* sequences that
// NextDocumentNumber depends on.
//
// Registers a t.Cleanup that deletes the store row and drops the two
// sequences so consecutive test runs start from a clean state.
//
// Slug is derived from the uuid to dodge the stores_slug_unique constraint
// when multiple tests run in the same package.
func seedStore(t *testing.T, db *gorm.DB, storeID uuid.UUID) {
	t.Helper()

	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]
	tenantID := uuid.New()

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		underscored := strings.ReplaceAll(storeID.String(), "-", "_")
		db.Exec("DROP SEQUENCE IF EXISTS mk_seq_order_" + underscored)
		db.Exec("DROP SEQUENCE IF EXISTS mk_seq_return_" + underscored)
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}
