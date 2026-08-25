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
// A row with no Response is ALSO a miss, not an error: Reserve creates such
// a row to claim a key before the work behind it has finished, and its
// caller has not completed yet. Treating it as a hit would replay an empty
// body to a second caller instead of telling them the truth — that a
// request with this key is still in flight.
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
	if len(row.Response) == 0 {
		return nil, false, nil
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

// Reserve claims key for the caller. It returns claimed=true when this
// caller won the race and should do the work, false when another caller
// already holds it — in which case the winner's response is the one that
// will be replayed, and this caller must not execute.
//
// This is what makes the guarantee hold across pods: Lookup-then-Save is
// check-then-act, and two pods can both miss before either saves. Reserve
// closes that gap with an ON CONFLICT DO NOTHING insert of a row with no
// Response — the row's mere existence is the claim, and Complete fills in
// the body once the work is done. The tenantID passed here need not be the
// final subscription's own tenant when that is not yet known at claim time
// (the trial-extend handler passes the store id it already has); it exists
// to satisfy idempotency_keys.tenant_id's NOT NULL constraint for purge
// scoping, not to be authoritative — Complete does not revise it.
func Reserve(ctx context.Context, db *gorm.DB, key, tenantID string, now time.Time, ttl time.Duration) (bool, error) {
	row := IdempotencyKey{
		Key:       key,
		TenantID:  tenantID,
		CreatedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(ttl),
	}
	res := db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).
		Create(&row)
	if res.Error != nil {
		return false, fmt.Errorf("idempotency: reserve: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// Complete stores the response body against an already-reserved key,
// turning a claimed-but-empty row into a replayable one.
func Complete(ctx context.Context, db *gorm.DB, key string, body json.RawMessage) error {
	err := db.WithContext(ctx).Model(&IdempotencyKey{}).
		Where("key = ?", key).
		Update("response", []byte(body)).Error
	if err != nil {
		return fmt.Errorf("idempotency: complete: %w", err)
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
