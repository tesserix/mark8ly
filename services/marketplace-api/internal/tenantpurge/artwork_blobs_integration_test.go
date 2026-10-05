//go:build integration

package tenantpurge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/blobreap"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #980: buyer artwork lives in a PRIVATE bucket, and until this landed the
// purge reaper was bound to the public one.
//
// The failure mode was not a crash. The rows naming a shopper's
// photograph cascaded away correctly, which also removed the 72h
// sweeper's only reach to the object — it sweeps on expires_at, and the
// row was gone — so nothing was left that knew the object existed. The
// photograph stayed in the bucket indefinitely.
//
// These tests exercise the real SQL in blobSources against the live
// schema, which is the half a unit test cannot cover: the queries reach
// order_item_personalisations through order_items and orders, and a wrong
// column name there fails inside the purge transaction and takes the
// whole purge with it.

const artworkBucket = "mark8ly-test-artwork"

// twoBucketReaper wires the reaper the way main.go does: the public media
// bucket and the private artwork bucket, each owning its own prefixes.
func twoBucketReaper(db *gorm.DB, pub, priv media.Deleter) *blobreap.Reaper {
	return blobreap.NewTargets(db, nil,
		blobreap.Target{Bucket: purgeBucket, Deleter: pub, Prefixes: media.ProductPrefixes},
		blobreap.Target{Bucket: artworkBucket, Deleter: priv, Prefixes: media.ArtworkPrefixes},
	)
}

// seedImageField creates a product and one image personalisation field on
// it, returning both ids.
//
// Built out properly rather than with random uuids because
// product_personalisation_fields carries a COMPOSITE foreign key —
// (product_id, store_id) references products (id, store_id) — and
// personalisation_uploads.field_id references it in turn. A random uuid
// aborts the transaction and poisons the rest of the suite.
func seedImageField(t *testing.T, db *gorm.DB, tenantID, storeID string) (productID, fieldID string) {
	t.Helper()
	productID, fieldID = uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id)
		 VALUES (?, ?, ?, ?, 'Figurine', 'draft', ?)`,
		productID, tenantID, storeID, "art-"+productID[:8], uuid.NewString()).Error)
	// kind='image' deliberately: the CHECK constraints reject max_length
	// on an image field and reject min_px/max_images on anything else, so
	// the kind and the columns have to agree.
	require.NoError(t, db.Exec(
		`INSERT INTO product_personalisation_fields
		   (id, tenant_id, store_id, product_id, key, label, kind, required, position, max_images, min_px)
		 VALUES (?, ?, ?, ?, 'photo', 'Your photo', 'image', true, 0, 1, 600)`,
		fieldID, tenantID, storeID, productID).Error)
	return productID, fieldID
}

// seedCartUpload inserts an unclaimed buyer upload — the state a shopper
// reaches by uploading on the product page and not checking out.
func seedCartUpload(t *testing.T, db *gorm.DB, tenantID, storeID, original, preview string) {
	t.Helper()
	productID, fieldID := seedImageField(t, db, tenantID, storeID)
	require.NoError(t, db.Exec(
		`INSERT INTO personalisation_uploads
		   (id, tenant_id, store_id, product_id, field_id, cart_token,
		    storage_key_original, storage_key, content_hash, content_type,
		    size_bytes, original_filename, state, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'image/jpeg', 1024, 'nana.jpg',
		         'verified', now() + interval '72 hours')`,
		uuid.NewString(), tenantID, storeID, productID, fieldID, uuid.NewString(),
		original, preview, uuid.NewString()).Error)
}

// seedOrderWithArtwork inserts an order carrying one claimed artwork
// snapshot, which is the state after checkout.
func seedOrderWithArtwork(t *testing.T, db *gorm.DB, tenantID, storeID, original, preview string) {
	t.Helper()
	orderID, itemID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders
		   (id, tenant_id, store_id, order_number, idempotency_key,
		    customer_email, subtotal, grand_total, currency_code)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 10, 10, 'EUR')`,
		orderID, tenantID, storeID, "ART-"+orderID[:8], "idem-"+orderID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO order_items
		   (id, order_id, title_snapshot, sku_snapshot, unit_price, quantity, line_total, currency_code)
		 VALUES (?, ?, 'Figurine', 'FIG-1', 10, 1, 10, 'EUR')`,
		itemID, orderID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO order_item_personalisations
		   (id, order_item_id, field_key, field_label, kind, price_delta,
		    storage_key_original, storage_key, position)
		 VALUES (?, ?, 'photo', 'Your photo', 'image', 0, ?, ?, 0)`,
		uuid.NewString(), itemID, original, preview).Error)
}

// The headline case. A purged tenant's buyer artwork must be destroyed,
// and destroyed from the PRIVATE bucket.
func TestIntegration_Purge_DestroysBuyerArtworkFromThePrivateBucket(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	tenantID := uuid.NewString()
	storeID := seedStore(t, db, tenantID)

	cartOriginal := "buyer-uploads/" + tenantID + "/cart-1/up-1.jpg"
	cartPreview := "buyer-uploads/" + tenantID + "/cart-1/up-1-preview.jpg"
	orderOriginal := "buyer-uploads/" + tenantID + "/cart-2/up-2.jpg"
	orderPreview := "buyer-uploads/" + tenantID + "/cart-2/up-2-preview.jpg"

	seedCartUpload(t, db, tenantID, storeID, cartOriginal, cartPreview)
	seedOrderWithArtwork(t, db, tenantID, storeID, orderOriginal, orderPreview)

	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, tenantID,
		[]string{storeID}, twoBucketReaper(db, pub, priv))
	require.NoError(t, err)

	// All four objects: both uploads, and the original AND the preview of
	// each. Leaving the preview would leave a recognisable photograph of
	// a person behind.
	require.Equal(t, 4, rep.Blobs.Deleted,
		"both originals and both previews must go: %+v", rep.Blobs)
	require.Zero(t, rep.Blobs.Failed)
	require.Zero(t, rep.Blobs.SkippedStillReferenced)

	for _, key := range []string{cartOriginal, cartPreview, orderOriginal, orderPreview} {
		require.Equal(t, 1, priv.Deleted(key), "not deleted from the private bucket: %s", key)
	}
	require.Empty(t, pub.DeletedKeys(),
		"artwork must never be deleted against the public media bucket")
}

// The pre-#980 wiring, asserted so a regression is loud.
//
// With only the public bucket wired, the artwork references are still
// COLLECTED — so a future reaper can act on them — but they must be
// reported as not-ours rather than deleted against the wrong bucket. A
// delete at the wrong bucket succeeds, because GCS treats a missing
// object as already gone, so the old code reported a clean success.
func TestIntegration_Purge_WithOnlyThePublicBucketRefusesArtwork(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	tenantID := uuid.NewString()
	storeID := seedStore(t, db, tenantID)

	original := "buyer-uploads/" + tenantID + "/cart-1/up-1.jpg"
	preview := "buyer-uploads/" + tenantID + "/cart-1/up-1-preview.jpg"
	seedCartUpload(t, db, tenantID, storeID, original, preview)

	pub := media.NewFakeUploader()
	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, tenantID,
		[]string{storeID}, blobreap.New(db, pub, purgeBucket, nil))
	require.NoError(t, err)

	require.Zero(t, rep.Blobs.Deleted, "nothing in the public bucket to delete")
	require.GreaterOrEqual(t, rep.Blobs.SkippedNotOurs, 2,
		"artwork must be reported as unreachable, not silently 'deleted': %+v", rep.Blobs)
	require.Empty(t, pub.DeletedKeys())

	// And the rows are gone regardless — the purge still did its job.
	var remaining int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM personalisation_uploads WHERE tenant_id = ?`,
		tenantID).Scan(&remaining).Error)
	require.Zero(t, remaining)
}

// The shared-object case for artwork, which is subtler than for product
// media because the two references cascade from DIFFERENT parents:
// personalisation_uploads from stores, order_item_personalisations from
// order_items. A purge that removes one tenant's cart row must not destroy
// an object another tenant's live order still points at.
func TestIntegration_Purge_LeavesArtworkAnotherTenantsOrderStillReferences(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)

	purged := uuid.NewString()
	survivor := uuid.NewString()
	purgedStore := seedStore(t, db, purged)
	survivorStore := seedStore(t, db, survivor)

	// One object, referenced from both tenants. Contrived for the purged
	// tenant's prefix, because what is being asserted is that the
	// SURVIVING reference wins over provenance-by-key-prefix — the key
	// looks like the purged tenant's and must still be spared.
	shared := "buyer-uploads/" + purged + "/cart-1/shared.jpg"
	sharedPreview := "buyer-uploads/" + purged + "/cart-1/shared-preview.jpg"

	seedCartUpload(t, db, purged, purgedStore, shared, sharedPreview)
	seedOrderWithArtwork(t, db, survivor, survivorStore, shared, sharedPreview)

	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, purged,
		[]string{purgedStore}, twoBucketReaper(db, pub, priv))
	require.NoError(t, err)

	require.Zero(t, rep.Blobs.Deleted,
		"an object a surviving order still displays must not be destroyed: %+v", rep.Blobs)
	require.Equal(t, 2, rep.Blobs.SkippedStillReferenced)
	require.Empty(t, priv.DeletedKeys())

	// The surviving tenant's rows are untouched.
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM order_item_personalisations p
		   JOIN order_items i ON i.id = p.order_item_id
		   JOIN orders o ON o.id = i.order_id
		  WHERE o.tenant_id = ?`, survivor).Scan(&n).Error)
	require.Equal(t, int64(1), n)
}

// Product media and buyer artwork in one purge must each go to their own
// bucket. This is the test that would have caught #980 by itself.
func TestIntegration_Purge_RoutesEachObjectToItsOwnBucket(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	tenantID := uuid.NewString()
	storeID := seedStore(t, db, tenantID)

	productKey := "tenants/" + tenantID + "/products/media/abc/photo.jpg"
	seedProductWithMedia(t, db, tenantID, storeID, productKey)

	original := "buyer-uploads/" + tenantID + "/cart-1/up-1.jpg"
	preview := "buyer-uploads/" + tenantID + "/cart-1/up-1-preview.jpg"
	seedCartUpload(t, db, tenantID, storeID, original, preview)

	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, tenantID,
		[]string{storeID}, twoBucketReaper(db, pub, priv))
	require.NoError(t, err)

	require.Equal(t, 3, rep.Blobs.Deleted, "one product image, two artwork objects: %+v", rep.Blobs)
	require.Zero(t, rep.Blobs.Failed)

	require.Equal(t, []string{productKey}, pub.DeletedKeys())
	require.ElementsMatch(t, []string{original, preview}, priv.DeletedKeys())
}
