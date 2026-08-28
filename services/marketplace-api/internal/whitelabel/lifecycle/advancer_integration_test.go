//go:build integration

package lifecycle_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
		DB: db, Apple: fakes.Apple, Google: fakes.Google,
		Firebase: fakes.Firebase, Creds: fakes.Creds,
		Clock:  func() time.Time { return time.Now().UTC() },
		Logger: slog.Default(),
	})
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
