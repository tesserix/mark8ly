package reconciliation

import (
	"context"
	"database/sql"
	"fmt"
)

// Spec is the daily schedule for the reconciliation pass.
//
// 02:15 sits between the 01:00 finalize and 03:00 hard-delete crons rather
// than on either hour: this pass makes up to batchSize Stripe API calls, and
// sharing a start minute with another job only makes a slow night harder to
// read in the logs.
const Spec = "15 2 * * *"

// RunWithLock runs one reconciliation pass, but only on the replica that wins
// a named advisory lock. The others log and return.
//
// This exists because the pass runs IN-PROCESS inside marketplace-api, which
// serves two replicas in production, and because the protection the code
// appeared to have was not real: the fetch query's FOR UPDATE SKIP LOCKED
// runs in auto-commit, so its row locks die with the statement and both pods
// select the same batch. Three comments in this package asserted the
// opposite; they are corrected.
//
// Double-running is not a crash — the work is idempotent — but it doubles the
// drift counter and writes every audit row twice, which is a poor property
// for the one job whose entire purpose is detecting discrepancy.
//
// The lock is session-scoped on a dedicated connection, taken with
// pg_try_advisory_lock so the loser SKIPS rather than queues: this is a daily
// sweep, and a second pass immediately behind the first would re-read rows
// the first has already reconciled. Same shape as
// billing/tax/revalidation.Cron, which solved this first.
func (r *Reconciler) RunWithLock(ctx context.Context) (int, error) {
	sqlDB, err := r.db.DB()
	if err != nil {
		return 0, fmt.Errorf("reconciliation: sql handle: %w", err)
	}

	// A dedicated connection, not the pool: a session-scoped advisory lock
	// belongs to the connection that took it, and anything from the pool may
	// be handed to another caller before the unlock runs.
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("reconciliation: dedicated conn: %w", err)
	}
	defer conn.Close()

	var acquired bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock(hashtext($1))`, cronLockKey,
	).Scan(&acquired); err != nil {
		return 0, fmt.Errorf("reconciliation: advisory lock: %w", err)
	}
	if !acquired {
		r.logger.Info("reconciliation: another replica holds the cron lock, skipping")
		return 0, nil
	}
	defer releaseLock(ctx, conn, r.logger)

	return r.RunOnce(ctx)
}

// releaseLock drops the advisory lock, and does it on a context that cannot
// already be cancelled: the lock is held by this connection until it is
// released or the connection closes, so a cancelled parent context must not
// be the reason it lingers.
func releaseLock(ctx context.Context, conn *sql.Conn, logger interface {
	Warn(msg string, args ...any)
}) {
	if _, err := conn.ExecContext(context.WithoutCancel(ctx),
		`SELECT pg_advisory_unlock(hashtext($1))`, cronLockKey); err != nil {
		logger.Warn("reconciliation: advisory unlock failed", "err", err)
	}
}
