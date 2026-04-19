package tax

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PauseThreshold — §5.2: cumulative outage > 72h within the active 14-day
// window pauses the deadline.
const PauseThreshold = 72 * time.Hour

// OutageKey identifies one unique outage episode. The same (store, country,
// registry, error_class) tuple identifies a single open row; BeginOutage is
// idempotent on it.
type OutageKey struct {
	StoreID    uuid.UUID
	TenantID   uuid.UUID
	Country    string
	Registry   string
	ErrorClass string
}

// ClockPauseTracker writes to tax_validation_outage_log and answers
// "is the 14-day deadline currently paused for this (store, country)?".
type ClockPauseTracker struct {
	db *gorm.DB
}

func NewClockPauseTracker(db *gorm.DB) *ClockPauseTracker {
	return &ClockPauseTracker{db: db}
}

// BeginOutage records a new open outage row if no open row already exists for
// the same (registry, error_class, store_id) tuple. Caller passes the wall
// clock at outage detection time.
func (t *ClockPauseTracker) BeginOutage(ctx context.Context, k OutageKey, at time.Time) error {
	if t == nil || t.db == nil {
		return nil
	}
	return t.db.WithContext(ctx).Exec(`
		INSERT INTO tax_validation_outage_log
			(country, registry, store_id, tenant_id, error_class, started_at)
		SELECT ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM tax_validation_outage_log
			 WHERE country=? AND registry=? AND error_class=?
			   AND (store_id = ? OR (store_id IS NULL AND ?::uuid IS NULL))
			   AND ended_at IS NULL
		)
	`,
		k.Country, k.Registry, nullableUUID(k.StoreID), nullableUUID(k.TenantID), k.ErrorClass, at,
		k.Country, k.Registry, k.ErrorClass,
		nullableUUID(k.StoreID), nullableUUID(k.StoreID),
	).Error
}

// EndOutage closes the most recent open row matching the key.
func (t *ClockPauseTracker) EndOutage(ctx context.Context, k OutageKey, at time.Time) error {
	if t == nil || t.db == nil {
		return nil
	}
	return t.db.WithContext(ctx).Exec(`
		UPDATE tax_validation_outage_log
		   SET ended_at         = ?,
		       seconds_observed = EXTRACT(EPOCH FROM (? - started_at))::INTEGER
		 WHERE id = (
			SELECT id FROM tax_validation_outage_log
			 WHERE country = ? AND registry = ?
			   AND (store_id = ? OR (store_id IS NULL AND ?::uuid IS NULL))
			   AND error_class = ?
			   AND ended_at IS NULL
			 ORDER BY started_at DESC
			 LIMIT 1
		 )
	`, at, at, k.Country, k.Registry, nullableUUID(k.StoreID), nullableUUID(k.StoreID), k.ErrorClass).Error
}

// IsPaused sums observed + still-open outage seconds for the (store, country)
// pair within the last 14 days; returns true when cumulative ≥ PauseThreshold.
func (t *ClockPauseTracker) IsPaused(ctx context.Context, storeID uuid.UUID, country string) (bool, error) {
	if t == nil || t.db == nil {
		return false, nil
	}
	var cumSeconds int64
	row := t.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(
			CASE
				WHEN ended_at IS NOT NULL THEN seconds_observed
				ELSE EXTRACT(EPOCH FROM (now() - started_at))::INTEGER
			END
		), 0)::BIGINT
		  FROM tax_validation_outage_log
		 WHERE country  = ?
		   AND store_id = ?
		   AND started_at > now() - INTERVAL '14 days'
	`, country, storeID).Row()
	if err := row.Scan(&cumSeconds); err != nil {
		return false, err
	}
	return time.Duration(cumSeconds)*time.Second >= PauseThreshold, nil
}

func nullableUUID(u uuid.UUID) any {
	if u == uuid.Nil {
		return nil
	}
	return u
}
