//go:build integration

package dunning_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedStore inserts a minimal stores row for (tenantID, storeID) so that a
// store_subscriptions row referencing storeID satisfies
// store_subscriptions_store_id_fkey.
//
// Unlike internal/order's seedStore helper, tenantID is supplied by the
// caller rather than generated here: dunning tests already mint their own
// tenantID for the subscription/audit rows they seed, and the store must
// carry that same tenant — otherwise the subscription and its store
// disagree about tenancy and any tenant-scoped assertion becomes
// meaningless.
//
// Registers a t.Cleanup that deletes the store row so consecutive test runs
// start from a clean state. No per-store sequences are dropped here (unlike
// internal/order's helper) — dunning doesn't touch order/return numbering.
func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	// Slug is derived from the store id to dodge the stores_slug_unique
	// constraint when multiple tests (or multiple stores within one test)
	// run in the same package.
	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}
