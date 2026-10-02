// Package blobreap destroys the GCS objects that deleted rows pointed at
// (#961).
//
// internal/customererasure does this for ONE customer's objects, where
// the subject owns everything being destroyed. Tenant purge and the
// 150-day hard delete are a different problem, and the difference is the
// reason this is a separate package rather than a copied function:
//
// # Product media objects are SHARED
//
// product_media.storage_key is content-addressed — the key is a sha256 of
// the bytes — and copy-to-store deliberately points several product_media
// rows at one object. internal/product/models.go says so plainly: refcount
// is "a `count(*) on storage_key` query". order_items.image_url then
// snapshots the same URL onto every line that ever sold the product.
//
// So "this tenant's row is gone" does NOT mean "this object is
// unreferenced". Deleting on row-disappearance alone would punch holes in
// a surviving tenant's catalogue — the same class of mistake as trusting
// a stored URL, which #971 was about.
//
// Reap therefore does two things before deleting anything:
//
//  1. Provenance. Every reference goes through media.KeyFromOwnBucketURL,
//     so an object in someone else's bucket, or under a prefix we do not
//     mint, is skipped rather than destroyed.
//  2. Reference counting. After the purge has committed, it asks the
//     database which of the candidate keys are STILL referenced by a
//     surviving row, and skips those.
//
// Both are batch queries, not per-key ones: a tenant with a large
// catalogue can present thousands of candidates, and a query per object
// would make a purge quadratic in its own size.
package blobreap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// DefaultMaxObjects bounds one reap.
//
// Deletion happens after the purge transaction has committed, so a long
// loop here holds nothing open — but it does hold a worker, and an
// operator watching a purge deserves it to end. Anything above the cap is
// reported as Unreaped rather than silently dropped; the bucket lifecycle
// rule is the backstop, and a second run picks up where this stopped
// because the rows are already gone and the keys are recomputed from
// what remains.
const DefaultMaxObjects = 5000

// Outcome is what a reap did. Every candidate lands in exactly one count.
type Outcome struct {
	// Deleted is objects destroyed.
	Deleted int `json:"deleted"`
	// SkippedNotOurs is references naming a bucket or key prefix this
	// service does not mint. Expected to be non-zero: customer-supplied
	// avatar URLs and merchant webhook endpoints both live in columns
	// that look like object references and are not.
	SkippedNotOurs int `json:"skipped_not_ours"`
	// SkippedStillReferenced is objects a SURVIVING row still points at —
	// the shared content-addressed media case. Not an error; the object
	// belongs to whoever is left.
	SkippedStillReferenced int `json:"skipped_still_referenced"`
	// Failed is objects that are ours, unreferenced, and could not be
	// destroyed. Non-zero means data outlived its purge.
	Failed int `json:"failed"`
	// Unreaped is candidates beyond the cap, not yet examined.
	Unreaped int `json:"unreaped"`
}

// Empty reports whether anything at all happened.
func (o Outcome) Empty() bool { return o == Outcome{} }

// Reaper destroys objects. Construct with New; a zero Reaper deletes
// nothing and reports every candidate as skipped, which is the correct
// behaviour for a deployment with no bucket configured.
type Reaper struct {
	deleter media.Deleter
	bucket  string
	db      *gorm.DB
	logger  *slog.Logger
	max     int
	// checkRefs answers "which of these keys does a surviving row still
	// point at". A field rather than a direct call so the decision logic
	// is testable without a database — the queries themselves are
	// covered by the integration tests.
	checkRefs func(ctx context.Context, keys []string) (map[string]struct{}, error)
}

// New builds a Reaper. deleter and bucket may be zero — see Reap.
func New(db *gorm.DB, deleter media.Deleter, bucket string, logger *slog.Logger) *Reaper {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Reaper{deleter: deleter, bucket: bucket, db: db, logger: logger, max: DefaultMaxObjects}
	r.checkRefs = r.survivingReferences
	return r
}

// WithMax overrides the per-reap cap. Zero or negative restores the default.
func (r *Reaper) WithMax(n int) *Reaper {
	if n <= 0 {
		n = DefaultMaxObjects
	}
	r.max = n
	return r
}

// Reap destroys the objects named by refs that are ours and that no
// surviving row still references.
//
// refs are raw column values — public URLs or bare storage keys, mixed
// freely, because the columns they come from are mixed. Duplicates are
// expected and collapsed: product_media alone contributes url,
// storage_key and gcs_path_original for one object.
//
// MUST be called AFTER the purge transaction has committed. The
// reference check asks what survives, and inside the transaction the
// answer would still include the rows about to be deleted — every object
// would look referenced and nothing would ever be reaped.
func (r *Reaper) Reap(ctx context.Context, refs []string) Outcome {
	var out Outcome
	if len(refs) == 0 {
		return out
	}
	if r == nil || r.deleter == nil || r.bucket == "" {
		// No bucket wired: say so in the report rather than reporting a
		// clean zero, which would be indistinguishable from "there was
		// nothing to delete".
		out.SkippedNotOurs = len(refs)
		return out
	}

	// Resolve to keys we own, collapsing duplicates. Order is preserved
	// so a capped run is deterministic and a re-run makes progress.
	seen := make(map[string]struct{}, len(refs))
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		key, ok := r.resolve(ref)
		if !ok {
			out.SkippedNotOurs++
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return out
	}
	if len(keys) > r.max {
		out.Unreaped = len(keys) - r.max
		keys = keys[:r.max]
	}

	live, err := r.checkRefs(ctx, keys)
	if err != nil {
		// Fail CLOSED. Not knowing what survives is not permission to
		// delete: a failed guard that deleted anyway would destroy
		// another tenant's catalogue, which is the thing this exists to
		// prevent.
		r.logger.Error("blobreap: reference check failed; destroyed nothing",
			"candidates", len(keys), "err", err)
		out.Failed += len(keys)
		return out
	}

	for _, key := range keys {
		if _, still := live[key]; still {
			out.SkippedStillReferenced++
			continue
		}
		if err := r.deleter.Delete(ctx, key); err != nil {
			out.Failed++
			// The key is a tenant id and a content hash — no personal
			// data — so naming it is safe and is what makes the failure
			// actionable.
			r.logger.Error("blobreap: object outlived its rows", "storage_key", key, "err", err)
			continue
		}
		out.Deleted++
	}
	return out
}

// resolve turns one column value into a key we are entitled to delete.
//
// Columns hold two shapes. product_media.url and the *_url columns hold a
// public URL; storage_key, gcs_path_original and mockup_storage_key hold a
// bare key. Both are checked against the same ownership rule, so a bare
// key outside our prefixes is refused exactly as a foreign URL is.
func (r *Reaper) resolve(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return media.KeyFromOwnBucketURL(r.bucket, ref)
	}
	// A bare key. Run it through the same gate by rebuilding the URL the
	// column would have held, so the prefix allowlist is applied in one
	// place rather than duplicated here.
	return media.KeyFromOwnBucketURL(r.bucket, "https://storage.googleapis.com/"+r.bucket+"/"+ref)
}

// referenceSources are the columns a SURVIVING row could still point at a
// candidate object with.
//
// Only the genuinely shareable ones are listed. product_media is the
// content-addressed table copy-to-store duplicates; order_items snapshots
// its URL onto every sold line and outlives the product. The per-entity
// columns — logos, avatars, mockups — carry a random component and cannot
// collide across tenants, so querying them would cost a scan to prove
// something the key format already guarantees.
//
// Adding a table here is cheap and always safe; omitting one that CAN
// share is how a surviving tenant loses an image.
var referenceSources = []struct {
	table  string
	column string
}{
	{"product_media", "storage_key"},
	{"product_media", "gcs_path_original"},
	{"product_media", "url"},
	{"order_items", "image_url"},
}

// survivingReferences returns the subset of keys that some remaining row
// still references. Two queries per source regardless of key count.
func (r *Reaper) survivingReferences(ctx context.Context, keys []string) (map[string]struct{}, error) {
	live := make(map[string]struct{})

	// URL columns store the public form, so compare against both shapes.
	urls := make([]string, 0, len(keys))
	for _, k := range keys {
		urls = append(urls, "https://storage.googleapis.com/"+r.bucket+"/"+k)
	}

	for _, src := range referenceSources {
		var found []string
		// `IN ?` with a slice, which is how every other query in this
		// service passes a list (outbox/publisher.go, order/repository.go).
		// GORM expands it; a pg-native `= ANY($1)` does not bind here,
		// because GORM uses `?` placeholders.
		q := fmt.Sprintf(
			`SELECT DISTINCT %s FROM %s WHERE %s IN ? OR %s IN ?`,
			src.column, src.table, src.column, src.column)
		if err := r.db.WithContext(ctx).Raw(q, keys, urls).Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("blobreap: scan %s.%s: %w", src.table, src.column, err)
		}
		for _, f := range found {
			if key, ok := r.resolve(f); ok {
				live[key] = struct{}{}
			}
		}
	}
	return live, nil
}
