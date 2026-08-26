// Command backfill-email populates store_subscriptions.email for rows that
// pre-date migration 104. customer.updated webhooks only fire on change, so
// historical customers with an email already set in Stripe won't have the
// column populated without this script (#381).
//
// Run once per environment after migration 104 lands. Idempotent — safe to
// re-run; it always re-reads from Stripe and writes the current value.
//
// Addresses Stripe reports that we would refuse to send to (the
// billing+<store_id>@mark8ly.local placeholders minted by subscription
// bootstrap) are counted as `Placeholder` and NOT written: storing one would
// only move the refusal from send time to a column nobody reads.
//
// Multi-tenant safety: each row is keyed by stripe_customer_id (1:1 with
// (tenant_id, store_id)). A failure on one row does not block other rows or
// other tenants — failures are logged and the script continues.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	var (
		batchSize int
		throttle  time.Duration
		dryRun    bool
	)
	flag.IntVar(&batchSize, "batch", 200, "rows fetched per DB scan")
	flag.DurationVar(&throttle, "throttle", 50*time.Millisecond, "sleep between Stripe API calls (rate-limit hedge)")
	flag.BoolVar(&dryRun, "dry-run", false, "log changes without writing")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("backfill-email: DATABASE_URL not set")
		os.Exit(1)
	}
	stripeKey := os.Getenv("STRIPE_BILLING_SECRET_KEY")
	if stripeKey == "" {
		log.Error("backfill-email: STRIPE_BILLING_SECRET_KEY not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("backfill-email: db open failed", "err", err)
		os.Exit(1)
	}
	stripeClient := billingstripe.New(stripeKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	stats, err := run(ctx, conn, stripeClient, batchSize, throttle, dryRun, log)
	if err != nil {
		log.Error("backfill-email: run failed", "err", err, "stats", stats)
		os.Exit(1)
	}
	log.Info("backfill-email: done", "stats", stats)
}

type runStats struct {
	Scanned      int
	Updated      int
	Unchanged    int
	NoneInStripe int
	Placeholder  int // Stripe holds an address we would refuse to send to
	Failed       int
}

func run(ctx context.Context, conn *gorm.DB, sc *billingstripe.Client, batchSize int, throttle time.Duration, dryRun bool, log *slog.Logger) (runStats, error) {
	var stats runStats

	// Keyset pagination via id ordering — avoids OFFSET drift on a live table
	// and keeps memory bounded regardless of subscription count.
	lastID := ""

	for {
		var rows []subscription.StoreSubscription
		q := conn.WithContext(ctx).
			Where("stripe_customer_id <> ''").
			Order("id ASC").
			Limit(batchSize)
		if lastID != "" {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			return stats, nil
		}

		for i := range rows {
			row := &rows[i]
			stats.Scanned++
			lastID = row.ID.String()

			addr, err := billingstripe.GetCustomerEmail(ctx, sc, row.StripeCustomerID)
			if err != nil {
				stats.Failed++
				log.Warn("backfill-email: stripe lookup failed; skipping",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String(),
					"stripe_customer_id", row.StripeCustomerID,
					"err", err.Error())
				time.Sleep(throttle)
				continue
			}

			if addr == "" {
				stats.NoneInStripe++
				time.Sleep(throttle)
				continue
			}

			if err := email.ValidateRecipient(addr); err != nil {
				stats.Placeholder++
				log.Warn("backfill-email: stripe holds an undeliverable address; not storing",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String(),
					"reason", email.SkipReason(err))
				time.Sleep(throttle)
				continue
			}

			if row.Email != nil && *row.Email == addr {
				stats.Unchanged++
				time.Sleep(throttle)
				continue
			}

			if dryRun {
				log.Info("backfill-email: dry-run would update",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String())
				stats.Updated++
				time.Sleep(throttle)
				continue
			}

			// Per-row UPDATE keyed by id — narrowest possible blast radius.
			res := conn.WithContext(ctx).Exec(`
				UPDATE store_subscriptions
				SET email = ?, updated_at = now()
				WHERE id = ?`,
				addr, row.ID,
			)
			if res.Error != nil {
				stats.Failed++
				log.Warn("backfill-email: update failed",
					"store_id", row.StoreID.String(), "err", res.Error.Error())
				time.Sleep(throttle)
				continue
			}
			stats.Updated++
			time.Sleep(throttle)
		}

		if len(rows) < batchSize {
			return stats, nil
		}
	}
}
