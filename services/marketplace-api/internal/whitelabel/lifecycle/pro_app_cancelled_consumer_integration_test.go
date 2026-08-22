//go:build integration

package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
