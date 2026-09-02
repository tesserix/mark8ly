// Command webhook-delivery-prune-cron deletes webhook_deliveries rows past
// the retention window (webhook.RetentionWindow — 30 days on every plan).
// webhook_deliveries carries delivery bodies for merchant-configured
// OUTBOUND webhooks (#562): storage cost on a shared db-f1-micro with no
// matching merchant value once a delivery is old enough that nobody is
// going to replay it. Deliberately NOT tied to FeatureAuditRetentionDays —
// see DeliveryRepo.Prune for why "forever" on Pro does not apply here.
//
// NOT internal/webhookprune. That package prunes webhook_events — the raw
// INBOUND provider (Stripe) event log — on a different 30/90-day age split
// and runs in-process on the trial scheduler inside cmd/marketplace-api.
// Near-identical names, different tables: this binary is the standalone
// k8s CronJob for the outbound delivery table only.
//
// Designed to run as a Cloud Run Job on a Cloud Scheduler trigger (daily).
// Exits non-zero only on infrastructure failures (DB connection, etc.) — a
// prune that removes zero rows is a normal, successful run.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("webhook-delivery-prune-cron: DATABASE_URL not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("webhook-delivery-prune-cron: db open failed", "err", err)
		os.Exit(1)
	}

	deliveries := webhook.NewDeliveryRepo(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	n, err := deliveries.Prune(ctx, webhook.RetentionWindow)
	if err != nil {
		log.Error("webhook-delivery-prune-cron: run failed", "err", err)
		os.Exit(1)
	}

	log.Info("webhook-delivery-prune-cron: done", "pruned", n)
}
