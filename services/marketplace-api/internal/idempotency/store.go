package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultTTL is how long a stored response stays replayable. Long enough to
// cover a client's retry budget, short enough that the table stays small.
const DefaultTTL = 24 * time.Hour

// Lookup returns a previously stored response for key.
//
// An EXPIRED row is a miss: expiry is a property of the row, not of whether
// the sweep happened to run recently, and replaying a response past its TTL
// would make the guarantee depend on cron timing.
//
// Expiry is judged against the DATABASE clock (`now()` in the query below),
// deliberately — every pod hitting this query agrees on what "now" means
// regardless of its own clock, which is what keeps the replay guarantee
// consistent across replicas. SweepExpired, below, takes an app-clock `now`
// instead; the two are not guaranteed to agree.
func Lookup(ctx context.Context, db *gorm.DB, key string) (json.RawMessage, bool, error) {
	var row IdempotencyKey
	err := db.WithContext(ctx).
		Where("key = ? AND expires_at > now()", key).
		First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("idempotency: lookup: %w", err)
	}
	return json.RawMessage(row.Response), true, nil
}

// Save stores a response under key.
//
// ON CONFLICT DO NOTHING, so the FIRST writer wins: two pods handling the
// same retry converge instead of one overwriting what the other already
// told a caller. A duplicate is therefore not an error.
func Save(ctx context.Context, db *gorm.DB, key, tenantID string, body json.RawMessage, now time.Time, ttl time.Duration) error {
	row := IdempotencyKey{
		Key:       key,
		TenantID:  tenantID,
		Response:  []byte(body),
		CreatedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(ttl),
	}
	err := db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("idempotency: save: %w", err)
	}
	return nil
}

// SweepExpired deletes rows past their expires_at, inclusive of the instant
// itself.
//
// NOTHING pruned this table before #286 — the comments in migration 000001
// and models.go claiming a nightly sweep were both wrong, and the only other
// references delete by tenant_id during tenant hard-delete and purge.
//
// now is a parameter (for deterministic tests), which makes it an APP-clock
// instant — unlike Lookup, which judges expiry against the database's own
// clock. Callers must keep the `now` they pass here strictly behind what
// Lookup would see, or a row Lookup would still honour could be deleted
// first (see platformadmin.sweepGrace, the production caller's margin).
func SweepExpired(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	res := db.WithContext(ctx).
		Where("expires_at <= ?", now.UTC()).
		Delete(&IdempotencyKey{})
	if res.Error != nil {
		return 0, fmt.Errorf("idempotency: sweep expired: %w", res.Error)
	}
	return res.RowsAffected, nil
}
