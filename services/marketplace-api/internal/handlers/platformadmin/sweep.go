package platformadmin

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/idempotency"
)

// SweepSpec is the cron expression for the expired nonce sweep — runs at
// 09:45 UTC daily, a free slot alongside the other daily crons (00:15
// expiry, 02:00 audit prune, 09:00 banner, 09:15 SCA, 09:30 trial).
const SweepSpec = "45 9 * * *"

// SweepExpiredNonces deletes platform_request_nonces rows past their
// expires_at. Safe by construction: expires_at is set to signedTS + window,
// exactly the instant a request stops being signature-valid (see
// RequirePlatformAuth), so a row can never be deleted while the request it
// guards is still replayable.
func SweepExpiredNonces(ctx context.Context, db *gorm.DB) (int64, error) {
	res := db.WithContext(ctx).
		Where("expires_at < now()").
		Delete(&Nonce{})
	if res.Error != nil {
		return 0, fmt.Errorf("platformadmin: sweep expired nonces: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// sweepGrace keeps the idempotency prune strictly behind Lookup's view of
// expiry. Lookup judges replay against the DATABASE clock (store.go); this
// sweep passes an app-clock instant. Without a margin, an app clock running
// ahead of the DB clock could delete a key Lookup would still honour, and a
// retry that should have replayed would silently re-execute the write
// instead. An hour is far beyond any plausible NTP skew and costs only a
// slightly later cleanup of rows that are already unreachable.
const sweepGrace = time.Hour

// SweepExpiredIdempotencyKeys deletes idempotency_keys rows that expired
// more than sweepGrace ago. It rides the same daily schedule as the nonce
// sweep because both tables exist only to serve this surface, and #286 is
// the first consumer this table has ever had.
func SweepExpiredIdempotencyKeys(ctx context.Context, db *gorm.DB) (int64, error) {
	return idempotency.SweepExpired(ctx, db, time.Now().UTC().Add(-sweepGrace))
}
