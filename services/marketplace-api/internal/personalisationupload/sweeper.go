package personalisationupload

import (
	"context"
	"log/slog"
	"time"
)

// SweepBatch bounds one sweep.
//
// The sweeper deletes a GCS object per row, so an unbounded batch is an
// unbounded run of network calls. Anything left over is picked up by the
// next tick — the rows are still expired, and the query is ordered by
// expiry so the oldest go first.
const SweepBatch = 200

// Sweeper destroys uploads nobody claimed.
//
// Without it the bucket accumulates every photograph a shopper ever
// abandoned, which is both a cost and — because these are pictures of
// people — a retention problem. 72 hours after an upload, if no order
// claimed it, it goes.
type Sweeper struct {
	repo   Repository
	svc    *Service
	logger *slog.Logger
}

// NewSweeper constructs a Sweeper.
func NewSweeper(repo Repository, svc *Service, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{repo: repo, svc: svc, logger: logger}
}

// SweepResult is what one tick did.
type SweepResult struct {
	RowsDeleted    int
	ObjectsDeleted int
}

// Sweep removes one batch of expired, unclaimed uploads and their objects.
//
// Rows first, objects after — and deliberately in that order. The row is
// deleted by ClaimExpired in a single DELETE ... RETURNING, so two
// replicas cannot both act on the same upload; whichever wins gets the
// rows back and destroys the objects. The reverse order would let a
// crash between the two leave a row pointing at an object that is already
// gone, which looks to a buyer like their photo vanished rather than
// their upload expiring.
func (s *Sweeper) Sweep(ctx context.Context) (SweepResult, error) {
	var out SweepResult
	if s == nil || s.svc == nil || !s.svc.Enabled() {
		// Nothing to sweep into: without a bucket there are no objects,
		// and deleting the rows would strand nothing but also achieve
		// nothing.
		return out, nil
	}

	rows, err := s.repo.ClaimExpired(ctx, time.Now(), SweepBatch)
	if err != nil {
		return out, err
	}
	out.RowsDeleted = len(rows)

	for i := range rows {
		up := rows[i]
		before := out.ObjectsDeleted
		s.svc.destroyBoth(ctx, &up)
		// destroyBoth logs its own failures; count what we attempted so
		// the tick's log line is honest about volume rather than silent.
		out.ObjectsDeleted = before + 1
	}

	if out.RowsDeleted > 0 {
		s.logger.Info("personalisationupload: swept expired uploads",
			"rows", out.RowsDeleted, "objects", out.ObjectsDeleted)
	}
	return out, nil
}
