//go:build integration

package lifecycle_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/firebase"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/lifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ageRow seeds a Row with scheduled_at N days in the past and
// next_action_at = now so the advancer immediately picks it up.
func ageRow(t *testing.T, daysAgo int, status lifecycle.Status) lifecycle.Row {
	t.Helper()
	scheduledAt := time.Now().UTC().Add(-time.Duration(daysAgo) * 24 * time.Hour)
	next := time.Now().UTC().Add(-time.Minute) // due
	return lifecycle.Row{
		TenantID:          uuid.New(),
		StoreID:           uuid.New(),
		Status:            status,
		ScheduledAt:       &scheduledAt,
		NextActionAt:      &next,
		AppleAppID:        "apple-123",
		GooglePackage:     "com.example.store",
		FirebaseProjectID: "fb-proj-1",
	}
}

func newAdvancer(t *testing.T, fakes struct {
	Apple    *apple.FakeClient
	Google   *googleplay.FakeClient
	Firebase *firebase.FakeClient
	Creds    *appcreds.Service
}) *lifecycle.Advancer {
	t.Helper()
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	return lifecycle.NewAdvancer(lifecycle.Config{
		DB:       db,
		Apple:    staticAppleFactory(fakes.Apple),
		Google:   staticGoogleFactory(fakes.Google),
		Firebase: fakes.Firebase, Creds: fakes.Creds,
		Clock:  func() time.Time { return time.Now().UTC() },
		Logger: slog.Default(),
	})
}

// staticAppleFactory adapts a single fake to the per-tenant factory the
// advancer now takes. Production resolves a different client per row from
// that row's stored credentials; a test cohort is one tenant, so handing
// back the same fake preserves exactly the call-count assertions the
// pre-factory tests made.
func staticAppleFactory(c lifecycle.AppleTeardownClient) lifecycle.AppleTeardownFactory {
	return func(context.Context, uuid.UUID, uuid.UUID) (lifecycle.AppleTeardownClient, error) {
		return c, nil
	}
}

func staticGoogleFactory(c googleplay.ClientAPI) lifecycle.GoogleTeardownFactory {
	return func(context.Context, uuid.UUID, uuid.UUID) (googleplay.ClientAPI, error) {
		return c, nil
	}
}

// failingAppleFactory stands in for credentials that have been revoked,
// or a store whose ASC key is already gone.
func failingAppleFactory(err error) lifecycle.AppleTeardownFactory {
	return func(context.Context, uuid.UUID, uuid.UUID) (lifecycle.AppleTeardownClient, error) {
		return nil, err
	}
}

func newCredsSvc(t *testing.T) (*appcreds.Service, *appcreds.FakeSM) {
	t.Helper()
	fake := appcreds.NewFakeSM()
	// Auditing intentionally off for this test — the supported opt-out is
	// a nil *audit.Emitter, not a nil Repo (see audit.NewEmitter).
	var em *audit.Emitter
	return appcreds.NewService(appcreds.Config{
		ProjectID: "test-proj", SM: fake, Emitter: em,
	}), fake
}

func TestAdvancer_Day7_BannerOnly_NoStatusChange(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	// Access the same DB via testdb helper re-open — NewDB truncates
	// on cleanup so we only need the single handle.
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")

	row := ageRow(t, 7, lifecycle.StatusSunsetScheduled)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusSunsetScheduled, after.Status,
		"day 7 banner tick must not change status")
	require.Equal(t, 0, appleCli.BlockDownloadsCallCount,
		"day 7 must not block downloads")
	require.NotNil(t, after.NextActionAt)
}

func TestAdvancer_Day30_BlocksDownloads(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	row := ageRow(t, 30, lifecycle.StatusSunsetScheduled)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusDownloadsBlocked, after.Status)
	require.Equal(t, 1, appleCli.BlockDownloadsCallCount)
	// Google returns ErrNotWired but the advancer swallows it; the
	// fake records the attempt.
	require.Equal(t, 1, gpCli.BlockDownloadsCallCount)

	// Transition log row appended.
	var logCount int64
	db.Model(&subscription.WhiteLabelAppLifecycleEntry{}).
		Where("store_id=?", row.StoreID).
		Count(&logCount)
	require.GreaterOrEqual(t, logCount, int64(1))
}

func TestAdvancer_Day60_PullsAndArchives(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	row := ageRow(t, 60, lifecycle.StatusDownloadsBlocked)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusPulled, after.Status)
	require.Equal(t, 1, appleCli.PullAppCallCount)
}

func TestAdvancer_Day90_PurgesAllFourCredentials(t *testing.T) {
	creds, fakeSM := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	row := ageRow(t, 90, lifecycle.StatusFirebaseArchived)
	require.NoError(t, db.Create(&row).Error)

	// Pre-store all four credentials.
	for _, ct := range appcreds.AllCredTypes() {
		require.NoError(t, creds.Store(context.Background(), appcreds.StoreInput{
			TenantID: row.TenantID, StoreID: row.StoreID,
			CredType: ct, Payload: []byte("x"), Actor: "seed",
		}))
		name := appcreds.Path("test-proj", row.TenantID.String(), ct)
		require.True(t, fakeSM.Has(name), "cred %s seeded", ct)
	}

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusCredentialsPurged, after.Status)
	require.Nil(t, after.NextActionAt, "terminal status must clear next_action_at")
	require.Equal(t, 1, fbCli.DeleteProjectCallCount)

	// All four credentials gone.
	for _, ct := range appcreds.AllCredTypes() {
		name := appcreds.Path("test-proj", row.TenantID.String(), ct)
		require.False(t, fakeSM.Has(name), "cred %s must be purged", ct)
	}
}

func TestAdvancer_TerminalRow_NotRepicked(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	// next_action_at = NULL means "not due"; advancer must skip.
	row := lifecycle.Row{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Status:       lifecycle.StatusCredentialsPurged,
		ScheduledAt:  nil,
		NextActionAt: nil,
	}
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))
	require.Equal(t, 0, appleCli.BlockDownloadsCallCount)
	require.Equal(t, 0, fbCli.DeleteProjectCallCount)
}

// ─── Factory-resolution failures ─────────────────────────────────────

// newAdvancerWithFactories is newAdvancer with the client factories
// supplied directly, for the resolution-failure cases.
func newAdvancerWithFactories(
	t *testing.T,
	db *gorm.DB,
	appleF lifecycle.AppleTeardownFactory,
	googleF lifecycle.GoogleTeardownFactory,
	fbCli *firebase.FakeClient,
	creds *appcreds.Service,
) *lifecycle.Advancer {
	t.Helper()
	return lifecycle.NewAdvancer(lifecycle.Config{
		DB: db, Apple: appleF, Google: googleF,
		Firebase: fbCli, Creds: creds,
		Clock:  func() time.Time { return time.Now().UTC() },
		Logger: slog.Default(),
	})
}

// TestAdvancer_Day30_AppleClientUnresolvable_DoesNotAdvance is the point
// of #702 in one test: a row whose merchant we cannot authenticate as has
// NOT had its downloads blocked, so it must not be recorded as though it
// had. The status stays sunset_scheduled, next_action_at stays due (so the
// step retries), and no lifecycle log row is appended.
func TestAdvancer_Day30_AppleClientUnresolvable_DoesNotAdvance(t *testing.T) {
	creds, _ := newCredsSvc(t)
	fbCli := firebase.NewFakeClient()
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	adv := newAdvancerWithFactories(t, db,
		failingAppleFactory(errors.New("appcreds: apple-asc-api-key not found")),
		staticGoogleFactory(googleplay.NewFakeClient()), fbCli, creds)

	row := ageRow(t, 30, lifecycle.StatusSunsetScheduled)
	dueBefore := *row.NextActionAt
	require.NoError(t, db.Create(&row).Error)

	// AdvanceDue logs and skips the row; the cohort-level call succeeds.
	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusSunsetScheduled, after.Status,
		"an unreachable Apple account must not advance to downloads_blocked")
	require.NotNil(t, after.NextActionAt)
	require.WithinDuration(t, dueBefore, *after.NextActionAt, time.Second,
		"next_action_at must stay due so the step retries next tick")

	var logCount int64
	require.NoError(t, db.Model(&subscription.WhiteLabelAppLifecycleEntry{}).
		Where("store_id=?", row.StoreID).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount,
		"no audit row may claim a teardown step that never reached Apple")
}

// TestAdvancer_Day60_AppleClientUnresolvable_DoesNotAdvance is the same
// guarantee at the pull step, which is the more damaging one to fake: it
// writes `pulled` against a listing still live in the App Store.
func TestAdvancer_Day60_AppleClientUnresolvable_DoesNotAdvance(t *testing.T) {
	creds, _ := newCredsSvc(t)
	fbCli := firebase.NewFakeClient()
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	adv := newAdvancerWithFactories(t, db,
		failingAppleFactory(errors.New("appcreds: apple-asc-api-key not found")),
		staticGoogleFactory(googleplay.NewFakeClient()), fbCli, creds)

	row := ageRow(t, 60, lifecycle.StatusDownloadsBlocked)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusDownloadsBlocked, after.Status,
		"an unreachable Apple account must not advance to pulled")
	require.Equal(t, 0, fbCli.ArchiveProjectCalls,
		"the firebase archive that follows pulled must not run either")
}

// TestAdvancer_Day30_NilAppleFactory_DoesNotAdvance covers the wiring
// mistake rather than the runtime one: a Config with no Apple factory at
// all must stall the row, not treat "nothing to call" as success.
func TestAdvancer_Day30_NilAppleFactory_DoesNotAdvance(t *testing.T) {
	creds, _ := newCredsSvc(t)
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	adv := newAdvancerWithFactories(t, db, nil,
		staticGoogleFactory(googleplay.NewFakeClient()), firebase.NewFakeClient(), creds)

	row := ageRow(t, 30, lifecycle.StatusSunsetScheduled)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusSunsetScheduled, after.Status)
}

// TestAdvancer_Day30_GoogleClientUnresolvable_StillAdvances pins the
// deliberate asymmetry: Play teardown is deferred, so a Play failure is
// warned and swallowed, while the identical Apple failure stalls the row.
// If this ever starts stalling, Play has silently become load-bearing.
func TestAdvancer_Day30_GoogleClientUnresolvable_StillAdvances(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli := apple.NewFakeClient()
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	adv := newAdvancerWithFactories(t, db,
		staticAppleFactory(appleCli),
		func(context.Context, uuid.UUID, uuid.UUID) (googleplay.ClientAPI, error) {
			return nil, errors.New("appcreds: google-play-service-account not found")
		},
		firebase.NewFakeClient(), creds)

	row := ageRow(t, 30, lifecycle.StatusSunsetScheduled)
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusDownloadsBlocked, after.Status,
		"a Play failure is tolerated; Apple is what makes the step real")
	require.Equal(t, 1, appleCli.BlockDownloadsCallCount)
}

// A Firebase archive that did not happen must be written down, because the
// row is about to claim `firebase_archived` and that status cannot say
// otherwise — it is a fixed value the state machine reads to reach day 90.
// The real Firebase client is an unimplemented stub that always returns
// ErrNotWired, so without this the ONLY durable record of the step asserts
// an archive that never occurred. tesserix-home#702 is precisely about a
// system reporting work it did not do.
func TestAdvancer_FirebaseArchiveFailure_IsRecordedBesideTheStatus(t *testing.T) {
	creds, _ := newCredsSvc(t)
	appleCli, gpCli, fbCli := apple.NewFakeClient(), googleplay.NewFakeClient(), firebase.NewFakeClient()
	fbCli.ArchiveErr = firebase.ErrNotWired
	adv := newAdvancer(t, struct {
		Apple    *apple.FakeClient
		Google   *googleplay.FakeClient
		Firebase *firebase.FakeClient
		Creds    *appcreds.Service
	}{appleCli, gpCli, fbCli, creds})

	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	// StatusPulled, not DownloadsBlocked: day 60 transitions INTO Pulled,
	// and `archiveFirebase` runs on the Pulled case on the following tick.
	row := ageRow(t, 60, lifecycle.StatusPulled)
	row.FirebaseProjectID = "merchant-proj-1"
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, adv.AdvanceDue(context.Background()))

	// The row still advances — erroring here would stall every row forever,
	// since the stub never succeeds, and would block the day-90 purge.
	var after lifecycle.Row
	require.NoError(t, db.Where("id=?", row.ID).First(&after).Error)
	require.Equal(t, lifecycle.StatusFirebaseArchived, after.Status)

	// …but the failure is durable, with a reason naming the project.
	var reasons []string
	require.NoError(t, db.Raw(
		`SELECT reason FROM white_label_app_lifecycle
		  WHERE store_id = ? AND reason IS NOT NULL`, row.StoreID,
	).Scan(&reasons).Error)
	require.NotEmpty(t, reasons, "a skipped firebase archive must leave a reason behind")
	require.Contains(t, reasons[0], "NOT performed")
	require.Contains(t, reasons[0], "merchant-proj-1")
}
