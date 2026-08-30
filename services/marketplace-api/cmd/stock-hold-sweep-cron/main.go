// Command stock-hold-sweep-cron deletes stock holds that expired long ago
// (#229/#231).
//
// # This is housekeeping, not correctness
//
// Availability is COMPUTED, never stored:
//
//	available = variant_stock.quantity
//	          - SUM(qty) FILTER (WHERE state = 'held' AND expires_at > now())
//
// so an expired hold already reduces availability by nothing. If this job
// never runs again, prices, stock and checkout all stay correct — only dead
// rows accumulate. That is the whole reason availability is computed rather
// than stored: a "reserved" counter would have made this sweeper
// load-bearing, and a missed run would silently strand stock.
//
// Because of that, a failure here is not urgent and must not read as though
// it were. It exits non-zero only on infrastructure failures; a sweep that
// deletes zero rows is a normal, successful run.
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

// defaultBatch bounds one run's delete. Batched with FOR UPDATE SKIP LOCKED
// so concurrent runs on multiple replicas neither block each other nor
// double-delete, and so a large backlog is drained over several runs rather
// than in one long-held lock.
const defaultBatch = 500

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("stock-hold-sweep-cron: DATABASE_URL not set")
		os.Exit(1)
	}

	batch := defaultBatch
	if v := os.Getenv("STOCK_HOLD_SWEEP_BATCH"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			// Refuse rather than silently falling back: a typo'd batch size
			// that quietly becomes 500 is a configuration that lies.
			log.Error("stock-hold-sweep-cron: STOCK_HOLD_SWEEP_BATCH must be a positive integer", "value", v)
			os.Exit(1)
		}
		batch = n
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("stock-hold-sweep-cron: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	n, err := stockhold.NewRepository().Sweep(ctx, conn, batch)
	if err != nil {
		log.Error("stock-hold-sweep-cron: run failed", "err", err)
		os.Exit(1)
	}

	log.Info("stock-hold-sweep-cron: done", "deleted", n, "batch", batch)
}
