package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProAppCancelledEvent is the shape consumed when subscription emits
// subscription.pro_app_cancelled — either the graceful 60-day teardown
// path or the merchant-initiated immediate pull (spec §13.5 + §15.5).
type ProAppCancelledEvent struct {
	TenantID                   uuid.UUID
	StoreID                    uuid.UUID
	AppleAppID                 string
	GooglePackage              string
	FirebaseProjectID          string
	MerchantInitiatedImmediate bool
}

// ProAppCancelledConsumer seeds a white_label_app_state row when the
// subscription package emits the pro_app_cancelled event.
//
// Graceful path: scheduled_at = now; advancer sees day-7 tick first,
// then day-30, day-60, day-90.
//
// Merchant-initiated immediate: scheduled_at = now − 53 days so the
// advancer's day-30 step fires immediately and the entire sequence
// compresses into ~7 days of real time (§15.5 "immediate-pull compresses
// to 7 days").
type ProAppCancelledConsumer struct {
	db    *gorm.DB
	clock Clock
}

// NewProAppCancelledConsumer wires the consumer. Clock defaults to
// time.Now when nil.
func NewProAppCancelledConsumer(db *gorm.DB, clock Clock) *ProAppCancelledConsumer {
	if clock == nil {
		clock = time.Now
	}
	return &ProAppCancelledConsumer{db: db, clock: clock}
}

// Handle inserts (or updates, if one already exists) a state row for
// the store. Idempotent under replay: a second Handle call for the
// same storeID is a no-op (ON CONFLICT DO NOTHING via unique index on
// store_id).
func (c *ProAppCancelledConsumer) Handle(ctx context.Context, ev ProAppCancelledEvent) error {
	if ev.TenantID == uuid.Nil || ev.StoreID == uuid.Nil {
		return errors.New("lifecycle/consumer: TenantID and StoreID are required")
	}

	now := c.clock().UTC()
	scheduledAt := now
	if ev.MerchantInitiatedImmediate {
		// 53-day backdating: day 7 tick is already past (53 > 7), and
		// day 30 becomes immediately due. Day 60/90 land at real-time
		// +7/+37 days from now. Matches spec §15.5 "≤7 days from
		// initiation to downloads blocked".
		scheduledAt = now.Add(-53 * 24 * time.Hour)
	}

	// Next action is "day 7 banner tick" from scheduled_at — for the
	// immediate path that's now-46d so clearly overdue, advancer picks
	// it up on next tick. For graceful, it's now+7d.
	next := scheduledAt.Add(7 * 24 * time.Hour)

	row := Row{
		ID:                uuid.New(),
		TenantID:          ev.TenantID,
		StoreID:           ev.StoreID,
		Status:            StatusSunsetScheduled,
		ScheduledAt:       &scheduledAt,
		NextActionAt:      &next,
		AppleAppID:        ev.AppleAppID,
		GooglePackage:     ev.GooglePackage,
		FirebaseProjectID: ev.FirebaseProjectID,
		MerchantInitiated: ev.MerchantInitiatedImmediate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// INSERT ... ON CONFLICT (store_id) DO NOTHING via raw SQL because
	// GORM's generic upsert on a UUID PK + secondary UNIQUE isn't the
	// shape we want here (we want DO NOTHING, not DO UPDATE).
	res := c.db.WithContext(ctx).Exec(`
		INSERT INTO white_label_app_state (
			id, tenant_id, store_id, status,
			scheduled_at, next_action_at,
			apple_app_id, google_package, firebase_project_id,
			merchant_initiated, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (store_id) DO NOTHING
	`,
		row.ID, row.TenantID, row.StoreID, string(row.Status),
		scheduledAt, next,
		row.AppleAppID, row.GooglePackage, row.FirebaseProjectID,
		row.MerchantInitiated, now, now)
	if res.Error != nil {
		return fmt.Errorf("lifecycle/consumer: insert: %w", res.Error)
	}
	return nil
}
