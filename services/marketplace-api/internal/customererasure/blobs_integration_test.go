//go:build integration

// In the INTERNAL package like the other erasure integration tests: the
// object half is asserted through reapBlobs' effect on a fake bucket and
// on the stored receipt, both of which are unexported concerns.
package customererasure

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

const testBucket = "mark8ly-test-media"

func gcsURL(key string) string {
	return "https://storage.googleapis.com/" + testBucket + "/" + key
}

// storedReceipt reads the receipt back out of the request's notes, which
// is where an auditor would read it.
func storedReceipt(t *testing.T, f fixture) Receipt {
	t.Helper()
	var notes *string
	require.NoError(t, f.db.Raw(
		`SELECT notes FROM customer_erasure_requests WHERE id = ?`, f.request).
		Scan(&notes).Error)
	require.NotNil(t, notes, "a completed request must carry its receipt")

	var got Receipt
	require.NoError(t, json.Unmarshal([]byte(*notes), &got))
	return got
}

func TestErasure_DestroysTheObjectAndNotJustTheRow(t *testing.T) {
	f := newFixture(t)
	key := "tenants/" + f.tenantID.String() + "/reviews/abc123/photo.jpg"

	// The fixture seeds a non-GCS URL; point it at our bucket so the
	// delete path is the one under test.
	require.NoError(t, f.db.Exec(
		`UPDATE review_media SET url = ? WHERE id = ?`, gcsURL(key), f.subject.mediaID).Error)

	fake := media.NewFakeUploader()
	fake.Register(media.Attrs{StorageKey: key, Size: 1024, ContentType: "image/jpeg"})

	e := newExecutor(t, f.db).WithBlobDeleter(fake, testBucket)
	_, err := e.Process(context.Background(), f.request)
	require.NoError(t, err)

	// The row is gone AND the object is gone. Before #961 only the first
	// of these was true, and the photograph stayed in a public bucket.
	var rows int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM review_media WHERE id = ?`, f.subject.mediaID).Scan(&rows).Error)
	require.Zero(t, rows)
	require.Equal(t, 1, fake.Deleted(key), "the object outlived its row")

	require.Equal(t, 1, storedReceipt(t, f).Blobs.Deleted,
		"the receipt is the evidence; it has to say the object went too")
}

func TestErasure_CollectsObjectURLsBeforeDeletingTheRows(t *testing.T) {
	// The ordering property, asserted the only way it can be: the row is
	// definitely gone afterwards, and the object was definitely deleted —
	// which is only possible if the URL was read before the DELETE ran.
	f := newFixture(t)
	key := "tenants/" + f.tenantID.String() + "/reviews/ordering/p.jpg"
	require.NoError(t, f.db.Exec(
		`UPDATE review_media SET url = ? WHERE id = ?`, gcsURL(key), f.subject.mediaID).Error)

	fake := media.NewFakeUploader()
	fake.Register(media.Attrs{StorageKey: key, Size: 10, ContentType: "image/jpeg"})

	_, err := newExecutor(t, f.db).WithBlobDeleter(fake, testBucket).
		Process(context.Background(), f.request)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{key}, fake.DeletedKeys())
}

func TestErasure_LeavesAnObjectTheShopperMerelyPointedAt(t *testing.T) {
	// The storefront accepts any storage.googleapis.com URL as an avatar
	// (handlers/storefront/customer_account.go), so a shopper can name a
	// merchant's product image. Erasing that shopper must not destroy it.
	f := newFixture(t)
	foreign := "https://storage.googleapis.com/some-other-bucket/tenants/x/products/media/h/hero.jpg"
	require.NoError(t, f.db.Exec(
		`UPDATE customer_profiles SET avatar_url = ? WHERE id = ?`,
		foreign, f.subject.profileID).Error)

	fake := media.NewFakeUploader()
	_, err := newExecutor(t, f.db).WithBlobDeleter(fake, testBucket).
		Process(context.Background(), f.request)
	require.NoError(t, err)

	require.Empty(t, fake.DeletedKeys(),
		"an object in a bucket we do not own must never be destroyed on a shopper's request")

	r := storedReceipt(t, f)
	require.Positive(t, r.Blobs.SkippedNotOurs,
		"the receipt must record that a referenced object was left alone, not imply there was none")
	require.Zero(t, r.Blobs.Deleted)
}

func TestErasure_WithNoBucketConfiguredStillErasesRowsAndSaysSo(t *testing.T) {
	// The pre-#961 behaviour, now reported instead of silent.
	f := newFixture(t)
	require.NoError(t, f.db.Exec(
		`UPDATE review_media SET url = ? WHERE id = ?`,
		gcsURL("tenants/t/reviews/a/p.jpg"), f.subject.mediaID).Error)

	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	var rows int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM review_media WHERE id = ?`, f.subject.mediaID).Scan(&rows).Error)
	require.Zero(t, rows, "the row erasure must not depend on object deletion being wired")

	require.Positive(t, storedReceipt(t, f).Blobs.SkippedNotOurs)
}

func TestErasure_ObjectFailureDoesNotReopenACompletedRequest(t *testing.T) {
	// The database erasure has committed by the time objects are touched.
	// Marking the request failed would make it claimable, and the retry
	// would re-run statements whose rows are gone — producing a receipt
	// claiming zero rows were destroyed.
	f := newFixture(t)
	key := "tenants/" + f.tenantID.String() + "/reviews/boom/p.jpg"
	require.NoError(t, f.db.Exec(
		`UPDATE review_media SET url = ? WHERE id = ?`, gcsURL(key), f.subject.mediaID).Error)

	d := &failingDeleter{failOn: map[string]bool{key: true}}
	_, err := newExecutor(t, f.db).WithBlobDeleter(d, testBucket).
		Process(context.Background(), f.request)
	require.NoError(t, err, "a failed object delete must not fail the erasure")

	var status string
	require.NoError(t, f.db.Raw(
		`SELECT status FROM customer_erasure_requests WHERE id = ?`, f.request).
		Scan(&status).Error)
	require.Equal(t, StatusCompleted, status)

	r := storedReceipt(t, f)
	require.Equal(t, 1, r.Blobs.Failed,
		"the receipt must admit that data outlived its erasure")
	require.Zero(t, r.Blobs.Deleted)
}

func TestErasure_DoesNotTouchABystandersObjects(t *testing.T) {
	f := newFixture(t)
	subjectKey := "tenants/" + f.tenantID.String() + "/reviews/subject/p.jpg"
	otherKey := "tenants/" + f.tenantID.String() + "/reviews/other/p.jpg"

	require.NoError(t, f.db.Exec(`UPDATE review_media SET url = ? WHERE id = ?`,
		gcsURL(subjectKey), f.subject.mediaID).Error)
	require.NoError(t, f.db.Exec(`UPDATE review_media SET url = ? WHERE id = ?`,
		gcsURL(otherKey), f.other.mediaID).Error)

	fake := media.NewFakeUploader()
	fake.Register(media.Attrs{StorageKey: subjectKey, Size: 1, ContentType: "image/jpeg"})
	fake.Register(media.Attrs{StorageKey: otherKey, Size: 1, ContentType: "image/jpeg"})

	_, err := newExecutor(t, f.db).WithBlobDeleter(fake, testBucket).
		Process(context.Background(), f.request)
	require.NoError(t, err)

	require.Equal(t, 1, fake.Deleted(subjectKey))
	require.Zero(t, fake.Deleted(otherKey),
		"the bystander's photograph must survive the subject's erasure")
}
