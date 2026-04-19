// Command reconciliation-cron runs the daily Stripe subscription status
// reconciliation job (P17 Task 10).
//
// It fetches up to 500 active StoreSubscriptions with a stripe_subscription_id,
// compares each against Stripe's authoritative status, and emits:
//   - mark8ly_subscription_reconciliation_drift_total{drift_type} counter
//   - audit_logs rows via the audit.Emitter for ops triage
//
// Designed to run as a Kubernetes CronJob (manifest in tesserix-k8s). Exits 0
// on success even when drift is detected — drift is informational, not fatal.
// Exits non-zero only on infrastructure failures (DB connection, etc.).
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/audit"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/reconciliation"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("reconciliation-cron: DATABASE_URL not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("reconciliation-cron: db open failed", "err", err)
		os.Exit(1)
	}

	// Build Stripe client — optional. Without it, reconciliation is skipped
	// with a warning (local dev without a Stripe key).
	var stripeClient *billingstripe.Client
	if key := os.Getenv("STRIPE_BILLING_SECRET_KEY"); key != "" {
		stripeClient = billingstripe.New(key)
	} else {
		log.Warn("reconciliation-cron: STRIPE_BILLING_SECRET_KEY not set — reconciliation will be skipped")
	}

	// Audit emitter — fire-and-forget. A nil emitter is safe (recordDrift
	// nil-checks it).
	auditRepo := audit.NewRepository()
	auditEmitter := audit.NewEmitter(audit.EmitterConfig{
		DB:     conn,
		Repo:   auditRepo,
		Logger: log,
	})

	r := reconciliation.New(conn, stripeClient, auditEmitter, log)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	driftCount, err := r.RunOnce(ctx)
	if err != nil {
		log.Error("reconciliation-cron: run failed", "err", err)
		// Drain audit queue before exit.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer drainCancel()
		auditEmitter.Stop(drainCtx)
		os.Exit(1)
	}

	log.Info("reconciliation-cron: done", "drift_count", driftCount)

	// Drain the async audit queue before the process exits.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer drainCancel()
	auditEmitter.Stop(drainCtx)
}
