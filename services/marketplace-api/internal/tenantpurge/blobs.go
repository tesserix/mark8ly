// Package tenantpurge — blobs.go: the objects a purge's rows pointed at (#961).
//
// The rows are deleted inside the purge transaction. The objects are
// destroyed only after it commits, and only once internal/blobreap has
// confirmed both that we minted them and that nothing surviving still
// references them — product media is content-addressed and shared across
// stores by copy-to-store, so a tenant's rows disappearing does not mean
// the object is unreferenced.
//
// The references must be COLLECTED inside the transaction, before the
// deletes run: afterwards the rows that named them are gone.
package tenantpurge

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// blobSources enumerates the columns that can hold a reference to an
// object this service minted, for a tenant and its stores.
//
// Enumerating liberally is safe: blobreap refuses anything outside our
// bucket and our key prefixes, so a column that turns out to hold
// something else contributes a skip rather than a deletion. Two columns
// are nonetheless left out ON PURPOSE, because relying on the guard to
// catch them would be relying on it for the wrong reason:
//
//   - customer_profiles.avatar_url is SHOPPER-SUPPLIED. Until #971 it
//     accepted any storage.googleapis.com URL, so legacy rows can name a
//     merchant's product image in our own bucket — which would pass the
//     provenance check. The reference guard would still save us, but the
//     honest answer is that these were never ours to delete.
//   - webhook_subscriptions.url is a merchant's HTTP endpoint. It is not
//     an object at all.
//
// order_items.image_url is also absent, for a different reason: it is a
// snapshot of a product_media object rather than an object of its own, and
// blobreap consults it as a SURVIVING reference instead. Listing it here
// would propose deleting an object that a live order still displays.
//
// user_profiles.avatar_url is absent for the reason purge.go already
// gives for excluding the table from the plan itself: "one row per GIP
// user, not owned by a single tenant". The account outlives the tenant,
// and so does the avatar. An earlier draft of this file selected it
// `WHERE tenant_id = ?` — a column user_profiles does not have, which
// would have failed collectBlobRefs and taken the whole purge down with
// it.
var blobSources = []struct {
	table string
	sql   string
}{
	{
		table: "product_media",
		sql: `SELECT m.url FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.tenant_id = ?
		      UNION
		      SELECT m.storage_key FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.tenant_id = ?
		      UNION
		      SELECT m.gcs_path_original FROM product_media m
		       JOIN products p ON p.id = m.product_id
		      WHERE p.tenant_id = ?`,
	},
	{
		table: "product_personalisation_fields",
		sql: `SELECT mockup_storage_key FROM product_personalisation_fields
		      WHERE tenant_id = ? AND mockup_storage_key IS NOT NULL`,
	},
	{
		table: "review_media",
		sql: `SELECT m.url FROM review_media m
		       JOIN reviews r ON r.id = m.review_id
		      WHERE r.tenant_id = ?`,
	},
	{
		table: "categories",
		sql:   `SELECT image_url FROM categories WHERE tenant_id = ? AND image_url IS NOT NULL`,
	},
	{
		table: "store_branding",
		sql: `SELECT b.logo_url FROM store_branding b
		       JOIN stores s ON s.id = b.store_id
		      WHERE s.tenant_id = ? AND b.logo_url IS NOT NULL`,
	},
	// Buyer artwork (#980). Lives in the PRIVATE bucket, so these
	// references are only reachable when the reaper has a target for it;
	// without one blobreap reports them as not-ours rather than deleting
	// them against the wrong bucket.
	//
	// Both columns: storage_key_original is the pristine upload the
	// merchant prints from, storage_key is the preview. Two objects, and
	// leaving the preview behind would leave a recognisable photograph of
	// a person behind.
	//
	// order_item_personalisations is reached through order_items because
	// it carries no tenant or store column of its own by design — the
	// snapshot has no FK back to the catalog so it survives the merchant
	// renaming or deleting the field it came from.
	{
		table: "personalisation_uploads",
		sql: `SELECT storage_key_original FROM personalisation_uploads
		      WHERE tenant_id = ?
		      UNION
		      SELECT storage_key FROM personalisation_uploads
		      WHERE tenant_id = ? AND storage_key IS NOT NULL`,
	},
	{
		table: "order_item_personalisations",
		sql: `SELECT p.storage_key_original FROM order_item_personalisations p
		       JOIN order_items i ON i.id = p.order_item_id
		       JOIN orders o ON o.id = i.order_id
		      WHERE o.tenant_id = ? AND p.storage_key_original IS NOT NULL
		      UNION
		      SELECT p.storage_key FROM order_item_personalisations p
		       JOIN order_items i ON i.id = p.order_item_id
		       JOIN orders o ON o.id = i.order_id
		      WHERE o.tenant_id = ? AND p.storage_key IS NOT NULL`,
	},
}

// collectBlobRefs reads every object reference the tenant's rows hold.
// Runs inside the purge transaction, BEFORE any delete.
//
// A failure fails the purge: proceeding would destroy the rows and lose
// the only record of which objects to destroy, which is the gap #961 is
// about rather than a recovery from it.
func collectBlobRefs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error) {
	var all []string
	for _, src := range blobSources {
		args := make([]any, strings.Count(src.sql, "?"))
		for i := range args {
			args[i] = tenantID
		}
		var refs []string
		if err := tx.WithContext(ctx).Raw(src.sql, args...).Scan(&refs).Error; err != nil {
			return nil, fmt.Errorf("tenantpurge: collect object refs from %s: %w", src.table, err)
		}
		all = append(all, refs...)
	}
	return all, nil
}
