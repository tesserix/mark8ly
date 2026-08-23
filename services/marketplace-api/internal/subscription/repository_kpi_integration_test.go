//go:build integration

package subscription_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// asOf is fixed (not time.Now()) so the window-boundary assertions below stay
// stable forever instead of drifting with the calendar.
var kpiTestAsOf = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

// seedStore inserts a minimal stores row for (tenantID, storeID) so that a
// store_subscriptions row referencing storeID satisfies
// store_subscriptions_store_id_fkey. Mirrors the equivalent helper in
// internal/subscription/dunning.
func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

func seedSubscription(t *testing.T, db *gorm.DB, tenantID uuid.UUID, status subscription.SubscriptionStatus, periodEnd *time.Time) {
	t.Helper()
	storeID := uuid.New()
	seedStore(t, db, tenantID, storeID)

	s := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + uuid.New().String(),
		Plan:             subscription.PlanTrial,
		Status:           status,
		CurrentPeriodEnd: periodEnd,
	}
	require.NoError(t, db.Create(&s).Error)
}

func at(d time.Duration) *time.Time {
	tm := kpiTestAsOf.Add(d)
	return &tm
}

// TestRepository_CountTrialsExpiring_WithinHorizon proves a trialing
// subscription ending inside the window is counted.
func TestRepository_CountTrialsExpiring_WithinHorizon(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(3*24*time.Hour))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)
}

// TestRepository_CountTrialsExpiring_OutsideHorizon proves a trial ending
// past the horizon (10 days out, horizon is 7) is not counted.
func TestRepository_CountTrialsExpiring_OutsideHorizon(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(10*24*time.Hour))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

// TestRepository_CountTrialsExpiring_RightEdgeInclusive proves a trial
// ending exactly at asOf+TrialExpiryHorizon counts (inclusive right edge).
func TestRepository_CountTrialsExpiring_RightEdgeInclusive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(subscription.TrialExpiryHorizon))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)
}

// TestRepository_CountTrialsExpiring_AlreadyExpiredExcluded proves the left
// edge is half-open: a trial whose period already ended is not "expiring".
func TestRepository_CountTrialsExpiring_AlreadyExpiredExcluded(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(-1*time.Hour))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

// TestRepository_CountTrialsExpiring_LeftEdgeExclusive proves the left edge
// is strictly exclusive: a trial whose period ends exactly at asOf has
// already expired and is not "expiring". A fixture an hour away from the
// boundary (see AlreadyExpiredExcluded above) can't distinguish `>` from
// `>=`; only current_period_end == asOf can.
func TestRepository_CountTrialsExpiring_LeftEdgeExclusive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(0))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

// TestRepository_CountTrialsExpiring_OnlyTrialingStatus proves an active
// subscription ending within the window is not counted — only trialing.
func TestRepository_CountTrialsExpiring_OnlyTrialingStatus(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusActive, at(3*24*time.Hour))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

// TestRepository_CountTrialsExpiring_NilPeriodEndExcluded proves a trialing
// row with a NULL current_period_end is skipped without error.
func TestRepository_CountTrialsExpiring_NilPeriodEndExcluded(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, nil)

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

// TestRepository_CountTrialsExpiring_CountsAcrossTenants proves the count is
// estate-wide: two trialing subscriptions under different tenants both count.
func TestRepository_CountTrialsExpiring_CountsAcrossTenants(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(2*24*time.Hour))
	seedSubscription(t, db, uuid.New(), subscription.StatusTrialing, at(5*24*time.Hour))

	got, err := repo.CountTrialsExpiring(context.Background(), db, kpiTestAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got)
}
