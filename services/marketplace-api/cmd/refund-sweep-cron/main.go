// Command refund-sweep-cron re-drives refund_transactions rows stuck in
// 'pending' — the never-lost guarantee for refunds. A row is stuck when the
// gateway call succeeded but the process crashed (or the DB blipped) before
// the finalize transaction committed. The sweeper re-calls the gateway with
// the SAME idempotency key (a provider no-op if the money already moved)
// and completes the DB finalize + bookkeeping.
//
// Designed to run as a Cloud Run Job on a Cloud Scheduler trigger (every 5
// min). Exits non-zero only on infrastructure failures (DB connection,
// etc.) — a sweep that resumes zero rows is a normal, successful run.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("refund-sweep-cron: DATABASE_URL not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("refund-sweep-cron: db open failed", "err", err)
		os.Exit(1)
	}

	// KNOWN GAP: no secret store is wired here, so GatewayFor cannot resolve
	// the gsm:// credential references on payment_gateway_configs and every
	// gateway re-drive fails (loudly — see orderrefund.resolveCred). The
	// sweep still runs and rows stay 'pending' for a later attempt, which is
	// the same outcome as before, when the raw reference was handed to the
	// gateway and came back 401. Closing this needs GCP_PROJECT_ID /
	// SECRET_NAME_PREFIX / SHIPPING_SECRET_STORE in the CronJob env plus
	// Secret Manager access for its service account (tesserix-k8s), so it is
	// deliberately out of scope of the API-path fix.
	res := orderrefund.NewResolver(conn)
	pay := payment.NewService(payment.NewRepository(conn))
	orders := order.NewService(conn, order.NewRepository(), outbox.NewRepository(conn))
	// enabled=true: the sweep is itself the recovery path for the
	// REFUND_GATEWAY_ENABLED kill switch — a stuck pending row must still be
	// re-driven so a later flip back to enabled doesn't leave it orphaned.
	coord := orderrefund.NewCoordinator(conn, res, pay, orders, order.NewRepository(), true)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	n, err := coord.ResumePending(ctx, 5*time.Minute, 200)
	if err != nil {
		log.Error("refund-sweep-cron: run failed", "err", err)
		os.Exit(1)
	}

	log.Info("refund-sweep-cron: done", "resumed", n)
}
