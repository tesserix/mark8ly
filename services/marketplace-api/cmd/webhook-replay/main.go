// Command webhook-replay is an OPERATOR TOOL, not a service. It lists Stripe
// webhook events that recovery has given up on, and puts chosen ones back in
// front of it.
//
// WHY THIS EXISTS: manual_review_required is how an event stops churning
// after MaxRetries — and nothing ever cleared it. Both recovery queries and
// the stale alert exclude flagged rows, so a flagged event was invisible and
// permanent: no retry, no page, no path back. The 2026-09-21 readiness
// assessment found 31 events sitting there, seven of them invoice.paid.
//
// The flag is right; the one-way door was not. An operator who has fixed the
// cause — a missing store_subscriptions row, a handler bug now deployed —
// needs to say "try these again", which is all this does. It clears the flag
// and resets retry_count; the orphan cron (5m) picks the event up on its next
// pass and dispatches it under the usual advisory lock, so nothing here
// bypasses the normal path or applies an effect itself.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/webhook-replay -list
//	DATABASE_URL=... go run ./cmd/webhook-replay -events evt_1,evt_2
//	DATABASE_URL=... go run ./cmd/webhook-replay -all -dry-run
//
// -list prints what is stuck and changes nothing. -events replays the named
// ids. -all replays every flagged event and must be typed deliberately; it
// is the right answer after a fix that explains all of them, and the wrong
// one if you have not read the list first.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

const listLimit = 500

func main() {
	var (
		list   bool
		all    bool
		events string
		dryRun bool
	)
	flag.BoolVar(&list, "list", false, "print the events awaiting manual review and exit")
	flag.BoolVar(&all, "all", false, "replay every event awaiting manual review")
	flag.StringVar(&events, "events", "", "comma-separated event ids to replay")
	flag.BoolVar(&dryRun, "dry-run", false, "report what would be replayed without writing")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("webhook-replay: DATABASE_URL not set")
		os.Exit(1)
	}
	if !list && !all && events == "" {
		log.Error("webhook-replay: pass -list, -events, or -all")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("webhook-replay: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	repo := webhookevents.NewRepository()

	if list {
		if err := printStuck(ctx, conn, repo, log); err != nil {
			log.Error("webhook-replay: list failed", "err", err)
			os.Exit(1)
		}
		return
	}

	ids, err := targets(ctx, conn, repo, all, events)
	if err != nil {
		log.Error("webhook-replay: could not determine targets", "err", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		log.Info("webhook-replay: nothing to replay")
		return
	}

	replayed := 0
	for _, id := range ids {
		if dryRun {
			log.Info("webhook-replay: would replay", "event_id", id)
			continue
		}
		if err := repo.ClearManualReview(ctx, conn, id); err != nil {
			// One bad id must not strand the rest — they are independent
			// events, frequently for different tenants.
			log.Error("webhook-replay: could not clear the flag", "event_id", id, "err", err)
			continue
		}
		replayed++
		log.Info("webhook-replay: queued for the orphan cron", "event_id", id)
	}

	log.Info("webhook-replay: done",
		"dry_run", dryRun, "targeted", len(ids), "replayed", replayed,
		"next", "the orphan cron dispatches these on its next pass")
}

// printStuck reports what is awaiting manual review, so an operator reads
// before acting.
func printStuck(ctx context.Context, conn *gorm.DB, repo webhookevents.Repository, log *slog.Logger) error {
	rows, err := repo.ListManualReview(ctx, conn, listLimit)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		log.Info("webhook-replay: no events are awaiting manual review")
		return nil
	}
	for _, e := range rows {
		reason := ""
		if e.ProcessingError != nil {
			reason = *e.ProcessingError
		}
		_, attributed := e.ResolvedStoreID()
		log.Info("webhook-replay: awaiting manual review",
			"event_id", e.EventID,
			"event_type", e.EventType,
			"received_at", e.ReceivedAt.UTC().Format(time.RFC3339),
			"retry_count", e.RetryCount,
			"attributed_to_store", attributed,
			"last_error", reason)
	}
	log.Info("webhook-replay: total awaiting manual review", "count", len(rows))
	return nil
}

func targets(ctx context.Context, conn *gorm.DB, repo webhookevents.Repository, all bool, events string) ([]string, error) {
	if all {
		rows, err := repo.ListManualReview(ctx, conn, listLimit)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(rows))
		for _, e := range rows {
			ids = append(ids, e.EventID)
		}
		return ids, nil
	}

	var ids []string
	for _, raw := range strings.Split(events, ",") {
		if id := strings.TrimSpace(raw); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
