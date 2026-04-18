//go:build integration

package subscription_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestSetPendingDowngrade_PersistsTrio(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStudio,
		Status:           subscription.StatusActive,
	}).Error)

	effective := time.Now().Add(15 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, repo.SetPendingDowngrade(context.Background(), db, tenantID, storeID,
		subscription.PlanStarter, subscription.PeriodMonthly, effective, "user_initiated"))

	got, err := repo.GetByStoreID(context.Background(), db, tenantID, storeID)
	require.NoError(t, err)
	require.NotNil(t, got.PendingDowngradePlan)
	require.Equal(t, subscription.PlanStarter, *got.PendingDowngradePlan)
	require.NotNil(t, got.PendingDowngradePeriod)
	require.Equal(t, subscription.PeriodMonthly, *got.PendingDowngradePeriod)
	require.NotNil(t, got.PendingDowngradeEffectiveAt)
	require.True(t, got.PendingDowngradeEffectiveAt.Equal(effective))
}

func TestClearPendingDowngrade_UnsetsTrio(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()
	tenantID, storeID := uuid.New(), uuid.New()
	effective := time.Now().Add(24 * time.Hour)
	studioToStarter := subscription.PlanStarter
	monthly := subscription.PeriodMonthly
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                    tenantID,
		StoreID:                     storeID,
		StripeCustomerID:            "cus_x",
		Plan:                        subscription.PlanStudio,
		Status:                      subscription.StatusActive,
		PendingDowngradePlan:        &studioToStarter,
		PendingDowngradePeriod:      &monthly,
		PendingDowngradeEffectiveAt: &effective,
	}).Error)

	require.NoError(t, repo.ClearPendingDowngrade(context.Background(), db, tenantID, storeID))

	got, err := repo.GetByStoreID(context.Background(), db, tenantID, storeID)
	require.NoError(t, err)
	require.Nil(t, got.PendingDowngradePlan)
	require.Nil(t, got.PendingDowngradePeriod)
	require.Nil(t, got.PendingDowngradeEffectiveAt)
}

func TestCommitDowngrade_SwapsPlanAndPeriod(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()
	tenantID, storeID := uuid.New(), uuid.New()
	starter := subscription.PlanStarter
	monthly := subscription.PeriodMonthly
	effective := time.Now().Add(-1 * time.Hour)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                    tenantID,
		StoreID:                     storeID,
		StripeCustomerID:            "cus_x",
		Plan:                        subscription.PlanStudio,
		Status:                      subscription.StatusActive,
		SubscriptionPeriod:          subscription.PeriodAnnual,
		PendingDowngradePlan:        &starter,
		PendingDowngradePeriod:      &monthly,
		PendingDowngradeEffectiveAt: &effective,
	}).Error)

	require.NoError(t, repo.CommitDowngrade(context.Background(), db, tenantID, storeID,
		subscription.PlanStarter, subscription.PeriodMonthly))

	got, err := repo.GetByStoreID(context.Background(), db, tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, subscription.PlanStarter, got.Plan)
	require.Equal(t, subscription.PeriodMonthly, got.SubscriptionPeriod)
	require.Nil(t, got.PendingDowngradePlan)
}

func TestFindPendingDowngradesReady_ReturnsExpired(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()

	tenantID := uuid.New()
	readyStore := uuid.New()
	futureStore := uuid.New()
	noPendingStore := uuid.New()

	starter := subscription.PlanStarter
	monthly := subscription.PeriodMonthly
	pastDue := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                    tenantID,
		StoreID:                     readyStore,
		StripeCustomerID:            "cus_ready",
		Plan:                        subscription.PlanStudio,
		Status:                      subscription.StatusActive,
		PendingDowngradePlan:        &starter,
		PendingDowngradePeriod:      &monthly,
		PendingDowngradeEffectiveAt: &pastDue,
	}).Error)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                    tenantID,
		StoreID:                     futureStore,
		StripeCustomerID:            "cus_future",
		Plan:                        subscription.PlanStudio,
		Status:                      subscription.StatusActive,
		PendingDowngradePlan:        &starter,
		PendingDowngradePeriod:      &monthly,
		PendingDowngradeEffectiveAt: &future,
	}).Error)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          noPendingStore,
		StripeCustomerID: "cus_nopending",
		Plan:             subscription.PlanStudio,
		Status:           subscription.StatusActive,
	}).Error)

	ready, err := repo.FindPendingDowngradesReady(context.Background(), db, time.Now())
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.Equal(t, readyStore, ready[0].StoreID)
}
