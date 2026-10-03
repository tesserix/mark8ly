// Package customererasure — blobs.go: destroying the OBJECTS an erasure's
// rows pointed at, not just the rows (#961).
//
// # Order is the whole design
//
// The rows are deleted first, in the erasure transaction, and the objects
// only after that transaction commits. Never the other way round: a crash
// between the two must leave a row pointing at a missing object (ugly,
// recoverable) rather than a live row pointing at an object we already
// destroyed, or worse, an object destroyed for an erasure that then rolled
// back and left the data in place.
//
// The URLs therefore have to be COLLECTED before the delete statements
// run — once the rows are gone there is nothing left to tell us which
// objects they named.
//
// # A failed object delete does not fail the erasure
//
// By the time deletion runs, the database erasure has committed, and that
// is the part the regulation is about. Marking the request failed would
// make it claimable again, and the retry would re-run statements whose
// rows are already gone — producing a receipt that claims zero rows were
// destroyed, which executor.go's replay comment already identifies as a
// false record.
//
// So the outcome is recorded ON the receipt instead, and the receipt is
// amended after the fact. An operator reading it can see exactly how many
// objects outlived their rows, which is the honest answer and the
// actionable one.
package customererasure

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// BlobOutcome is the object half of a receipt.
type BlobOutcome struct {
	// Deleted is how many objects were destroyed.
	Deleted int `json:"deleted"`
	// SkippedNotOurs is how many referenced objects this service is not
	// entitled to delete, because the stored URL named a bucket or a key
	// prefix we do not mint.
	//
	// This is expected to be NON-ZERO in normal operation, and that is a
	// finding rather than a bug in this code: the storefront accepts any
	// storage.googleapis.com URL as a customer's avatar
	// (handlers/storefront/customer_account.go), so a stored avatar URL
	// routinely names an object nobody here created. Deleting those would
	// let a shopper have a merchant's product image destroyed by filing
	// an erasure request. See media.KeyFromOwnBucketURL.
	SkippedNotOurs int `json:"skipped_not_ours"`
	// Failed is how many of OUR objects could not be destroyed. Non-zero
	// means data outlived its erasure and someone has to look.
	Failed int `json:"failed"`
}

// blobSources are the columns that hold a URL to an object belonging to
// the erasure subject.
//
// review_media is listed even though nothing writes it today — the table
// exists (migration 000017) and internal/review only ever initialises an
// empty slice, so there are no review photos in any bucket yet. It is
// wired now so that the day review uploads ship, erasure already reaches
// them; a table added to the plan and forgotten here is exactly the gap
// #961 is about.
//
// product_media is deliberately ABSENT: it is merchant catalog content,
// not the subject's personal data, and it survives an erasure. Tenant
// purge and the hard-delete sweeper own those objects.
var blobSources = []struct {
	table string
	sql   string
	// binds is how many placeholders the query has, always an even
	// number of (storeID, email). Zero means the common case, 2.
	binds int
}{
	{
		table: "review_media",
		sql: `SELECT m.url FROM review_media m
		       JOIN reviews r ON r.id = m.review_id
		      WHERE r.store_id = ? AND r.customer_email = ?
		        AND m.url IS NOT NULL AND m.url <> ''`,
	},
	{
		// The buyer's artwork on their orders (#967). Lives in the
		// PRIVATE bucket, so with today's single-bucket reaper every one
		// of these is reported as skipped rather than destroyed — see
		// #980. Listed now so that when the second reaper lands these are
		// already covered, and so the receipt states plainly that a
		// photograph was left behind rather than implying there was none.
		table: "order_item_personalisations",
		sql: `SELECT p.storage_key_original FROM order_item_personalisations p
		       JOIN order_items i ON i.id = p.order_item_id
		       JOIN orders o ON o.id = i.order_id
		      WHERE o.store_id = ? AND o.customer_email = ?
		        AND p.storage_key_original IS NOT NULL
		      UNION
		      SELECT p.storage_key FROM order_item_personalisations p
		       JOIN order_items i ON i.id = p.order_item_id
		       JOIN orders o ON o.id = i.order_id
		      WHERE o.store_id = ? AND o.customer_email = ?
		        AND p.storage_key IS NOT NULL`,
		binds: 4,
	},
	{
		table: "customer_profiles",
		sql: `SELECT avatar_url FROM customer_profiles
		      WHERE store_id = ? AND email = ?
		        AND avatar_url IS NOT NULL AND avatar_url <> ''`,
	},
}

// collectBlobURLs reads every object URL the subject's rows reference.
// Runs inside the erasure transaction, BEFORE any delete statement.
//
// A failure here fails the erasure, deliberately: proceeding would delete
// the rows and silently lose the only record of which objects to destroy,
// which is the pre-existing bug rather than a recovery from it.
func collectBlobURLs(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, email string) ([]string, error) {
	var all []string
	for _, src := range blobSources {
		n := src.binds
		if n <= 0 {
			n = 2
		}
		bind := make([]any, 0, n)
		for i := 0; i < n/2; i++ {
			bind = append(bind, storeID, email)
		}
		var urls []string
		if err := tx.WithContext(ctx).Raw(src.sql, bind...).Scan(&urls).Error; err != nil {
			// The table name is safe to report; the driver message is not
			// — it can embed the bound email. Same rule as StepError.
			return nil, fmt.Errorf("customererasure: collect object urls from %s", src.table)
		}
		all = append(all, urls...)
	}
	return all, nil
}

// reapBlobs destroys the objects named by urls that this service is
// entitled to destroy, and reports what happened to the rest.
//
// Nil-safe in both directions: no deleter wired, or no bucket configured,
// means every URL is counted as skipped rather than silently forgotten —
// a deployment that cannot delete objects should say so in its receipts.
func (e *Executor) reapBlobs(ctx context.Context, urls []string) BlobOutcome {
	var out BlobOutcome
	if len(urls) == 0 {
		return out
	}
	if e.blobs == nil || e.blobBucket == "" {
		out.SkippedNotOurs = len(urls)
		return out
	}

	// Deduplicate: two rows may name the same object, and deleting it
	// twice would report two deletions of one thing.
	seen := make(map[string]struct{}, len(urls))
	for _, raw := range urls {
		key, ok := media.KeyFromOwnBucketURL(e.blobBucket, raw)
		if !ok {
			out.SkippedNotOurs++
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		if err := e.blobs.Delete(ctx, key); err != nil {
			out.Failed++
			// The key is ours and carries no personal data — a tenant id
			// and a content hash — so it is safe to name, and naming it is
			// what makes the failure actionable.
			e.logger.Error("customer erasure: object outlived its row",
				"storage_key", key, "err", err)
			continue
		}
		out.Deleted++
	}
	return out
}

// amendReceiptWithBlobs rewrites the stored receipt once the object half
// is known.
//
// A separate write from the erasure transaction, and deliberately so: the
// objects cannot be deleted inside it (a network call under an advisory
// lock, unrollbackable if the transaction then fails), and the receipt
// must describe what actually happened rather than what was about to be
// attempted.
//
// context.WithoutCancel for the reason markFailed uses it: the operator
// may well have closed the tab, and losing the amendment would leave a
// receipt that overstates the erasure.
func (e *Executor) amendReceiptWithBlobs(ctx context.Context, requestID uuid.UUID, receipt Receipt) {
	notes, err := json.Marshal(receipt)
	if err != nil {
		e.logger.Error("customer erasure: encode amended receipt",
			"request_id", requestID.String(), "err", err)
		return
	}
	res := e.db.WithContext(context.WithoutCancel(ctx)).Exec(`
		UPDATE customer_erasure_requests SET notes = ?
		 WHERE id = ? AND status = ?`, string(notes), requestID, StatusCompleted)
	if res.Error != nil {
		// The erasure stands; only the evidence of the object half is
		// missing. Loud, because a receipt that silently omits it is
		// indistinguishable from one where nothing needed deleting.
		e.logger.Error("customer erasure: could not record the object half of the receipt",
			"request_id", requestID.String(),
			"blobs_deleted", receipt.Blobs.Deleted,
			"blobs_failed", receipt.Blobs.Failed,
			"err", res.Error)
	}
}

// WithBlobDeleter wires object deletion. Without it an erasure destroys
// rows only — which is how this service behaved before #961 — and every
// referenced object is reported as skipped rather than quietly ignored.
//
// bucket is the name of OUR bucket: media.KeyFromOwnBucketURL refuses any
// URL naming a different one, which is what stops a shopper-supplied
// avatar URL from aiming this at someone else's object.
func (e *Executor) WithBlobDeleter(d media.Deleter, bucket string) *Executor {
	e.blobs = d
	e.blobBucket = bucket
	return e
}

// logBlobOutcome records the object half next to the row half.
func logBlobOutcome(logger *slog.Logger, requestID uuid.UUID, out BlobOutcome) {
	if out == (BlobOutcome{}) {
		return
	}
	logger.Info("customer erasure: objects",
		"request_id", requestID.String(),
		"deleted", out.Deleted,
		"skipped_not_ours", out.SkippedNotOurs,
		"failed", out.Failed,
	)
}
