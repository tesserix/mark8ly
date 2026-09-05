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

// ErrNoAppIdentifiers is returned when an event names neither an Apple
// app, a Google package, nor a Firebase project.
//
// Seeding such a row would be actively harmful rather than merely
// useless. The advancer skips pullApps when AppleAppID is empty and
// archiveFirebase when FirebaseProjectID is empty (advancer.go), so a
// row with all three blank walks the whole state machine to
// credentials_purged without touching a single store listing. The
// merchant's app stays live and downloadable while the platform records
// a completed teardown — a silent false success nobody goes looking for.
// A teardown that fails loudly at seed time is strictly better: it
// leaves the honest no-op we have today and surfaces the missing
// identifiers to whoever emitted the event.
//
// Only an event with none of the three is refused. A store may
// legitimately ship on Apple with no Firebase project (or any other
// partial combination), and those must still seed.
var ErrNoAppIdentifiers = errors.New("lifecycle/consumer: event carries no AppleAppID, GooglePackage or FirebaseProjectID")

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

// validateEvent rejects events that cannot produce a meaningful
// teardown. See ErrNoAppIdentifiers for why a blank-identifier event is
// worse than no event at all.
func validateEvent(ev ProAppCancelledEvent) error {
	if ev.TenantID == uuid.Nil || ev.StoreID == uuid.Nil {
		return errors.New("lifecycle/consumer: TenantID and StoreID are required")
	}
	if ev.AppleAppID == "" && ev.GooglePackage == "" && ev.FirebaseProjectID == "" {
		return ErrNoAppIdentifiers
	}
	return nil
}

// Handle inserts (or updates, if one already exists) a state row for
// the store. Idempotent under replay: a second Handle call for the
// same storeID is a no-op (ON CONFLICT DO NOTHING via unique index on
// store_id).
func (c *ProAppCancelledConsumer) Handle(ctx context.Context, ev ProAppCancelledEvent) error {
	// Validation runs before any SQL: a refused event must leave no row
	// behind for the advancer to pick up.
	if err := validateEvent(ev); err != nil {
		return err
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
