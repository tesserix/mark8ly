// Internal package: reapBlobs and resolveRef are unexported, and the
// decision they make is the whole of the #980 fix on the erasure side.
package customererasure

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// #980, erasure half.
//
// Two separate bugs met here, and the second is the nastier one:
//
//  1. the executor knew one bucket, and buyer artwork lives in another;
//  2. reapBlobs called KeyFromOwnBucketURL, which requires an
//     "https://storage.googleapis.com/" prefix — but the artwork columns
//     (order_item_personalisations.storage_key_original and .storage_key)
//     hold a BARE KEY. So every artwork reference was refused as "not
//     ours", and the erasure receipt said "skipped".
//
// The receipt being honest is the floor, not the goal. A subject asked
// for their data to be erased and their photograph stayed in a bucket.

const artworkBucket = "mark8ly-prod-artwork"

func artworkKey(suffix string) string {
	return "buyer-uploads/t1/cart-7/" + suffix
}

// bothBuckets builds an Executor wired for product media AND artwork,
// returning the two deleters separately so a test can prove which one was
// used.
func bothBuckets() (*Executor, *media.FakeUploader, *media.FakeUploader) {
	pub, priv := media.NewFakeUploader(), media.NewFakeUploader()
	e := &Executor{
		logger:        quietLogger(),
		blobs:         pub,
		blobBucket:    bucket,
		artwork:       priv,
		artworkBucket: artworkBucket,
	}
	return e, pub, priv
}

func TestReapBlobs_DestroysArtworkGivenAsABareKey(t *testing.T) {
	// The shape the real column holds. Before #980 this was counted as
	// SkippedNotOurs and the object survived the erasure request.
	e, pub, priv := bothBuckets()
	original, preview := artworkKey("up.jpg"), artworkKey("up-preview.jpg")

	out := e.reapBlobs(context.Background(), []string{original, preview})

	require.Equal(t, 2, out.Deleted, "%+v", out)
	require.Zero(t, out.SkippedNotOurs)
	require.Zero(t, out.Failed)
	require.ElementsMatch(t, []string{original, preview}, priv.DeletedKeys())
	require.Empty(t, pub.DeletedKeys(),
		"artwork must never be deleted against the public media bucket")
}

func TestReapBlobs_RoutesProductMediaAndArtworkToTheirOwnBuckets(t *testing.T) {
	e, pub, priv := bothBuckets()
	product := "tenants/t1/products/media/abc/photo.jpg"
	art := artworkKey("up.jpg")

	out := e.reapBlobs(context.Background(), []string{ownURL(product), art})

	require.Equal(t, 2, out.Deleted, "%+v", out)
	require.Equal(t, []string{product}, pub.DeletedKeys())
	require.Equal(t, []string{art}, priv.DeletedKeys())
}

func TestReapBlobs_WithoutAnArtworkDeleterReportsSkippedNotDeleted(t *testing.T) {
	// The honest-but-not-good-enough state: a deployment with no private
	// bucket. It must say "skipped" rather than claim a deletion, because
	// the receipt is what somebody reads to answer "was it destroyed".
	pub := media.NewFakeUploader()
	e := &Executor{logger: quietLogger(), blobs: pub, blobBucket: bucket}

	out := e.reapBlobs(context.Background(), []string{artworkKey("up.jpg")})

	require.Equal(t, 1, out.SkippedNotOurs, "%+v", out)
	require.Zero(t, out.Deleted)
	require.Empty(t, pub.DeletedKeys(),
		"an artwork key must not be deleted against the public bucket")
}

func TestReapBlobs_RefusesAnArtworkKeyURLNamingThePublicBucket(t *testing.T) {
	// A URL is explicit about its bucket, so this names nothing that
	// exists. It must be refused rather than reassigned to whichever
	// bucket owns the prefix — otherwise a stored URL in a column gets to
	// choose which bucket we delete from.
	e, pub, priv := bothBuckets()

	out := e.reapBlobs(context.Background(), []string{ownURL(artworkKey("up.jpg"))})

	require.Equal(t, 1, out.SkippedNotOurs, "%+v", out)
	require.Empty(t, pub.DeletedKeys())
	require.Empty(t, priv.DeletedKeys())
}

func TestReapBlobs_StillRefusesAForeignBucket(t *testing.T) {
	// The original reason KeyFromOwnBucketURL exists: a shopper could set
	// customer_profiles.avatar_url to any storage.googleapis.com URL.
	// Adding a second target must not widen that hole.
	e, pub, priv := bothBuckets()

	out := e.reapBlobs(context.Background(), []string{
		"https://storage.googleapis.com/someone-elses-bucket/tenants/t1/a.jpg",
		"https://storage.googleapis.com/someone-elses-bucket/buyer-uploads/t1/c/a.jpg",
		// A prefix-extension attack on each of our own bucket names.
		"https://storage.googleapis.com/" + bucket + "-evil/tenants/t1/a.jpg",
		"https://storage.googleapis.com/" + artworkBucket + "-evil/buyer-uploads/t1/c/a.jpg",
	})

	require.Equal(t, 4, out.SkippedNotOurs, "%+v", out)
	require.Empty(t, pub.DeletedKeys())
	require.Empty(t, priv.DeletedKeys())
}

func TestReapBlobs_RefusesAKeyOutsideEveryOwnedPrefix(t *testing.T) {
	// A bare key now reaches the gate without a URL to vouch for it, so
	// the prefix allowlist is the only thing standing between a column
	// value and a delete. Worth asserting directly.
	e, pub, priv := bothBuckets()

	out := e.reapBlobs(context.Background(), []string{
		"random/thing.jpg",
		"../../etc/passwd",
		"/tenants/t1/leading-slash.jpg",
		"",
	})

	require.Zero(t, out.Deleted, "%+v", out)
	require.Empty(t, pub.DeletedKeys())
	require.Empty(t, priv.DeletedKeys())
}

func TestReapBlobs_CollapsesTheKeyAndURLFormsOfOneArtworkObject(t *testing.T) {
	e, _, priv := bothBuckets()
	key := artworkKey("up.jpg")

	out := e.reapBlobs(context.Background(), []string{
		key,
		"https://storage.googleapis.com/" + artworkBucket + "/" + key,
	})

	require.Equal(t, 1, out.Deleted, "two shapes, one object: %+v", out)
	require.Equal(t, 1, priv.Deleted(key), "deleted once, not twice")
}

func TestReapBlobs_CountsAnArtworkDeleteFailureWithoutLosingTheRest(t *testing.T) {
	// A failure here means a photograph outlived an erasure request, so
	// it must be counted and logged rather than swallowed — and must not
	// stop the remaining objects being destroyed.
	bad := artworkKey("boom.jpg")
	good := artworkKey("ok.jpg")
	failing := &failingDeleter{failOn: map[string]bool{bad: true}}
	e := &Executor{
		logger:        quietLogger(),
		artwork:       failing,
		artworkBucket: artworkBucket,
	}

	out := e.reapBlobs(context.Background(), []string{bad, good})

	require.Equal(t, 1, out.Failed, "%+v", out)
	require.Equal(t, 1, out.Deleted)
	require.ElementsMatch(t, []string{bad, good}, failing.calls)
}
