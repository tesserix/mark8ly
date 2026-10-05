//go:build integration

// In the INTERNAL package like the other erasure integration tests: the
// object half is asserted through reapBlobs' effect on a fake bucket and
// on the stored receipt, both unexported concerns.
package customererasure

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// #980 on the GDPR path: a subject asks for erasure and their uploaded
// photograph must actually be destroyed.
//
// Two reasons it was not, before this:
//
//  1. the executor knew one bucket, and artwork lives in another; and
//  2. the artwork columns hold a BARE KEY, while reapBlobs only accepted
//     a public URL — so every reference was refused as "not ours".
//
// This exercises the real SQL as well as the routing. The artwork query
// reaches order_item_personalisations through order_items and orders and
// filters on orders.customer_email, which is the join a unit test cannot
// check.

const testArtworkBucket = "mark8ly-test-artwork"

// seedOrderArtwork hangs one claimed artwork snapshot off an existing
// order, returning the original and preview keys.
//
// order_item_personalisations has no tenant or store column by design —
// the snapshot carries no FK to the catalog so an order survives the
// merchant editing the field — so it is reached through order_items.
func seedOrderArtwork(t *testing.T, db *gorm.DB, orderID uuid.UUID, tag string) (original, preview string) {
	t.Helper()
	itemID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO order_items
		   (id, order_id, title_snapshot, sku_snapshot, unit_price, quantity, line_total, currency_code)
		 VALUES (?, ?, 'Figurine', 'FIG-1', 10, 1, 10, 'USD')`,
		itemID, orderID).Error)

	original = "buyer-uploads/" + orderID.String() + "/" + tag + "/up.jpg"
	preview = "buyer-uploads/" + orderID.String() + "/" + tag + "/up-preview.jpg"
	require.NoError(t, db.Exec(
		`INSERT INTO order_item_personalisations
		   (id, order_item_id, field_key, field_label, kind, price_delta,
		    storage_key_original, storage_key, position)
		 VALUES (?, ?, 'photo', 'Your photo', 'image', 0, ?, ?, 0)`,
		uuid.New(), itemID, original, preview).Error)
	return original, preview
}

func TestErasure_DestroysTheSubjectsArtworkFromThePrivateBucket(t *testing.T) {
	f := newFixture(t)
	original, preview := seedOrderArtwork(t, f.db, f.subject.orderID, "subject")

	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	priv.Register(media.Attrs{StorageKey: original, Size: 2048, ContentType: "image/jpeg"})
	priv.Register(media.Attrs{StorageKey: preview, Size: 256, ContentType: "image/jpeg"})

	e := newExecutor(t, f.db).
		WithBlobDeleter(pub, testBucket).
		WithArtworkDeleter(priv, testArtworkBucket)
	_, err := e.Process(context.Background(), f.request)
	require.NoError(t, err)

	// Both objects: the original the merchant printed from AND the
	// preview. Leaving the preview leaves a recognisable photograph of a
	// person behind, which is not a partial success.
	require.Equal(t, 1, priv.Deleted(original), "the subject's original survived their erasure")
	require.Equal(t, 1, priv.Deleted(preview), "the subject's preview survived their erasure")
	require.Empty(t, pub.DeletedKeys(),
		"artwork must never be deleted against the public media bucket")

	// The rows went too.
	var rows int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM order_item_personalisations p
		   JOIN order_items i ON i.id = p.order_item_id
		  WHERE i.order_id = ?`, f.subject.orderID).Scan(&rows).Error)
	require.Zero(t, rows)

	// The receipt is the evidence an auditor reads.
	require.GreaterOrEqual(t, storedReceipt(t, f).Blobs.Deleted, 2,
		"the receipt must record that the photographs went, not only the rows")
}

// Erasure is scoped to ONE store. The fixture's twin is a different
// tenant and store carrying the SAME email address, which is the case
// that decides whether one shopper's request can destroy another
// merchant's customer's artwork.
func TestErasure_LeavesAnotherStoresArtworkForTheSameEmail(t *testing.T) {
	f := newFixture(t)
	subjectOriginal, _ := seedOrderArtwork(t, f.db, f.subject.orderID, "subject")
	twinOriginal, twinPreview := seedOrderArtwork(t, f.db, f.twin.orderID, "twin")

	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	e := newExecutor(t, f.db).
		WithBlobDeleter(pub, testBucket).
		WithArtworkDeleter(priv, testArtworkBucket)
	_, err := e.Process(context.Background(), f.request)
	require.NoError(t, err)

	require.Equal(t, 1, priv.Deleted(subjectOriginal), "the subject's artwork must go")
	require.Zero(t, priv.Deleted(twinOriginal),
		"another store's customer artwork must survive an erasure scoped to this store")
	require.Zero(t, priv.Deleted(twinPreview))

	// And the twin's rows are intact.
	var rows int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM order_item_personalisations p
		   JOIN order_items i ON i.id = p.order_item_id
		  WHERE i.order_id = ?`, f.twin.orderID).Scan(&rows).Error)
	require.Equal(t, int64(1), rows)
}

// Without a private bucket the receipt must say "skipped" rather than
// claim a deletion. Honest reporting is the floor: somebody reads this
// receipt to answer "was it destroyed", and a clean zero would read as
// "there was nothing to destroy".
func TestErasure_WithoutAnArtworkBucketReportsTheArtworkAsSkipped(t *testing.T) {
	f := newFixture(t)
	seedOrderArtwork(t, f.db, f.subject.orderID, "subject")

	pub := media.NewFakeUploader()
	e := newExecutor(t, f.db).WithBlobDeleter(pub, testBucket)
	_, err := e.Process(context.Background(), f.request)
	require.NoError(t, err)

	require.Empty(t, pub.DeletedKeys(),
		"an artwork key must not be deleted against the public bucket")

	rec := storedReceipt(t, f)
	require.GreaterOrEqual(t, rec.Blobs.SkippedNotOurs, 2,
		"the receipt must admit the photographs were left behind: %+v", rec.Blobs)

	// The rows are still erased — the SQL half of an erasure does not
	// depend on having a bucket.
	var rows int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM order_item_personalisations p
		   JOIN order_items i ON i.id = p.order_item_id
		  WHERE i.order_id = ?`, f.subject.orderID).Scan(&rows).Error)
	require.Zero(t, rows)
}
