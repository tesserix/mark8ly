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

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/firebase"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
	wlmetrics "github.com/mark8ly/marketplace-api/internal/whitelabel/metrics"
)

// Clock returns the current wall time. Injectable so tests can
// deterministically age rows.
type Clock func() time.Time

// Config groups Advancer dependencies. All fields required in
// production; tests inject fakes for Apple/Google/Firebase.
//
// Apple and Google are FACTORIES, not clients: their credentials are
// per-tenant and the advancer walks a multi-tenant cohort, so a client is
// resolved per row. See AppleTeardownFactory for why one shared client
// cannot work. Firebase is a plain client — its stub takes no per-tenant
// configuration.
type Config struct {
	DB       *gorm.DB
	Apple    AppleTeardownFactory
	Google   GoogleTeardownFactory
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
	apple    AppleTeardownFactory
	google   GoogleTeardownFactory
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
				r.ScheduledAt.Add(30*24*time.Hour))
		}
		// Day 30 — block downloads.
		if err := a.blockDownloads(ctx, r); err != nil {
			return err
		}
		return a.transition(ctx, r, StatusDownloadsBlocked,
			r.ScheduledAt.Add(60*24*time.Hour))

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
			r.ScheduledAt.Add(90*24*time.Hour))

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
// Idempotent — Apple's PATCH is safe to re-apply, and Play's edit
// lifecycle short-circuits when the production track is already halted.
//
// An Apple failure stalls the row (see appleClient); a Play failure is
// tolerated but WRITTEN DOWN, because the row is about to claim
// downloads_blocked and that status cannot say "Apple only".
func (a *Advancer) blockDownloads(ctx context.Context, r Row) error {
	if r.AppleAppID != "" {
		cli, err := a.appleClient(ctx, r)
		if err != nil {
			return err
		}
		if err := cli.BlockDownloads(ctx, r.AppleAppID); err != nil {
			return fmt.Errorf("apple.BlockDownloads(%s): %w", r.AppleAppID, err)
		}
	}
	if r.GooglePackage != "" {
		cli, err := a.googleClient(ctx, r)
		if err != nil {
			a.recordGoogleSkip(ctx, r, StatusDownloadsBlocked, stepBlockDownloads, err)
			return nil
		}
		if err := cli.BlockDownloads(ctx, r.GooglePackage); err != nil {
			a.recordGoogleSkip(ctx, r, StatusDownloadsBlocked, stepBlockDownloads, err)
		}
	}
	return nil
}

// Teardown step identifiers. They are Prometheus label values as well as
// note wording (recordGoogleSkip renders underscores as spaces), so they
// must stay a small closed set — see wlmetrics.LifecycleStepSkipped.
const (
	stepBlockDownloads      = "block_downloads"
	stepBlockDownloadsRetry = "block_downloads_day60_retry"
	stepPullApp             = "pull_app"
	stepArchiveProject      = "archive_project"
)

// pullApps removes the public listings at day 60.
//
// PLAY CANNOT DO THIS, EVER. Unpublishing a Play listing has no Android
// Publisher API (googleplay.ErrUnpublishNotSupported), so unlike day 30
// this is not a failure a later tick can turn into a success. The row
// still advances — stalling would block the day-90 credential purge on
// work no retry can complete — so the refusal is recorded beside the
// status, exactly as archiveFirebase does for the Firebase stub. Without
// that note the `pulled` status would be the only durable record of the
// step, asserting a takedown that did not happen.
//
// The day-30 halt IS retryable, and is re-attempted here before the pull.
// See the comment on that call.
func (a *Advancer) pullApps(ctx context.Context, r Row) error {
	if r.AppleAppID != "" {
		cli, err := a.appleClient(ctx, r)
		if err != nil {
			return err
		}
		if err := cli.PullApp(ctx, r.AppleAppID); err != nil {
			return fmt.Errorf("apple.PullApp(%s): %w", r.AppleAppID, err)
		}
	}
	if r.GooglePackage != "" {
		cli, err := a.googleClient(ctx, r)
		if err != nil {
			a.recordGoogleSkip(ctx, r, StatusPulled, stepPullApp, err)
			return nil
		}
		// RE-ATTEMPT THE DAY-30 HALT FIRST. Day 30 advances the row
		// whether or not Play was reached, so a transient failure there —
		// a 503, a momentarily unreachable Google — would otherwise leave
		// the app serving forever with no second chance. BlockDownloads
		// is idempotent (an already-halted production track short-circuits
		// without committing an edit), so this costs one GET when day 30
		// succeeded, and converts "permanently unhalted" into "halted one
		// tick late" when it did not.
		//
		// It is deliberately not a substitute for the pull below: halting
		// stops new installs, it does not remove the listing.
		if err := cli.BlockDownloads(ctx, r.GooglePackage); err != nil {
			a.recordGoogleSkip(ctx, r, StatusPulled, stepBlockDownloadsRetry, err)
		}
		if err := cli.PullApp(ctx, r.GooglePackage); err != nil {
			a.recordGoogleSkip(ctx, r, StatusPulled, stepPullApp, err)
		}
	}
	return nil
}

// recordGoogleSkip writes down Play work the advancer did not do, next to
// the status that is about to imply it did.
//
// The status cannot carry the caveat: it is a fixed value the state
// machine reads to reach the next step, and erroring instead would stall
// the row — permanently, in the day-60 case, since no retry can unpublish
// a listing. So the truth goes BESIDE the status, in the same append-only
// lifecycle table a reader already consults. This is the reasoning
// archiveFirebase applies to the Firebase stub; tesserix-home#702's whole
// subject is a system reporting work it did not do.
//
// The note write is deliberately non-fatal: failing the advance on a
// bookkeeping insert would stall the row, and the WARN has already gone
// out.
func (a *Advancer) recordGoogleSkip(ctx context.Context, r Row, next Status, step string, cause error) {
	// The counter is the only thing an alert can see. LifecycleTransition
	// increments identically whether or not Play was reached, and a reason
	// string inside a database row is not a signal.
	wlmetrics.LifecycleStepSkipped.WithLabelValues("google_play", step).Inc()

	a.logger.WarnContext(ctx, "lifecycle: google step skipped",
		"store_id", r.StoreID, "package", r.GooglePackage, "step", step, "err", cause)

	reason := fmt.Sprintf("google play %s NOT performed for package %s: %v",
		strings.ReplaceAll(step, "_", " "), r.GooglePackage, cause)
	if errors.Is(cause, googleplay.ErrUnpublishNotSupported) {
		// Say what an operator has to DO. A reason that only reports the
		// API gap reads as a bug to be fixed in code, and this one cannot
		// be: the listing stays public until a human unpublishes it.
		reason += " (still public; requires a manual Play Console unpublish)"
	}
	if noteErr := a.appendNote(ctx, r, next, reason); noteErr != nil {
		a.logger.WarnContext(ctx, "lifecycle: could not record google skip",
			"store_id", r.StoreID, "err", noteErr)
	}
}

// appleClient resolves the App Store Connect client for one row's tenant.
//
// A resolution failure — no factory wired, credentials revoked, the store's
// ASC key already purged — is returned as an ERROR, which stalls the row:
// advanceOne aborts, no status is written and no lifecycle log row is
// appended, so next_action_at stays due and the step retries next tick.
//
// It is deliberately NOT "log it and mark the step done". A row whose
// merchant we cannot authenticate as has not had its listing blocked or
// pulled; advancing it would write downloads_blocked or pulled into the
// append-only audit table asserting a teardown that never happened, which
// is the exact failure #702 exists to fix. A row stuck retrying with an
// ERROR log every tick is a visible, truthful state; a row that walked to
// credentials_purged having touched nothing is not.
func (a *Advancer) appleClient(ctx context.Context, r Row) (AppleTeardownClient, error) {
	if a.apple == nil {
		return nil, fmt.Errorf("lifecycle: no Apple client factory configured (store %s carries apple app id %s)",
			r.StoreID, r.AppleAppID)
	}
	cli, err := a.apple(ctx, r.TenantID, r.StoreID)
	if err != nil {
		return nil, fmt.Errorf("lifecycle: apple client for store %s: %w", r.StoreID, err)
	}
	if cli == nil {
		return nil, fmt.Errorf("lifecycle: apple client factory returned no client for store %s", r.StoreID)
	}
	return cli, nil
}

// googleClient resolves the Play client for one row's tenant.
//
// Unlike appleClient, a failure here is tolerated by the callers: Play
// teardown is only partly possible — day 30 works, day 60 has no API at
// all. The asymmetry is deliberate: Apple stalls, Play warns AND records
// what it did not do (recordGoogleSkip).
//
// THIS PATH IS REACHABLE SINCE #872. It previously could not run at all —
// rows carried no GooglePackage because nothing produced one (#702
// decision 4) — and this comment said so. The optional `package_name` on
// the Play credential upload is now that source, so a row belonging to a
// merchant who supplied one reaches here for real. Rows without a package
// still skip the guard in the callers and never arrive.
func (a *Advancer) googleClient(ctx context.Context, r Row) (googleplay.ClientAPI, error) {
	if a.google == nil {
		return nil, fmt.Errorf("lifecycle: no Google client factory configured (store %s carries package %s)",
			r.StoreID, r.GooglePackage)
	}
	cli, err := a.google(ctx, r.TenantID, r.StoreID)
	if err != nil {
		return nil, fmt.Errorf("lifecycle: google client for store %s: %w", r.StoreID, err)
	}
	if cli == nil {
		return nil, fmt.Errorf("lifecycle: google client factory returned no client for store %s", r.StoreID)
	}
	return cli, nil
}

func (a *Advancer) archiveFirebase(ctx context.Context, r Row) error {
	if r.FirebaseProjectID == "" {
		return nil // no firebase project on this store — skip
	}
	if err := a.firebase.ArchiveProject(ctx, r.FirebaseProjectID); err != nil {
		a.logger.WarnContext(ctx, "lifecycle: firebase archive skipped",
			"store_id", r.StoreID, "err", err)
		// The row is about to transition to `firebase_archived`, and that
		// status would otherwise be the ONLY durable record of this step —
		// asserting an archive that did not happen. The status cannot say
		// so: it is a fixed value the state machine reads to reach day 90,
		// and erroring here instead would stall every row forever, because
		// the Firebase client is an unimplemented stub that always returns
		// ErrNotWired. So the truth goes beside the status rather than in
		// it. tesserix-home#702's whole subject is a system reporting work
		// it did not do; a status this code KNOWS is untrue must not be
		// left as the only thing written down.
		wlmetrics.LifecycleStepSkipped.WithLabelValues("firebase", stepArchiveProject).Inc()
		if noteErr := a.appendNote(ctx, r, StatusFirebaseArchived,
			fmt.Sprintf("firebase archive NOT performed for project %s: %v", r.FirebaseProjectID, err),
		); noteErr != nil {
			// Deliberately not fatal: failing the advance here would stall
			// the row on a bookkeeping write, and the WARN above has
			// already been emitted.
			a.logger.WarnContext(ctx, "lifecycle: could not record firebase skip",
				"store_id", r.StoreID, "err", noteErr)
		}
	}
	return nil
}

// appendNote writes a lifecycle row carrying a REASON rather than marking a
// transition. Same append-only table as appendLog, deliberately: a reader
// asking "what happened to this store" gets one ordered history, not a
// status trail plus a separate place the caveats live.
func (a *Advancer) appendNote(ctx context.Context, r Row, status Status, reason string) error {
	now := a.clock()
	entry := subscription.WhiteLabelAppLifecycleEntry{
		ID:          uuid.New(),
		StoreID:     r.StoreID,
		TenantID:    r.TenantID,
		Status:      status,
		ScheduledAt: &now,
		Actor:       "system:cron:lifecycle",
		Reason:      &reason,
	}
	if err := a.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("lifecycle: append note: %w", err)
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
