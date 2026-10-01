// This file is in the INTERNAL package because reapBlobs is unexported
// and needs an Executor built by hand — NewExecutor requires a database,
// and the decision this tests (which objects are ours to destroy) needs
// none.
package customererasure

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

const bucket = "mark8ly-prod-media"

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func ownURL(key string) string {
	return "https://storage.googleapis.com/" + bucket + "/" + key
}

type failingDeleter struct {
	failOn map[string]bool
	calls  []string
}

func (f *failingDeleter) Delete(_ context.Context, key string) error {
	f.calls = append(f.calls, key)
	if f.failOn[key] {
		return errors.New("gcs: permission denied")
	}
	return nil
}

func TestReapBlobs_DeletesOurObjects(t *testing.T) {
	fake := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: fake, blobBucket: bucket}

	out := e.reapBlobs(context.Background(), []string{
		ownURL("tenants/t1/reviews/abc/photo.jpg"),
		ownURL("users/u1/avatar/9f1.png"),
	})

	require.Equal(t, BlobOutcome{Deleted: 2}, out)
	require.ElementsMatch(t,
		[]string{"tenants/t1/reviews/abc/photo.jpg", "users/u1/avatar/9f1.png"},
		fake.DeletedKeys())
}

// The reason the provenance check exists.
//
// The storefront accepts ANY storage.googleapis.com URL as a customer's
// avatar (handlers/storefront/customer_account.go), so a shopper can
// point theirs at a merchant's product image. If erasure trusted the
// stored URL, filing an erasure request would destroy that image.
func TestReapBlobs_WillNotDeleteAnObjectAShopperMerelyNamed(t *testing.T) {
	fake := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: fake, blobBucket: bucket}

	out := e.reapBlobs(context.Background(), []string{
		// Another bucket entirely.
		"https://storage.googleapis.com/some-other-bucket/tenants/t9/products/media/x/hero.jpg",
		// Our bucket, but a key we never mint.
		ownURL("public/press-kit.zip"),
		// Not even a GCS URL.
		"https://cdn.example.com/avatar.png",
	})

	require.Equal(t, BlobOutcome{SkippedNotOurs: 3}, out)
	require.Empty(t, fake.DeletedKeys(), "nothing outside our own prefixes may be destroyed")
}

func TestReapBlobs_CountsFailuresWithoutStopping(t *testing.T) {
	// One bad object must not prevent the rest of the erasure's objects
	// from being destroyed — the subject asked for all of them to go.
	d := &failingDeleter{failOn: map[string]bool{"tenants/t1/reviews/b/two.jpg": true}}
	e := &Executor{logger: quietLogger(), blobs: d, blobBucket: bucket}

	out := e.reapBlobs(context.Background(), []string{
		ownURL("tenants/t1/reviews/a/one.jpg"),
		ownURL("tenants/t1/reviews/b/two.jpg"),
		ownURL("tenants/t1/reviews/c/three.jpg"),
	})

	require.Equal(t, BlobOutcome{Deleted: 2, Failed: 1}, out)
	require.Len(t, d.calls, 3, "a failure must not abort the remaining objects")
}

func TestReapBlobs_DeduplicatesTheSameObject(t *testing.T) {
	// Two rows may name one object. Deleting it twice would report two
	// destructions of one thing, and the receipt is evidence.
	fake := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: fake, blobBucket: bucket}

	out := e.reapBlobs(context.Background(), []string{
		ownURL("tenants/t1/reviews/a/one.jpg"),
		ownURL("tenants/t1/reviews/a/one.jpg"),
	})

	require.Equal(t, BlobOutcome{Deleted: 1}, out)
	require.Equal(t, 1, fake.Deleted("tenants/t1/reviews/a/one.jpg"))
}

func TestReapBlobs_WithNoDeleterWiredReportsSkippedNotSilence(t *testing.T) {
	// A deployment that cannot delete objects must say so in its
	// receipts. Reporting zero would be indistinguishable from "there
	// was nothing to delete", which is the pre-#961 lie.
	e := &Executor{logger: quietLogger()}
	out := e.reapBlobs(context.Background(), []string{ownURL("tenants/t1/x.jpg")})
	require.Equal(t, BlobOutcome{SkippedNotOurs: 1}, out)
}

func TestReapBlobs_WithBucketUnsetDeletesNothing(t *testing.T) {
	fake := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: fake}
	out := e.reapBlobs(context.Background(), []string{ownURL("tenants/t1/x.jpg")})
	require.Equal(t, BlobOutcome{SkippedNotOurs: 1}, out)
	require.Empty(t, fake.DeletedKeys())
}

func TestReapBlobs_NothingToDoIsNotAnOutcome(t *testing.T) {
	fake := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: fake, blobBucket: bucket}
	require.Equal(t, BlobOutcome{}, e.reapBlobs(context.Background(), nil))
}

func TestBlobSources_CoverEveryCustomerOwnedObjectColumn(t *testing.T) {
	// A guard, not a tautology: these two are the only columns in the
	// schema that hold a URL to an object belonging to the SUBJECT.
	// product_media, branding logos and user_profiles.avatar_url are
	// merchant content and survive an erasure — they belong to tenant
	// purge and the hard-delete sweeper.
	//
	// If a third customer-owned object column lands, add it here and the
	// collect query with it. The erasure plan's own coverage test will
	// catch the table; nothing but this catches the object.
	tables := make([]string, 0, len(blobSources))
	for _, s := range blobSources {
		tables = append(tables, s.table)
	}
	require.ElementsMatch(t, []string{"review_media", "customer_profiles"}, tables)
}
