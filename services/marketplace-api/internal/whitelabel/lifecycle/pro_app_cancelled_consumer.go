package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
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
	db        *gorm.DB
	clock     Clock
	discovery *Discovery
	logger    *slog.Logger
}

// NewProAppCancelledConsumer wires the consumer. Clock defaults to
// time.Now when nil.
func NewProAppCancelledConsumer(db *gorm.DB, clock Clock) *ProAppCancelledConsumer {
	if clock == nil {
		clock = time.Now
	}
	return &ProAppCancelledConsumer{db: db, clock: clock, logger: slog.Default()}
}

// WithDiscovery returns a copy of the consumer that can resolve
// identifiers for itself, enabling ProAppCancelled. Handle is unaffected
// — a caller that already knows the identifiers still uses it directly.
func (c *ProAppCancelledConsumer) WithDiscovery(d *Discovery) *ProAppCancelledConsumer {
	next := *c
	next.discovery = d
	return &next
}

// WithLogger returns a copy of the consumer logging to l.
func (c *ProAppCancelledConsumer) WithLogger(l *slog.Logger) *ProAppCancelledConsumer {
	if l == nil {
		return c
	}
	next := *c
	next.logger = l
	return &next
}

// ProAppCancelled discovers the store's app identifiers and seeds the
// teardown row. It is the method the subscription finalize cron calls
// through the small notifier interface that package defines — an
// in-process call, deliberately not pub/sub: the emitter and the
// consumer run in the same binary, and a topic would add a delivery
// failure mode without adding a capability.
//
// Discovery runs here, at cancellation, because the advancer purges the
// credentials it needs at day 90 (see Discovery).
func (c *ProAppCancelledConsumer) ProAppCancelled(ctx context.Context, tenantID, storeID uuid.UUID) error {
	if c.discovery == nil {
		return ErrDiscoveryNotConfigured
	}
	ev, err := c.discovery.discover(ctx, tenantID, storeID)
	if err != nil {
		return err
	}
	// The cron path is the graceful 60-day teardown. The compressed
	// merchant-initiated path (§15.5) enters through Handle directly.
	ev.MerchantInitiatedImmediate = false
	return c.Handle(ctx, ev)
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
	if res.RowsAffected == 0 {
		// ON CONFLICT DO NOTHING fired: a row already exists for this
		// store. Replay, not a new teardown — do not append a second
		// coverage note to the append-only log.
		return nil
	}

	coverage := teardownCoverage(ev)
	// Say at seed time what this teardown will and will not reach. The
	// log line is for whoever is watching the cancellation; the
	// white_label_app_lifecycle row below is the durable half, still
	// readable in ninety days when the state row has advanced to
	// credentials_purged and no longer shows how it got there.
	c.log().InfoContext(ctx, "lifecycle/consumer: teardown seeded",
		"store_id", ev.StoreID, "tenant_id", ev.TenantID, "coverage", coverage)
	if err := c.appendCoverageNote(ctx, ev, now, coverage); err != nil {
		// The state row is committed and the teardown will run; failing
		// the whole Handle here would make the caller retry a seed that
		// already succeeded. Log loudly instead.
		c.log().ErrorContext(ctx, "lifecycle/consumer: teardown seeded but coverage note not recorded",
			"store_id", ev.StoreID, "err", err)
	}
	return nil
}

// seedActor is the actor recorded on the seed-time coverage note.
const seedActor = "system:consumer:pro_app_cancelled"

// appendCoverageNote writes one row into the append-only
// white_label_app_lifecycle log stating what this teardown covers.
func (c *ProAppCancelledConsumer) appendCoverageNote(ctx context.Context, ev ProAppCancelledEvent, now time.Time, coverage string) error {
	scheduled := now
	entry := subscription.WhiteLabelAppLifecycleEntry{
		ID:          uuid.New(),
		StoreID:     ev.StoreID,
		TenantID:    ev.TenantID,
		Status:      StatusSunsetScheduled,
		ScheduledAt: &scheduled,
		Actor:       seedActor,
		Reason:      &coverage,
	}
	if err := c.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("lifecycle/consumer: append coverage note: %w", err)
	}
	return nil
}

// teardownCoverage renders, per surface, what this row will and will not
// retire.
//
// WHY THIS EXISTS RATHER THAN A PLACEHOLDER IDENTIFIER: the advancer
// guards both of its Google calls with `if r.GooglePackage != ""`
// (advancer.go), and GooglePackage is empty by design — there is no
// source for it. So Play is never attempted, never errors, and never
// logs: the row would advance all the way to credentials_purged having
// said nothing whatsoever about Google, which is the same "we report a
// teardown we never performed" failure this whole path exists to prevent,
// one level down.
//
// TWO SEPARATE REASONS, both stated in the note. The Play client is NOT a
// stub any more — day 30 halts the production track for real — so the
// missing package identifier is the only thing stopping day 30. Day 60 is
// stopped by something no code can lift: unpublishing a Play listing has
// no Android Publisher API (googleplay.ErrUnpublishNotSupported) and
// needs a human in the Play Console. A note that named only the stub
// would be false today AND would hide the permanent half.
//
// Filling GooglePackage with a placeholder to make the guard fire would
// be worse: it trades a silent skip for a row asserting an identifier
// nobody has. So the absence is stated in words, durably, instead.
func teardownCoverage(ev ProAppCancelledEvent) string {
	parts := make([]string, 0, 3)

	if ev.AppleAppID != "" {
		parts = append(parts, fmt.Sprintf("apple=will_attempt(app_id=%s)", ev.AppleAppID))
	} else {
		parts = append(parts, "apple=not_attempted(no App Store Connect app id was discovered)")
	}

	if ev.GooglePackage != "" {
		// Day 30 only. Naming the halt and the un-pullable listing here is
		// what stops a reader in ninety days from taking "will_attempt" for
		// "the listing came down".
		parts = append(parts, fmt.Sprintf(
			"google_play=will_attempt(package=%s, day-30 download halt only; day-60 unpublish has no "+
				"Android Publisher API and requires a manual Play Console action)", ev.GooglePackage))
	} else {
		parts = append(parts, "google_play=NOT_ATTEMPTED(no package identifier is discoverable at cancel time, "+
			"so the advancer's day-30 and day-60 Google calls, both guarded on google_package, "+
			"will not run for this row; and even with a package, day 60 could never complete — "+
			"unpublishing a Play listing has no Android Publisher API and requires a manual "+
			"Play Console action — the merchant's Play listing, if any, stays live)")
	}

	if ev.FirebaseProjectID != "" {
		parts = append(parts, fmt.Sprintf(
			"firebase=will_attempt(project=%s, inferred from the Google Play service account's project_id; "+
				"the Firebase client is a stub, so the attempt is logged rather than effective)",
			ev.FirebaseProjectID))
	} else {
		parts = append(parts, "firebase=not_attempted(no project id was discovered)")
	}

	return strings.Join(parts, "; ")
}

func (c *ProAppCancelledConsumer) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}
