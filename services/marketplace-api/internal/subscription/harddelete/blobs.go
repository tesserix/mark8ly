// Package harddelete — blobs.go: the objects a 150-day hard delete's rows
// pointed at (#961).
//
// Same rule as tenant purge, scoped to one STORE rather than a tenant:
// collect the references inside the transaction before the sweep deletes
// the rows that name them, then destroy the objects after it commits, and
// only those internal/blobreap confirms we minted and nothing surviving
// still references.
//
// The reference check matters more here than almost anywhere. A hard
// delete removes ONE store while its tenant's other stores live on, and
// copy-to-store deliberately points their product_media rows at the same
// content-addressed object. Deleting on row-disappearance alone would
// blank images in a store the merchant still trades from.
package harddelete

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// blobSources enumerates columns holding a reference to an object this
// service minted, for one store.
//
// Deliberately absent, for the reasons spelled out in
// tenantpurge/blobs.go: customer_profiles.avatar_url (shopper-supplied,
// never ours), webhook_subscriptions.url (an HTTP endpoint), and
// order_items.image_url (a snapshot blobreap consults as a SURVIVING
// reference rather than a thing to delete).
//
// user_profiles is absent too, and for a reason specific to this sweep:
// staff accounts belong to the TENANT, not to the store being deleted.
// A merchant closing one of three stores keeps their avatar.
var blobSources = []struct {
	table string
	sql   string
}{
	{
		table: "product_media",
		sql: `SELECT m.url FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.store_id = ?
		      UNION
		      SELECT m.storage_key FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.store_id = ?
		      UNION
		      SELECT m.gcs_path_original FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.store_id = ?`,
	},
	{
		table: "product_personalisation_fields",
		sql: `SELECT mockup_storage_key FROM product_personalisation_fields
		      WHERE store_id = ? AND mockup_storage_key IS NOT NULL`,
	},
	{
		table: "review_media",
		sql: `SELECT m.url FROM review_media m
		       JOIN reviews r ON r.id = m.review_id
		      WHERE r.store_id = ?`,
	},
	{
		table: "categories",
		sql:   `SELECT image_url FROM categories WHERE store_id = ? AND image_url IS NOT NULL`,
	},
	{
		table: "store_branding",
		sql:   `SELECT logo_url FROM store_branding WHERE store_id = ? AND logo_url IS NOT NULL`,
	},
}

// collectBlobRefs reads every object reference the store's rows hold.
// Runs inside the hard-delete transaction, BEFORE Sweep.
//
// A failure fails the hard delete: proceeding would destroy the rows and
// lose the only record of which objects to destroy.
func collectBlobRefs(ctx context.Context, tx *gorm.DB, storeID uuid.UUID) ([]string, error) {
	var all []string
	for _, src := range blobSources {
		args := make([]any, strings.Count(src.sql, "?"))
		for i := range args {
			args[i] = storeID
		}
		var refs []string
		if err := tx.WithContext(ctx).Raw(src.sql, args...).Scan(&refs).Error; err != nil {
			return nil, fmt.Errorf("harddelete: collect object refs from %s: %w", src.table, err)
		}
		all = append(all, refs...)
	}
	return all, nil
}
