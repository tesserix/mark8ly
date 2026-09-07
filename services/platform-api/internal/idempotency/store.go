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
// An EXPIRED row is a miss: expiry is a property of the row, not of
// whether a sweep happened to run recently, and replaying a response past
// its TTL would make the guarantee depend on cron timing.
//
// A row with no Response is ALSO a miss, not an error: Reserve creates
// such a row to claim a key before the work behind it has finished, and
// its caller has not completed yet. Treating it as a hit would replay an
// empty body to a second caller instead of telling them the truth — that
// a request with this key is still in flight.
//
// Expiry is judged against the DATABASE clock (`now()` in the query
// below), deliberately — every pod hitting this query agrees on what "now"
// means regardless of its own clock. SweepExpired, below, takes an
// app-clock `now` instead; the two are not guaranteed to agree.
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
	return row.Response, true, nil
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
// the body once the work is done.
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

// Release drops a reservation whose work did not complete, so a corrected
// retry with the same key is not blocked until the TTL expires.
//
// Only ever call this for a key THIS caller reserved and did not Complete.
// A reservation that outlives its failed attempt turns a mistyped request
// into a key that answers 409 in_progress for a day.
//
// Scoped to key AND an empty Response, so this can never delete a
// completed row that some other caller is entitled to replay.
func Release(ctx context.Context, db *gorm.DB, key string) error {
	err := db.WithContext(ctx).
		Where("key = ? AND response IS NULL", key).
		Delete(&IdempotencyKey{}).Error
	if err != nil {
		return fmt.Errorf("idempotency: release: %w", err)
	}
	return nil
}

// SweepExpired deletes rows past their expires_at, inclusive of the
// instant itself. Not wired to a cron in this service yet — see the
// package doc comment — but kept alongside Reserve/Lookup/Complete/Release
// so wiring one later is not a second copy of this file.
func SweepExpired(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	res := db.WithContext(ctx).
		Where("expires_at <= ?", now.UTC()).
		Delete(&IdempotencyKey{})
	if res.Error != nil {
		return 0, fmt.Errorf("idempotency: sweep expired: %w", res.Error)
	}
	return res.RowsAffected, nil
}
