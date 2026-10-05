// Internal-package test: the routing decision is unexported, and it is
// the whole of #980.
package blobreap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// #980: product media is public-read, buyer artwork is in a separate
// private bucket, and a Reaper bound to one bucket silently mishandles
// the other.
//
// "Silently" is the part worth defending against. GCS answers a delete
// for a missing object with SUCCESS, because deletion is idempotent — so
// a reaper that computes the right key and sends it to the wrong bucket
// reports Deleted, the operator sees a clean report, and the buyer's
// photograph is still there. No error, no retry, no alert. These tests
// assert on WHICH deleter was called, not on the Outcome counts, because
// the counts look identical either way.

const (
	publicBucket  = "mark8ly-prod-media"
	privateBucket = "mark8ly-prod-artwork"
)

func bucketURL(b, key string) string {
	return "https://storage.googleapis.com/" + b + "/" + key
}

// twoBucketReaper wires both targets and returns the two fakes
// separately, so a test can prove the object went to the right one.
func twoBucketReaper(t *testing.T, live ...string) (*Reaper, *media.FakeUploader, *media.FakeUploader) {
	t.Helper()
	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	r := &Reaper{
		targets: []Target{
			{Bucket: publicBucket, Deleter: pub, Prefixes: media.ProductPrefixes},
			{Bucket: privateBucket, Deleter: priv, Prefixes: media.ArtworkPrefixes},
		},
		logger: quiet(), max: DefaultMaxObjects,
	}
	set := make(map[string]struct{}, len(live))
	for _, k := range live {
		set[k] = struct{}{}
	}
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) { return set, nil }
	return r, pub, priv
}

func TestReap_SendsArtworkToThePrivateBucketOnly(t *testing.T) {
	r, pub, priv := twoBucketReaper(t)

	artwork := "buyer-uploads/t1/cart-9/upload-1.jpg"
	product := "tenants/t1/products/media/aaa/one.jpg"

	out := r.Reap(context.Background(), []string{artwork, product})
	require.Equal(t, Outcome{Deleted: 2}, out)

	require.Equal(t, []string{artwork}, priv.DeletedKeys(),
		"the buyer's photograph must be deleted from the private bucket")
	require.Equal(t, []string{product}, pub.DeletedKeys(),
		"product media must be deleted from the public bucket")
}

func TestReap_RefusesArtworkWhenOnlyThePublicBucketIsWired(t *testing.T) {
	// The pre-#980 wiring: one reaper, the public bucket. The old code
	// accepted buyer-uploads/ against it — media.ownedPrefixes listed the
	// prefix globally — and issued the delete at the wrong bucket, which
	// succeeded and destroyed nothing.
	//
	// Now it must be refused outright and counted as not-ours, so the
	// report says "this was not mine to delete" instead of "deleted".
	fake := media.NewFakeUploader()
	r := New(nil, fake, publicBucket, quiet())
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) {
		return nil, nil
	}

	out := r.Reap(context.Background(), []string{"buyer-uploads/t1/cart-9/upload-1.jpg"})
	require.Equal(t, Outcome{SkippedNotOurs: 1}, out)
	require.Empty(t, fake.DeletedKeys(),
		"an artwork key must never be deleted against the public bucket")
}

func TestReap_RefusesAnArtworkKeyURLThatNamesThePublicBucket(t *testing.T) {
	// A URL is explicit about its bucket, so this names an object that
	// does not exist: an artwork key under the public bucket. It belongs
	// to NEITHER target and must be refused rather than reassigned to
	// whichever bucket happens to own the prefix.
	//
	// Reassigning would be the dangerous reading: it would let a stored
	// URL in a column decide which bucket we delete from.
	r, pub, priv := twoBucketReaper(t)

	out := r.Reap(context.Background(), []string{
		bucketURL(publicBucket, "buyer-uploads/t1/cart-9/upload-1.jpg"),
		bucketURL(privateBucket, "tenants/t1/products/media/aaa/one.jpg"),
	})
	require.Equal(t, Outcome{SkippedNotOurs: 2}, out)
	require.Empty(t, pub.DeletedKeys())
	require.Empty(t, priv.DeletedKeys())
}

func TestReap_AcceptsArtworkAsEitherAKeyOrItsOwnBucketURL(t *testing.T) {
	// personalisation_uploads.storage_key_original holds a bare key;
	// nothing stops a future column holding the URL form. Both shapes
	// must land on the same object in the same bucket, once.
	r, pub, priv := twoBucketReaper(t)

	key := "buyer-uploads/t1/cart-9/upload-1.jpg"
	out := r.Reap(context.Background(), []string{key, bucketURL(privateBucket, key)})

	require.Equal(t, Outcome{Deleted: 1}, out, "the two shapes are one object")
	require.Equal(t, []string{key}, priv.DeletedKeys())
	require.Equal(t, 1, priv.Deleted(key), "deleted once, not twice")
	require.Empty(t, pub.DeletedKeys())
}

func TestReap_KeepsArtworkAnOrderStillReferences(t *testing.T) {
	// The claimed-upload case. One object is named by both
	// personalisation_uploads and order_item_personalisations, and those
	// cascade from different parents (stores, order_items). If one row
	// survives the object belongs to whoever is left, exactly as a shared
	// product image does.
	kept := "buyer-uploads/t1/cart-9/kept.jpg"
	gone := "buyer-uploads/t1/cart-9/gone.jpg"
	r, pub, priv := twoBucketReaper(t, kept)

	out := r.Reap(context.Background(), []string{kept, gone})
	require.Equal(t, Outcome{Deleted: 1, SkippedStillReferenced: 1}, out)
	require.Equal(t, []string{gone}, priv.DeletedKeys(),
		"an object a live order still points at must survive")
	require.Empty(t, pub.DeletedKeys())
}

func TestReap_FailsClosedAcrossBothBuckets(t *testing.T) {
	// Not knowing what survives is not permission to delete, and that has
	// to hold for the private bucket too — this is the bucket where a
	// wrong delete destroys a photograph of a person rather than a
	// reproducible product shot.
	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	r := &Reaper{
		targets: []Target{
			{Bucket: publicBucket, Deleter: pub, Prefixes: media.ProductPrefixes},
			{Bucket: privateBucket, Deleter: priv, Prefixes: media.ArtworkPrefixes},
		},
		logger: quiet(), max: DefaultMaxObjects,
	}
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) {
		return nil, context.DeadlineExceeded
	}

	out := r.Reap(context.Background(), []string{
		"buyer-uploads/t1/cart-9/a.jpg",
		"tenants/t1/products/media/aaa/b.jpg",
	})
	require.Equal(t, Outcome{Failed: 2}, out)
	require.Empty(t, priv.DeletedKeys(), "a failed guard must destroy no artwork")
	require.Empty(t, pub.DeletedKeys())
}

func TestReap_DeduplicatesPerBucketNotPerKey(t *testing.T) {
	// Defensive, and cheap. The prefixes are disjoint today, so the same
	// key cannot legitimately appear in both buckets — but if someone
	// adds a third target with an overlapping prefix, collapsing on the
	// key alone would silently drop one of two genuinely distinct
	// objects. Keyed on (bucket, key), the count is right either way.
	shared := "tenants/t1/products/media/aaa/one.jpg"
	pubA, pubB := media.NewFakeUploader(), media.NewFakeUploader()
	r := &Reaper{
		targets: []Target{
			{Bucket: "bucket-a", Deleter: pubA, Prefixes: media.ProductPrefixes},
			{Bucket: "bucket-b", Deleter: pubB, Prefixes: media.ProductPrefixes},
		},
		logger: quiet(), max: DefaultMaxObjects,
	}
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) { return nil, nil }

	out := r.Reap(context.Background(), []string{
		bucketURL("bucket-a", shared),
		bucketURL("bucket-b", shared),
	})
	require.Equal(t, Outcome{Deleted: 2}, out, "same key, two buckets, two objects")
	require.Equal(t, []string{shared}, pubA.DeletedKeys())
	require.Equal(t, []string{shared}, pubB.DeletedKeys())
}

func TestNewTargets_DropsUnusableTargetsAndReportsBuckets(t *testing.T) {
	fake := media.NewFakeUploader()
	r := NewTargets(nil, quiet(),
		Target{Bucket: publicBucket, Deleter: fake, Prefixes: media.ProductPrefixes},
		// No deleter: the deployment has a private bucket name but an
		// uploader that cannot delete.
		Target{Bucket: privateBucket, Prefixes: media.ArtworkPrefixes},
		// No bucket: MARKETPLACE_PRIVATE_GCS_BUCKET unset.
		Target{Deleter: fake, Prefixes: media.ArtworkPrefixes},
		// No prefixes: would otherwise accept nothing but is still junk.
		Target{Bucket: "x", Deleter: fake},
	)
	require.Equal(t, []string{publicBucket}, r.Buckets())

	// And with the artwork target dropped, artwork is not-ours rather
	// than deleted against the surviving bucket.
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) { return nil, nil }
	out := r.Reap(context.Background(), []string{"buyer-uploads/t1/c/u.jpg"})
	require.Equal(t, Outcome{SkippedNotOurs: 1}, out)
	require.Empty(t, fake.DeletedKeys())
}

func TestReap_NoTargetsReportsEverythingAsNotOurs(t *testing.T) {
	r := NewTargets(nil, quiet())
	out := r.Reap(context.Background(), []string{
		"buyer-uploads/t1/c/u.jpg", "tenants/t1/products/media/a/b.jpg",
	})
	require.Equal(t, Outcome{SkippedNotOurs: 2}, out,
		"a clean zero would be indistinguishable from having nothing to do")
}
