//go:build integration

package tax_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestClockPause_SingleOutageUnderThreshold(t *testing.T) {
	db := testdb.NewDB(t, "tax_validation_outage_log")
	tracker := tax.NewClockPauseTracker(db)
	storeID, tenantID := uuid.New(), uuid.New()
	start := time.Now().Add(-48 * time.Hour)
	key := tax.OutageKey{StoreID: storeID, TenantID: tenantID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx"}

	require.NoError(t, tracker.BeginOutage(context.Background(), key, start))
	require.NoError(t, tracker.EndOutage(context.Background(), key, start.Add(24*time.Hour)))

	paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
	require.NoError(t, err)
	require.False(t, paused, "24h < 72h threshold")
}

func TestClockPause_CumulativeOverThresholdPauses(t *testing.T) {
	db := testdb.NewDB(t, "tax_validation_outage_log")
	tracker := tax.NewClockPauseTracker(db)
	storeID, tenantID := uuid.New(), uuid.New()
	now := time.Now()

	// Three 30h outages summing to 90h cumulative across 14d.
	for i := 0; i < 3; i++ {
		s := now.Add(-time.Duration((i+1)*72) * time.Hour)
		key := tax.OutageKey{
			StoreID: storeID, TenantID: tenantID, Country: "GB", Registry: "HMRC",
			ErrorClass: "5xx",
		}
		require.NoError(t, tracker.BeginOutage(context.Background(), key, s))
		require.NoError(t, tracker.EndOutage(context.Background(), key, s.Add(30*time.Hour)))
	}

	paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
	require.NoError(t, err)
	require.True(t, paused, "90h cumulative > 72h threshold")
}

func TestClockPause_OpenOutageCounted(t *testing.T) {
	db := testdb.NewDB(t, "tax_validation_outage_log")
	tracker := tax.NewClockPauseTracker(db)
	storeID, tenantID := uuid.New(), uuid.New()
	start := time.Now().Add(-96 * time.Hour)
	key := tax.OutageKey{StoreID: storeID, TenantID: tenantID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx"}

	require.NoError(t, tracker.BeginOutage(context.Background(), key, start))

	paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
	require.NoError(t, err)
	require.True(t, paused, "open outage counts toward cumulative")
}

func TestClockPause_BeginOutageIdempotent(t *testing.T) {
	db := testdb.NewDB(t, "tax_validation_outage_log")
	tracker := tax.NewClockPauseTracker(db)
	storeID := uuid.New()
	key := tax.OutageKey{StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx"}
	now := time.Now()

	require.NoError(t, tracker.BeginOutage(context.Background(), key, now))
	require.NoError(t, tracker.BeginOutage(context.Background(), key, now.Add(time.Minute)))

	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tax_validation_outage_log WHERE store_id=? AND ended_at IS NULL`, storeID).Row().Scan(&n))
	require.Equal(t, int64(1), n, "second BeginOutage with same key must be no-op while first is still open")
}
