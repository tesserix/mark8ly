// Command erasure-worker runs GDPR art.17 customer erasures without the
// console (#259).
//
// It exists because the console must not be the only way to satisfy a
// statutory obligation. GDPR's response window keeps running while
// platform-admin is down, mid-deploy, or unreachable to whoever is on call,
// and "we could not reach our own admin UI" is not a defence a regulator
// accepts. This binary needs nothing but DATABASE_URL.
//
// Usage:
//
//	erasure-worker --request-id <uuid>   process exactly one request
//	erasure-worker --all [--limit N]     process the pending queue, bounded
//
// One of the two is REQUIRED. There is no default mode: a binary that erases
// customer data when run with no arguments is a mistake waiting to be made,
// and a no-argument invocation is far more likely to be a typo than an
// instruction to destroy every pending subject's data.
//
// Exit codes: 0 when every request attempted completed (a run that finds
// nothing pending is a normal, successful run); 1 on a usage or
// infrastructure failure; 2 when at least one erasure failed — those rows are
// left in 'failed' with a PII-free note and are claimable again on the next
// run.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customererasure"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

// defaultLimit bounds --all. An erasure is irreversible and holds a
// per-store advisory lock, so a run that swept an unbounded queue would be
// both a long lock hold and a large blast radius from one command. Raise it
// deliberately with --limit.
const defaultLimit = 50

// runTimeout bounds the whole run. Each erasure is one transaction over a
// couple of dozen statements; a run that has not finished in this long is
// stuck on a lock, and failing loudly beats holding it longer.
const runTimeout = 10 * time.Minute

const (
	exitUsageOrInfra = 1
	exitErasureFail  = 2
)

func main() {
	var (
		requestID = flag.String("request-id", "", "process exactly one erasure request by id")
		all       = flag.Bool("all", false, "process pending erasure requests, oldest first")
		limit     = flag.Int("limit", defaultLimit, "maximum requests to process with --all")
	)
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	one, sweep := *requestID != "", *all
	if one == sweep {
		log.Error("erasure-worker: pass exactly one of --request-id or --all")
		os.Exit(exitUsageOrInfra)
	}
	if *all && *limit <= 0 {
		log.Error("erasure-worker: --limit must be positive")
		os.Exit(exitUsageOrInfra)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("erasure-worker: DATABASE_URL not set")
		os.Exit(exitUsageOrInfra)
	}
	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("erasure-worker: db open failed", "err", err)
		os.Exit(exitUsageOrInfra)
	}

	executor, err := customererasure.NewExecutor(conn, log)
	if err != nil {
		log.Error("erasure-worker: executor could not be built", "err", err)
		os.Exit(exitUsageOrInfra)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	ids, err := targets(ctx, executor, *requestID, *all, *limit)
	if err != nil {
		log.Error("erasure-worker: could not determine what to process", "err", err)
		os.Exit(exitUsageOrInfra)
	}

	var completed, skipped, failed int
	for _, id := range ids {
		receipt, err := executor.Process(ctx, id)
		switch {
		case err == nil:
			completed++
			// Counts and table names only — never the subject's address.
			log.Info("erasure-worker: request completed",
				"request_id", id.String(),
				"deleted_tables", len(receipt.Deleted),
				"anonymised_tables", len(receipt.Anonymised),
				"retained_tables", len(receipt.RetainedTables))
		case errors.Is(err, customererasure.ErrAlreadyClaimed):
			// Another worker, or an operator in the console, got there first.
			// Not a failure: the request is somebody's, just not ours.
			skipped++
			log.Info("erasure-worker: request already claimed elsewhere", "request_id", id.String())
		case errors.Is(err, customererasure.ErrRequestNotFound):
			// Only reachable via --request-id; --all reads live ids.
			failed++
			log.Error("erasure-worker: no such erasure request", "request_id", id.String())
		default:
			// The row is now 'failed' with a PII-free note, written by the
			// executor OUTSIDE the rolled-back transaction, and is claimable
			// again on the next run.
			failed++
			log.Error("erasure-worker: request failed", "request_id", id.String(), "err", err)
		}
	}

	log.Info("erasure-worker: done", "completed", completed, "skipped", skipped, "failed", failed)
	if failed > 0 {
		os.Exit(exitErasureFail)
	}
}

// targets resolves the flags into the ids to attempt. --request-id does NOT
// consult the queue: an operator naming a specific request usually wants one
// the queue would not have offered — a 'failed' retry, or a row somebody else
// left in a strange state — and silently doing nothing would be the worst
// answer to an explicit instruction.
func targets(ctx context.Context, e *customererasure.Executor, requestID string, all bool, limit int) ([]uuid.UUID, error) {
	if !all {
		id, err := uuid.Parse(requestID)
		if err != nil {
			return nil, err
		}
		return []uuid.UUID{id}, nil
	}
	return e.PendingIDs(ctx, limit)
}
