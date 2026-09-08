// Command backfill-trial-start gives every store that has no subscription row
// one, dating its trial from the STORE's own created_at rather than from the
// moment of the backfill.
//
// Why any store lacks one: until #827 the only code that created a
// store_subscriptions row was reachable from a single CTA on the admin Billing
// page. A merchant who never opened that page has no row — no trial clock, no
// expiry, and no presence in any billing cron. #827 makes onboarding create
// the row going forward; this covers everyone who signed up before it.
//
// # The date is the whole point
//
// trial.EndsAt derives the trial end from the subscription row's created_at
// when trial_ends_at is NULL. A naive backfill would therefore hand every
// existing merchant a brand-new 90 days starting today, including merchants
// who signed up a year ago.
//
// So this writes trial_ends_at EXPLICITLY, as store.created_at + 90 days, and
// EndsAt treats a non-NULL value as authoritative. For a store older than 90
// days that writes a trial end in the past — an already-expired trial. That is
// the truthful answer and it is deliberate: the alternative is granting a free
// trial to every dormant account in the estate.
//
// It is also why --dry-run is the DEFAULT. The expired-trial rows are
// merchant-visible and feed the expiry cron the next time it runs; whoever
// runs this should read the summary first and decide, rather than discover it.
//
// # What it does not do
//
// No Stripe call, and no stripe_customer_id. A trial row does not need one
// (#827) and minting a customer for a dormant store would be both wasteful and
// a way for a bad billing key to fail the whole backfill. The customer is
// attached lazily when the merchant first adds a card.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	var (
		batchSize int
		apply     bool
	)
	flag.IntVar(&batchSize, "batch", 200, "rows fetched per DB scan")
	// Inverted deliberately: --dry-run defaulting to true would let
	// `-dry-run=false` write by accident from a shell that mangles the value.
	// Writing requires naming the intent.
	flag.BoolVar(&apply, "apply", false, "actually write rows (default: report only)")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("backfill-trial-start: DATABASE_URL not set")
		os.Exit(1)
	}
	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("backfill-trial-start: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	stats, err := run(ctx, conn, batchSize, apply, time.Now().UTC(), log)
	if err != nil {
		log.Error("backfill-trial-start: run failed", "err", err, "stats", stats)
		os.Exit(1)
	}
	if !apply {
		log.Info("backfill-trial-start: DRY RUN — nothing was written; re-run with -apply", "stats", stats)
		return
	}
	log.Info("backfill-trial-start: done", "stats", stats)
}

// storeRow is the projection this command reads. Declared here rather than
// importing the stores model so the backfill cannot start depending on
// unrelated columns.
type storeRow struct {
	ID       string
	TenantID string
	Currency string
	// CreatedAt is NULL for a store mirrored before migration 136, which
	// added the column. Such a row cannot be dated and is SKIPPED rather
	// than guessed at — see run().
	CreatedAt *time.Time
}

type runStats struct {
	Scanned int
	Created int
	// Undated counts stores whose projection has no created_at — mirrored
	// before migration 136 and not re-synced since. They are skipped, not
	// guessed at, and reported so the operator knows the run was partial.
	Undated int
	// AlreadyExpired counts rows whose backfilled trial end is in the PAST,
	// because the store is older than the trial length. Reported separately
	// because it is the number a human needs before running with -apply: it
	// is how many merchants become "trial expired" the moment this lands.
	AlreadyExpired int
	Failed         int
}

func run(ctx context.Context, conn *gorm.DB, batchSize int, apply bool, now time.Time, log *slog.Logger) (runStats, error) {
	var stats runStats
	lastID := ""

	for {
		var rows []storeRow
		q := conn.WithContext(ctx).
			Table("stores s").
			Select("s.id AS id, s.tenant_id AS tenant_id, s.currency_code AS currency, s.created_at AS created_at").
			Joins("LEFT JOIN store_subscriptions ss ON ss.store_id = s.id").
			Where("ss.id IS NULL").
			Order("s.id ASC").
			Limit(batchSize)
		if lastID != "" {
			q = q.Where("s.id > ?", lastID)
		}
		if err := q.Scan(&rows).Error; err != nil {
			return stats, fmt.Errorf("scan stores without a subscription: %w", err)
		}
		if len(rows) == 0 {
			return stats, nil
		}

		for _, r := range rows {
			lastID = r.ID
			stats.Scanned++

			// No creation date, no trial date. Using synced_at instead would
			// date the trial from the last time this projection copied the
			// row — i.e. from a store-settings edit — and using now() would
			// hand a year-old store a fresh 90 days. Both are worse than
			// saying so and moving on; platform-api re-sends created_at on
			// its next upsert, and a later run picks the row up.
			if r.CreatedAt == nil {
				stats.Undated++
				log.Warn("backfill-trial-start: store has no created_at — skipping",
					"store_id", r.ID, "tenant_id", r.TenantID,
					"hint", "re-sync the store from platform-api, then re-run")
				continue
			}

			trialEnd, expired := backfilledTrialEnd(*r.CreatedAt, now)
			if expired {
				stats.AlreadyExpired++
			}

			log.Info("backfill-trial-start: store has no subscription",
				"store_id", r.ID, "tenant_id", r.TenantID,
				"store_created_at", r.CreatedAt.UTC(),
				"trial_ends_at", trialEnd, "already_expired", expired,
				"will_write", apply)

			if !apply {
				continue
			}

			// Raw INSERT rather than the repository: the repository's Create
			// takes the full model and would carry defaults this command is
			// deliberately not setting (no Stripe customer, no period). The
			// ON CONFLICT makes a re-run converge rather than fail, and makes
			// this safe to run alongside live onboarding traffic that may be
			// creating the same row concurrently.
			res := conn.WithContext(ctx).Exec(`
				INSERT INTO store_subscriptions
					(tenant_id, store_id, stripe_customer_id, plan, status, billing_currency, trial_ends_at, created_at)
				VALUES (?, ?, '', 'trial', 'signup', NULLIF(LOWER(?), ''), ?, ?)
				ON CONFLICT (store_id) DO NOTHING`,
				r.TenantID, r.ID, r.Currency, trialEnd, r.CreatedAt.UTC())
			if res.Error != nil {
				stats.Failed++
				log.Error("backfill-trial-start: insert failed",
					"store_id", r.ID, "err", res.Error)
				continue
			}
			if res.RowsAffected > 0 {
				stats.Created++
			}
		}
	}
}

// backfilledTrialEnd is the rule this command exists to apply: a trial dated
// from the STORE's creation, not from the moment of the backfill. It also
// reports whether that end has already passed.
//
// Its own function so the rule can be tested without a database, because it is
// the part with a merchant-visible consequence: get it wrong in one direction
// and every dormant account in the estate receives a fresh 90-day trial; get
// it wrong in the other and an active merchant's trial ends early.
//
// Uses trial.TrialDays rather than a local 90 so it cannot drift from the
// trial length the rest of the service enforces.
func backfilledTrialEnd(storeCreatedAt, now time.Time) (end time.Time, alreadyExpired bool) {
	// trial.EndsAt, not arithmetic. It is the ONLY definition of a trial end
	// (#353), and TestTrialEndIsDerivedInExactlyOnePlace fails the build for
	// any file that recomputes one — which this did, on its first draft.
	//
	// The subscription passed in is the row this command is about to write,
	// as it would look once written: created at the store's signup, never
	// extended. So the date asked for here is exactly the date stored below.
	end = trial.EndsAt(subscription.StoreSubscription{CreatedAt: storeCreatedAt})
	// Not After, not Before: an end exactly at `now` has passed. Same
	// boundary trial.Extendable uses, and the two must agree or a backfilled
	// row could be reported here as live and refused there.
	return end, !end.After(now)
}
