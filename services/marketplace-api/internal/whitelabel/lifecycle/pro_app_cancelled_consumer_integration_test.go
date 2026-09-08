//go:build integration

package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/lifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestConsumer_GracefulPath_SeedsSunsetScheduled(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	fixedNow := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c := lifecycle.NewProAppCancelledConsumer(db, func() time.Time { return fixedNow })

	tenantID, storeID := uuid.New(), uuid.New()
	err := c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
		TenantID:   tenantID,
		StoreID:    storeID,
		AppleAppID: "a1", GooglePackage: "com.x", FirebaseProjectID: "fb-x",
		MerchantInitiatedImmediate: false,
	})
	require.NoError(t, err)

	var row lifecycle.Row
	require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
	require.Equal(t, lifecycle.StatusSunsetScheduled, row.Status)
	require.NotNil(t, row.ScheduledAt)
	require.WithinDuration(t, fixedNow, *row.ScheduledAt, time.Second)
	require.False(t, row.MerchantInitiated)

	// next_action_at = scheduled_at + 7d (day-7 banner tick)
	require.NotNil(t, row.NextActionAt)
	require.WithinDuration(t, fixedNow.Add(7*24*time.Hour), *row.NextActionAt, time.Second)
}

func TestConsumer_MerchantInitiatedImmediate_Backdates53Days(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	fixedNow := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c := lifecycle.NewProAppCancelledConsumer(db, func() time.Time { return fixedNow })

	tenantID, storeID := uuid.New(), uuid.New()
	err := c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
		TenantID: tenantID, StoreID: storeID,
		AppleAppID: "a1", MerchantInitiatedImmediate: true,
	})
	require.NoError(t, err)

	var row lifecycle.Row
	require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
	require.True(t, row.MerchantInitiated)

	// scheduled_at = now - 53 days
	expectedSched := fixedNow.Add(-53 * 24 * time.Hour)
	require.NotNil(t, row.ScheduledAt)
	require.WithinDuration(t, expectedSched, *row.ScheduledAt, time.Second)

	// next_action_at = scheduled_at + 7d = now - 46d (clearly overdue)
	require.NotNil(t, row.NextActionAt)
	require.True(t, row.NextActionAt.Before(fixedNow),
		"next_action_at must be in the past so advancer picks it up")
}

func TestConsumer_Replay_DoesNothing(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	tenantID, storeID := uuid.New(), uuid.New()
	ev := lifecycle.ProAppCancelledEvent{TenantID: tenantID, StoreID: storeID, AppleAppID: "a1"}

	// First Handle seeds the row.
	require.NoError(t, c.Handle(context.Background(), ev))

	// Second Handle must be a no-op — unique constraint on store_id.
	require.NoError(t, c.Handle(context.Background(), ev))

	var count int64
	db.Model(&lifecycle.Row{}).Where("store_id=?", storeID).Count(&count)
	require.Equal(t, int64(1), count, "replay must not create a duplicate row")
}

func TestConsumer_RejectsMissingIDs(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	cases := []lifecycle.ProAppCancelledEvent{
		{TenantID: uuid.Nil, StoreID: uuid.New()},
		{TenantID: uuid.New(), StoreID: uuid.Nil},
	}
	for i, ev := range cases {
		if err := c.Handle(context.Background(), ev); err == nil {
			t.Errorf("case %d: Handle(missing id) = nil; want error", i)
		}
	}
}

func TestConsumer_FirebaseOnly_StillSeeds(t *testing.T) {
	// A store may have a Firebase project and no Apple listing. The
	// no-identifier guard must not reject a partial-but-real event.
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
		TenantID: tenantID, StoreID: storeID, FirebaseProjectID: "fb-x",
	}))

	var row lifecycle.Row
	require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
	require.Equal(t, "fb-x", row.FirebaseProjectID)
}

func TestConsumer_NoIdentifiers_WritesNoRow(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	tenantID, storeID := uuid.New(), uuid.New()
	err := c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
		TenantID: tenantID, StoreID: storeID,
	})
	require.ErrorIs(t, err, lifecycle.ErrNoAppIdentifiers)

	var count int64
	db.Model(&lifecycle.Row{}).Where("store_id=?", storeID).Count(&count)
	require.Zero(t, count, "a refused event must leave no row for the advancer")
}

// ─── Seed-time coverage note (#702) ──────────────────────────────────

// countCoverageNotes returns the seed-time coverage rows for a store.
func coverageNotes(t *testing.T, db *gorm.DB, storeID uuid.UUID) []subscription.WhiteLabelAppLifecycleEntry {
	t.Helper()
	var entries []subscription.WhiteLabelAppLifecycleEntry
	require.NoError(t, db.Where("store_id = ? AND actor = ?", storeID, "system:consumer:pro_app_cancelled").
		Find(&entries).Error)
	return entries
}

// With google_package empty by design (decision 4), the advancer's
// `if r.GooglePackage != ""` guards mean Play is never attempted, never
// errors and never logs — the row would otherwise reach
// credentials_purged having said nothing at all about Google. The
// durable statement is what stops that.
func TestConsumer_RecordsPlayAsNotAttempted(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
		TenantID: tenantID, StoreID: storeID,
		AppleAppID: "6448000111", FirebaseProjectID: "merchant-app-42",
	}))

	// The state row must NOT carry a placeholder package.
	var row lifecycle.Row
	require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
	require.Empty(t, row.GooglePackage, "no placeholder identifier may be invented")

	notes := coverageNotes(t, db, storeID)
	require.Len(t, notes, 1)
	require.NotNil(t, notes[0].Reason)
	require.Contains(t, *notes[0].Reason, "google_play=NOT_ATTEMPTED")
	require.Contains(t, *notes[0].Reason, "apple=will_attempt(app_id=6448000111)")
	require.Equal(t, lifecycle.StatusSunsetScheduled, notes[0].Status)
}

// Replay must not append a second note: ON CONFLICT DO NOTHING means no
// new teardown was scheduled, so nothing new happened to record.
func TestConsumer_Replay_AppendsOneCoverageNote(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")
	c := lifecycle.NewProAppCancelledConsumer(db, nil)

	tenantID, storeID := uuid.New(), uuid.New()
	ev := lifecycle.ProAppCancelledEvent{TenantID: tenantID, StoreID: storeID, AppleAppID: "a1"}
	require.NoError(t, c.Handle(context.Background(), ev))
	require.NoError(t, c.Handle(context.Background(), ev))

	require.Len(t, coverageNotes(t, db, storeID), 1)
}

// End-to-end through the delivery path the finalize cron uses: one ASC
// app discovered, row seeded, Play stated as not attempted.
func TestProAppCancelled_DiscoversAndSeeds(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")

	fake := apple.NewFakeClient()
	fake.Apps = []apple.App{{ID: "6448000111", BundleID: "com.merchant.shop"}}
	c := lifecycle.NewProAppCancelledConsumer(db, nil).
		WithDiscovery(&lifecycle.Discovery{
			Apple: func(context.Context, uuid.UUID, uuid.UUID) (lifecycle.AppleLister, error) {
				return fake, nil
			},
			// No credential loader: the Play service account is
			// unavailable, so no Firebase project is discovered.
		})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, c.ProAppCancelled(context.Background(), tenantID, storeID))

	var row lifecycle.Row
	require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
	require.Equal(t, "6448000111", row.AppleAppID)
	require.Empty(t, row.GooglePackage)
	require.Equal(t, lifecycle.StatusSunsetScheduled, row.Status)
	require.False(t, row.MerchantInitiated)

	notes := coverageNotes(t, db, storeID)
	require.Len(t, notes, 1)
	require.Contains(t, *notes[0].Reason, "google_play=NOT_ATTEMPTED")
}

// An ambiguous account seeds nothing at all.
func TestProAppCancelled_AmbiguousAccount_SeedsNothing(t *testing.T) {
	db := testdb.NewDB(t, "white_label_app_state", "white_label_app_lifecycle")

	fake := apple.NewFakeClient()
	fake.Apps = []apple.App{{ID: "111"}, {ID: "222"}}
	c := lifecycle.NewProAppCancelledConsumer(db, nil).
		WithDiscovery(&lifecycle.Discovery{
			Apple: func(context.Context, uuid.UUID, uuid.UUID) (lifecycle.AppleLister, error) {
				return fake, nil
			},
		})

	storeID := uuid.New()
	err := c.ProAppCancelled(context.Background(), uuid.New(), storeID)
	require.ErrorIs(t, err, lifecycle.ErrAmbiguousAppleApp)

	var count int64
	db.Model(&lifecycle.Row{}).Where("store_id=?", storeID).Count(&count)
	require.Zero(t, count)
	require.Empty(t, coverageNotes(t, db, storeID))
}
