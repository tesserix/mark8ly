package media_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

const ourBucket = "mark8ly-prod-media"

func TestKeyFromOwnBucketURL_AcceptsOurOwnKeys(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{
			"https://storage.googleapis.com/" + ourBucket + "/tenants/t1/products/media/abc123/photo.jpg",
			"tenants/t1/products/media/abc123/photo.jpg",
		},
		{
			"https://storage.googleapis.com/" + ourBucket + "/users/u1/avatar/9f1.png",
			"users/u1/avatar/9f1.png",
		},
		{
			// Cache-busting query strings still name the same object.
			"https://storage.googleapis.com/" + ourBucket + "/users/u1/avatar/9f1.png?v=2",
			"users/u1/avatar/9f1.png",
		},
	} {
		got, ok := media.KeyFromOwnBucketURL(ourBucket, tc.url)
		require.Truef(t, ok, "should have been accepted: %s", tc.url)
		require.Equal(t, tc.want, got)
	}
}

// This is the test the function exists for.
//
// customer_profiles.avatar_url is written straight from the storefront and
// the only check it passes is HasPrefix("https://storage.googleapis.com/").
// A shopper can therefore name a merchant's product image — in this
// estate's own bucket or any other — and a URL-trusting deleter would
// destroy it on that shopper's erasure request.
func TestKeyFromOwnBucketURL_RefusesObjectsThatAreNotOurs(t *testing.T) {
	cases := map[string]string{
		"another bucket entirely": "https://storage.googleapis.com/someone-elses-bucket/tenants/t1/products/media/a/p.jpg",
		"our bucket name as a prefix of theirs": "https://storage.googleapis.com/" + ourBucket +
			"-evil/tenants/t1/products/media/a/p.jpg",
		"a key outside every prefix we mint": "https://storage.googleapis.com/" + ourBucket + "/public/brochure.pdf",
		"a different host":                   "https://cdn.example.com/" + ourBucket + "/tenants/t1/x.jpg",
		"an http URL":                        "http://storage.googleapis.com/" + ourBucket + "/tenants/t1/x.jpg",
		"path traversal out of the prefix":   "https://storage.googleapis.com/" + ourBucket + "/tenants/../../etc/x",
		"nothing after the bucket":           "https://storage.googleapis.com/" + ourBucket + "/",
		"empty":                              "",
		"not a URL at all":                   "tenants/t1/products/media/a/p.jpg",
	}
	for name, url := range cases {
		t.Run(name, func(t *testing.T) {
			key, ok := media.KeyFromOwnBucketURL(ourBucket, url)
			require.Falsef(t, ok, "accepted an object that is not ours: %s", url)
			require.Empty(t, key)
		})
	}
}

func TestKeyFromOwnBucketURL_RefusesEverythingWhenBucketIsUnknown(t *testing.T) {
	// A deployment with no configured bucket must delete nothing rather
	// than matching against "".
	_, ok := media.KeyFromOwnBucketURL("",
		"https://storage.googleapis.com//tenants/t1/products/media/a/p.jpg")
	require.False(t, ok)
}

func TestFakeUploader_RecordsDeletes(t *testing.T) {
	f := media.NewFakeUploader()
	f.Register(media.Attrs{StorageKey: "tenants/t1/x.jpg", Size: 10, ContentType: "image/jpeg"})

	require.NoError(t, f.Delete(context.Background(), "tenants/t1/x.jpg"))
	require.Equal(t, 1, f.Deleted("tenants/t1/x.jpg"))
	require.ElementsMatch(t, []string{"tenants/t1/x.jpg"}, f.DeletedKeys())

	// The object is gone from the registry too, so a Verify after a
	// delete fails the way the real bucket would.
	_, err := f.Verify(context.Background(), "tenants/t1/x.jpg")
	require.ErrorIs(t, err, media.ErrNotFound)
}

func TestFakeUploader_DeleteIsIdempotent(t *testing.T) {
	// Deletion is the desired end state, so a retry after a partial
	// failure must be able to reach "done" rather than erroring on the
	// half it already finished.
	f := media.NewFakeUploader()
	require.NoError(t, f.Delete(context.Background(), "tenants/t1/gone.jpg"))
	require.NoError(t, f.Delete(context.Background(), "tenants/t1/gone.jpg"))
	require.Equal(t, 2, f.Deleted("tenants/t1/gone.jpg"))
}
