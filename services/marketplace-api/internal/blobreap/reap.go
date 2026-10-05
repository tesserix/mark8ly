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
//
// # Objects live in more than one bucket (#980)
//
// Product media is public-read; buyer-supplied artwork is in a separate
// private bucket, because a photograph of someone's child is not the same
// category of object as a product shot. A Reaper therefore holds a SET of
// targets, each a (bucket, deleter, prefixes) triple, and routes every
// reference to exactly one of them by key prefix.
//
// Routing by prefix rather than by "try each bucket" is the whole point.
// "buyer-uploads/..." is a well-formed key under a prefix this service
// mints, so a bucket-blind check accepts it against the PUBLIC bucket —
// and GCS answers a delete for a missing object with success, by design,
// because deletion is idempotent. The reap would report Deleted and the
// photograph would still be sitting in the private bucket. A wrong answer
// that looks like a right one.
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

// Target is one bucket, the deleter that can reach it, and the key
// prefixes that live in it.
//
// Prefixes is not decoration and must not be widened to "everything we
// mint". It is what stops an artwork key being deleted against the public
// bucket — see the package doc.
type Target struct {
	Bucket   string
	Deleter  media.Deleter
	Prefixes []string
}

func (t Target) usable() bool {
	return t.Bucket != "" && t.Deleter != nil && len(t.Prefixes) > 0
}

// Reaper destroys objects. Construct with New or NewTargets; a Reaper
// with no usable target deletes nothing and reports every candidate as
// skipped, which is the correct behaviour for a deployment with no
// bucket configured.
type Reaper struct {
	targets []Target
	db      *gorm.DB
	logger  *slog.Logger
	max     int
	// checkRefs answers "which of these keys does a surviving row still
	// point at". A field rather than a direct call so the decision logic
	// is testable without a database — the queries themselves are
	// covered by the integration tests.
	checkRefs func(ctx context.Context, keys []string) (map[string]struct{}, error)
}

// New builds a Reaper over the PUBLIC media bucket only. deleter and
// bucket may be zero — see Reap.
//
// Kept for callers that only ever deal with product media. Note that it
// now accepts only media.ProductPrefixes: before #980 it also accepted
// artwork keys and would have issued their deletes against this bucket,
// which is the bug rather than a feature worth preserving.
func New(db *gorm.DB, deleter media.Deleter, bucket string, logger *slog.Logger) *Reaper {
	return NewTargets(db, logger, Target{
		Bucket:   bucket,
		Deleter:  deleter,
		Prefixes: media.ProductPrefixes,
	})
}

// NewTargets builds a Reaper over several buckets. Unusable targets — no
// bucket, no deleter, or no prefixes — are dropped, so a deployment with
// only a public bucket configured behaves exactly as New.
func NewTargets(db *gorm.DB, logger *slog.Logger, targets ...Target) *Reaper {
	if logger == nil {
		logger = slog.Default()
	}
	keep := make([]Target, 0, len(targets))
	for _, t := range targets {
		if t.usable() {
			keep = append(keep, t)
		}
	}
	r := &Reaper{targets: keep, db: db, logger: logger, max: DefaultMaxObjects}
	r.checkRefs = r.survivingReferences
	return r
}

// Buckets returns the buckets this reaper can reach, for logging.
func (r *Reaper) Buckets() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.targets))
	for _, t := range r.targets {
		out = append(out, t.Bucket)
	}
	return out
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
	if r == nil || len(r.targets) == 0 {
		// No bucket wired: say so in the report rather than reporting a
		// clean zero, which would be indistinguishable from "there was
		// nothing to delete".
		out.SkippedNotOurs = len(refs)
		return out
	}

	// Resolve to keys we own, collapsing duplicates. Order is preserved
	// so a capped run is deterministic and a re-run makes progress.
	//
	// Keys are deduplicated per (bucket, key): the same key in two
	// buckets is two objects, and nothing guarantees the prefixes stay
	// disjoint if someone adds a third bucket later.
	type candidate struct {
		key    string
		target *Target
	}
	seen := make(map[string]struct{}, len(refs))
	cands := make([]candidate, 0, len(refs))
	for _, ref := range refs {
		key, t, ok := r.resolve(ref)
		if !ok {
			out.SkippedNotOurs++
			continue
		}
		dedup := t.Bucket + "\x00" + key
		if _, dup := seen[dedup]; dup {
			continue
		}
		seen[dedup] = struct{}{}
		cands = append(cands, candidate{key: key, target: t})
	}
	keys := make([]string, 0, len(cands))
	for _, c := range cands {
		keys = append(keys, c.key)
	}
	if len(keys) == 0 {
		return out
	}
	if len(keys) > r.max {
		out.Unreaped = len(keys) - r.max
		keys = keys[:r.max]
		cands = cands[:r.max]
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

	for _, c := range cands {
		if _, still := live[c.key]; still {
			out.SkippedStillReferenced++
			continue
		}
		// Through the owning target's deleter, never a default one. A
		// delete issued at the wrong bucket succeeds — GCS treats a
		// missing object as already deleted — so getting this wrong is
		// silent.
		if err := c.target.Deleter.Delete(ctx, c.key); err != nil {
			out.Failed++
			// The key is a tenant id and a content hash — no personal
			// data — so naming it is safe and is what makes the failure
			// actionable.
			r.logger.Error("blobreap: object outlived its rows",
				"bucket", c.target.Bucket, "storage_key", c.key, "err", err)
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
func (r *Reaper) resolve(ref string) (string, *Target, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil, false
	}
	isURL := strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
	for i := range r.targets {
		t := &r.targets[i]
		candidate := ref
		if !isURL {
			// A bare key. Rebuild the URL the column would have held so
			// the ownership gate is applied in one place rather than
			// duplicated here.
			candidate = "https://storage.googleapis.com/" + t.Bucket + "/" + ref
		}
		// Checked against THIS target's prefixes, not the global set: a
		// URL naming the public bucket with an artwork key belongs to
		// neither target and must be refused, not reassigned.
		if key, ok := media.KeyFromBucketURLWithPrefixes(t.Bucket, candidate, t.Prefixes); ok {
			return key, t, true
		}
	}
	return "", nil, false
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

	// Buyer artwork (#980). One uploaded object is referenced TWICE once
	// an order claims it: personalisation_uploads keeps the cart-side row
	// and order_item_personalisations snapshots the same storage key onto
	// the order. The two cascade from different parents — stores and
	// order_items — so a delete that removes one can easily leave the
	// other alive, and the object still belongs to whoever is left.
	//
	// Both columns per table, because storage_key_original is the
	// pristine upload and storage_key is the preview, and they are
	// separate objects that are reaped independently.
	{"personalisation_uploads", "storage_key_original"},
	{"personalisation_uploads", "storage_key"},
	{"order_item_personalisations", "storage_key_original"},
	{"order_item_personalisations", "storage_key"},
}

// survivingReferences returns the subset of keys that some remaining row
// still references. Two queries per source regardless of key count.
func (r *Reaper) survivingReferences(ctx context.Context, keys []string) (map[string]struct{}, error) {
	live := make(map[string]struct{})

	// URL columns store the public form, so compare against both shapes.
	// Once per bucket: the same key could be stored as a URL naming
	// either, and asking only about one would miss a live reference and
	// delete an object somebody is still showing.
	urls := make([]string, 0, len(keys)*len(r.targets))
	for _, t := range r.targets {
		for _, k := range keys {
			urls = append(urls, "https://storage.googleapis.com/"+t.Bucket+"/"+k)
		}
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
			if key, _, ok := r.resolve(f); ok {
				live[key] = struct{}{}
			}
		}
	}
	return live, nil
}
