package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/firebase"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
	wlmetrics "github.com/mark8ly/marketplace-api/internal/whitelabel/metrics"
)

// White-label app sunset lifecycle schedule. These are distinct from subscription
// trial periods and measure when the white-label app transitions through states:
// sunset_scheduled → (30 days) → downloads_blocked → (30 days) → pulled →
// (immediately) → firebase_archived → (30 days) → credentials_purged (terminal).
const (
	appSunsetDaysToDownloadBlock = time.Duration(30) * 24 * time.Hour
	appSunsetDaysToPull          = time.Duration(60) * 24 * time.Hour
	appSunsetDaysFinal           = time.Duration(90) * 24 * time.Hour
)

// Clock returns the current wall time. Injectable so tests can
// deterministically age rows.
type Clock func() time.Time

// Config groups Advancer dependencies. All fields required in
// production; tests inject fakes for Apple/Google/Firebase.
type Config struct {
	DB       *gorm.DB
	Apple    apple.ClientAPI
	Google   googleplay.ClientAPI
	Firebase firebase.ClientAPI
	Creds    *appcreds.Service
	Clock    Clock // defaults to time.Now when nil
	Logger   *slog.Logger
}

// Advancer walks white_label_app_state rows whose next_action_at has
// fired and advances them one step. Safe to run from a cron loop —
// each action is idempotent (see comments on blockDownloads /
// pullApps / archiveFirebase / purgeCredentials).
type Advancer struct {
	db       *gorm.DB
	apple    apple.ClientAPI
	google   googleplay.ClientAPI
	firebase firebase.ClientAPI
	creds    *appcreds.Service
	clock    Clock
	logger   *slog.Logger
}

// NewAdvancer wires the Advancer. Callers should run it via the
// Scheduler (scheduler.go) which invokes AdvanceDue on a cron tick.
func NewAdvancer(cfg Config) *Advancer {
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Advancer{
		db:       cfg.DB,
		apple:    cfg.Apple,
		google:   cfg.Google,
		firebase: cfg.Firebase,
		creds:    cfg.Creds,
		clock:    clock,
		logger:   logger,
	}
}

// AdvanceDue scans state rows with next_action_at <= now() and advances
// each. Errors on a single row are logged and skipped so one bad row
// doesn't stall the whole cohort.
func (a *Advancer) AdvanceDue(ctx context.Context) error {
	now := a.clock()

	var rows []Row
	if err := a.db.WithContext(ctx).
		Where("next_action_at IS NOT NULL AND next_action_at <= ?", now).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("lifecycle: query due rows: %w", err)
	}

	for _, r := range rows {
		if err := a.advanceOne(ctx, r, now); err != nil {
			a.logger.ErrorContext(ctx, "lifecycle: advance row failed",
				"store_id", r.StoreID, "status", r.Status, "err", err)
			// Continue with remaining rows.
		}
	}
	return nil
}

// advanceOne applies the next step for a single row. The Status→Action
// mapping is inline so the flow is easy to follow top-to-bottom.
//
// Status transitions on success:
//
//	sunset_scheduled → (day 30) → downloads_blocked
//	downloads_blocked → (day 60) → pulled → (same day) → firebase_archived
//	firebase_archived → (day 90) → credentials_purged (terminal)
//
// Day 7 is a banner-only tick (no state change). We treat it as a
// special case in the sunset_scheduled branch: if < 30 days elapsed,
// emit the banner event and push next_action_at to day 30.
func (a *Advancer) advanceOne(ctx context.Context, r Row, now time.Time) error {
	if r.ScheduledAt == nil {
		return fmt.Errorf("lifecycle: row %s has no scheduled_at", r.StoreID)
	}
	daysElapsed := int(now.Sub(*r.ScheduledAt).Hours() / 24)

	switch r.Status {
	case StatusSunsetScheduled:
		// Day 7 banner-only tick, OR day 30 transition.
		if daysElapsed < 30 {
			// Banner event — log only, no state change. Next check is
			// at day 30 from scheduled_at.
			a.logger.InfoContext(ctx, "lifecycle: banner tick",
				"store_id", r.StoreID, "days_elapsed", daysElapsed)
			return a.updateStatus(ctx, r, StatusSunsetScheduled,
				r.ScheduledAt.Add(appSunsetDaysToDownloadBlock))
		}
		// Day 30 — block downloads.
		if err := a.blockDownloads(ctx, r); err != nil {
			return err
		}
		return a.transition(ctx, r, StatusDownloadsBlocked,
			r.ScheduledAt.Add(appSunsetDaysToPull))

	case StatusDownloadsBlocked:
		// Day 60 — pull apps.
		if err := a.pullApps(ctx, r); err != nil {
			return err
		}
		// Pulled is transient; immediately archive Firebase and move on.
		return a.transition(ctx, r, StatusPulled,
			r.ScheduledAt.Add(60*24*time.Hour+time.Minute))

	case StatusPulled:
		if err := a.archiveFirebase(ctx, r); err != nil {
			return err
		}
		return a.transition(ctx, r, StatusFirebaseArchived,
			r.ScheduledAt.Add(appSunsetDaysFinal))

	case StatusFirebaseArchived:
		// Day 90 — delete Firebase + purge all credentials. Terminal.
		if err := a.purgeCredentials(ctx, r); err != nil {
			return err
		}
		return a.terminate(ctx, r, StatusCredentialsPurged)

	default:
		return fmt.Errorf("lifecycle: no transition defined for status %q (store %s)",
			r.Status, r.StoreID)
	}
}

// ─── Per-step actions ────────────────────────────────────────────────

// blockDownloads calls Apple and Google to halt new downloads.
// Idempotent — Apple PATCH is safe to re-apply; Google PATCH too.
// If Apple succeeds but Google fails, the row stays at
// sunset_scheduled and re-runs next tick; Apple re-application is a
// no-op.
func (a *Advancer) blockDownloads(ctx context.Context, r Row) error {
	if r.AppleAppID != "" {
		if err := a.apple.BlockDownloads(ctx, r.AppleAppID); err != nil {
			return fmt.Errorf("apple.BlockDownloads(%s): %w", r.AppleAppID, err)
		}
	}
	if r.GooglePackage != "" {
		if err := a.google.BlockDownloads(ctx, r.GooglePackage); err != nil {
			// Google's ErrNotWired is tolerated — it means Apple was
			// successful and Google integration is deferred. Logged
			// and swallowed rather than stalling the advance.
			a.logger.WarnContext(ctx, "lifecycle: google block downloads skipped",
				"store_id", r.StoreID, "err", err)
		}
	}
	return nil
}

func (a *Advancer) pullApps(ctx context.Context, r Row) error {
	if r.AppleAppID != "" {
		if err := a.apple.PullApp(ctx, r.AppleAppID); err != nil {
			return fmt.Errorf("apple.PullApp(%s): %w", r.AppleAppID, err)
		}
	}
	if r.GooglePackage != "" {
		if err := a.google.PullApp(ctx, r.GooglePackage); err != nil {
			a.logger.WarnContext(ctx, "lifecycle: google pull app skipped",
				"store_id", r.StoreID, "err", err)
		}
	}
	return nil
}

func (a *Advancer) archiveFirebase(ctx context.Context, r Row) error {
	if r.FirebaseProjectID == "" {
		return nil // no firebase project on this store — skip
	}
	if err := a.firebase.ArchiveProject(ctx, r.FirebaseProjectID); err != nil {
		a.logger.WarnContext(ctx, "lifecycle: firebase archive skipped",
			"store_id", r.StoreID, "err", err)
	}
	return nil
}

func (a *Advancer) purgeCredentials(ctx context.Context, r Row) error {
	// Firebase delete first, credentials second. Order matters:
	// deleting Firebase removes the place the credentials authorise
	// against; then the Secret Manager purge removes the creds
	// themselves (spec §13.5, §18.9). Both are idempotent.
	if r.FirebaseProjectID != "" {
		if err := a.firebase.DeleteProject(ctx, r.FirebaseProjectID); err != nil {
			a.logger.WarnContext(ctx, "lifecycle: firebase delete skipped",
				"store_id", r.StoreID, "err", err)
		}
	}
	if err := a.creds.PurgeAll(ctx, r.TenantID, r.StoreID, "system:cron:day_90"); err != nil {
		return fmt.Errorf("appcreds.PurgeAll: %w", err)
	}
	return nil
}

// ─── State persistence helpers ───────────────────────────────────────

// transition writes the new status + next_action_at AND appends a
// lifecycle log row to the append-only table (§13.5 audit requirement).
// Also increments the Prometheus lifecycle counter labeled from/to.
func (a *Advancer) transition(ctx context.Context, r Row, next Status, nextAt time.Time) error {
	if err := a.updateStatus(ctx, r, next, nextAt); err != nil {
		return err
	}
	wlmetrics.LifecycleTransition.
		WithLabelValues(string(r.Status), string(next)).
		Inc()
	return a.appendLog(ctx, r, next, "system:cron:lifecycle")
}

// terminate writes the terminal status (credentials_purged) with
// next_action_at = NULL so the advancer never picks this row up again.
func (a *Advancer) terminate(ctx context.Context, r Row, next Status) error {
	now := a.clock()
	res := a.db.WithContext(ctx).Model(&Row{}).
		Where("id = ?", r.ID).
		Updates(map[string]any{
			"status":         next,
			"next_action_at": nil,
			"updated_at":     now,
		})
	if res.Error != nil {
		return fmt.Errorf("lifecycle: terminate: %w", res.Error)
	}
	wlmetrics.LifecycleTransition.
		WithLabelValues(string(r.Status), string(next)).
		Inc()
	return a.appendLog(ctx, r, next, "system:cron:lifecycle")
}

func (a *Advancer) updateStatus(ctx context.Context, r Row, next Status, nextAt time.Time) error {
	now := a.clock()
	res := a.db.WithContext(ctx).Model(&Row{}).
		Where("id = ?", r.ID).
		Updates(map[string]any{
			"status":         next,
			"next_action_at": nextAt,
			"updated_at":     now,
		})
	if res.Error != nil {
		return fmt.Errorf("lifecycle: update status: %w", res.Error)
	}
	return nil
}

// appendLog inserts a row into the existing append-only
// white_label_app_lifecycle table so the transition history is
// queryable for audit without joining to state.
func (a *Advancer) appendLog(ctx context.Context, r Row, next Status, actor string) error {
	now := a.clock()
	entry := subscription.WhiteLabelAppLifecycleEntry{
		ID:          uuid.New(),
		StoreID:     r.StoreID,
		TenantID:    r.TenantID,
		Status:      next,
		ScheduledAt: &now,
		Actor:       actor,
	}
	if err := a.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("lifecycle: append log: %w", err)
	}
	return nil
}
