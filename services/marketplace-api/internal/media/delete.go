// Package media — delete.go: removing an object from the bucket, and
// deciding whether a stored URL names an object we are entitled to
// remove (#961).
//
// # Why this did not exist
//
// Nothing in marketplace-api has ever deleted a GCS object. Erasure,
// tenant purge and the hard-delete sweeper are all SQL-only, so a GDPR
// art.17 request deleted the row that pointed at a customer's
// photograph and left the photograph itself in a public bucket
// indefinitely.
//
// # Why deletion needs a provenance check, not just a URL
//
// The obvious implementation — take the stored URL, strip the prefix,
// delete the key — is unsafe here, because not every stored URL names
// an object we minted.
//
// customer_profiles.avatar_url is written straight from the storefront
// (handlers/storefront/customer_account.go), and the only check it
// passes is HasPrefix("https://storage.googleapis.com/"). Any bucket,
// any object. A shopper can point their avatar at a merchant's product
// image — in this estate's own bucket — and a URL-trusting deleter
// would then destroy that image on the shopper's erasure request.
//
// So KeyFromOwnBucketURL answers a narrower question: does this URL name
// an object in OUR bucket, under a prefix WE generate? Anything else is
// not ours to delete, and the caller is expected to leave it alone and
// say so.
package media

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
)

// Deleter removes an object. Implemented by GCSUploader; the
// FakeUploader records calls so tests can assert a blob was destroyed
// rather than only that a row was.
//
// A capability interface rather than a method on Uploader, following
// MetadataSetter: callers type-assert, and a deployment with no real
// bucket simply has nothing to delete.
type Deleter interface {
	Delete(ctx context.Context, storageKey string) error
}

// Delete removes one object. A missing object is NOT an error: deletion
// is the desired end state, and an erasure that is retried after a
// partial failure must be able to reach "done" rather than failing
// forever on the half it already finished.
func (u *GCSUploader) Delete(ctx context.Context, storageKey string) error {
	if strings.TrimSpace(storageKey) == "" {
		return errors.New("media: refusing to delete an empty storage key")
	}
	err := u.bucket.Object(storageKey).Delete(ctx)
	if err == nil || errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return fmt.Errorf("media: delete object: %w", err)
}

// ownedPrefixes are the key prefixes this service generates. A key
// outside all of them was not minted here, whatever a stored URL claims.
//
// Keep in step with the key builders: BuildStorageKey (product media,
// "tenants/<id>/products/media/..."), buildAvatarStorageKey
// ("users/<id>/avatar/..."), and the branding logo path.
var ownedPrefixes = []string{
	"tenants/",
	"users/",
	// Buyer-supplied artwork (#963). Lives in the PRIVATE bucket, under a
	// prefix of its own so it can never be confused with product media by
	// a reaper, a lifecycle rule, or a human reading a bucket listing.
	//
	// Listing it here makes the prefix recognisable as ours; it does not
	// by itself make it reachable. A reaper is bound to one bucket, and
	// the tenant-purge reaper is bound to the public one — see the note
	// on the purge path.
	"buyer-uploads/",
}

// publicURLHost is the only host a stored URL may use to be considered
// ours. A CDN alias would need adding here deliberately — and would need
// the same bucket check, since a CDN in front of someone else's bucket
// is still someone else's bucket.
const publicURLHost = "https://storage.googleapis.com/"

// KeyFromOwnBucketURL extracts the object key from a persisted public
// URL, and reports false unless the URL names an object in `bucket`
// under a prefix this service generates.
//
// It returns false — rather than an error — for every rejection,
// because the caller's correct response to all of them is identical:
// leave the object alone.
//
// Rejected, deliberately:
//   - a different host (another provider, a CDN we have not vetted)
//   - a different bucket, which is the shopper-supplied-avatar case and
//     the reason this function exists
//   - a key outside ownedPrefixes
//   - path traversal, or a key that is empty after the bucket segment
func KeyFromOwnBucketURL(bucket, rawURL string) (string, bool) {
	if bucket == "" {
		return "", false
	}
	url := strings.TrimSpace(rawURL)
	if !strings.HasPrefix(url, publicURLHost) {
		return "", false
	}
	rest := strings.TrimPrefix(url, publicURLHost)

	// The bucket must be the whole first path segment. A prefix match
	// would accept "our-bucket-evil/..." as "our-bucket".
	prefix := bucket + "/"
	if !strings.HasPrefix(rest, prefix) {
		return "", false
	}
	key := strings.TrimPrefix(rest, prefix)

	// Strip any query or fragment: a signed or cache-busted URL still
	// names the same object, and leaving them on would address nothing.
	if i := strings.IndexAny(key, "?#"); i >= 0 {
		key = key[:i]
	}
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", false
	}

	for _, p := range ownedPrefixes {
		if strings.HasPrefix(key, p) {
			return key, true
		}
	}
	return "", false
}

// Delete implements Deleter for the fake, recording the key so tests can
// assert the object was destroyed and not merely dereferenced.
func (f *FakeUploader) Delete(_ context.Context, storageKey string) error {
	if f.deleted == nil {
		f.deleted = map[string]int{}
	}
	f.deleted[storageKey]++
	delete(f.attrs, storageKey)
	return nil
}

// Deleted reports how many times Delete was called for a key. Zero for a
// key never deleted.
func (f *FakeUploader) Deleted(storageKey string) int { return f.deleted[storageKey] }

// DeletedKeys returns every key Delete was called with.
func (f *FakeUploader) DeletedKeys() []string {
	out := make([]string, 0, len(f.deleted))
	for k := range f.deleted {
		out = append(out, k)
	}
	return out
}

// Compile-time check.
var (
	_ Deleter = (*GCSUploader)(nil)
	_ Deleter = (*FakeUploader)(nil)
)
