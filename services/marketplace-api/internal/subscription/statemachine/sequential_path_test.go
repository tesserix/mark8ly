//go:build integration

package statemachine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestSequentialPath_ExpiredToHardDeleted exercises the full teardown sequence
// mandated by §17.2: expired → store_closed → pending_hard_delete → hard_deleted.
// Each step must succeed and produce the correct persisted status before the
// next transition is attempted. This pins the invariant that no step in the
// chain can be skipped.
func TestSequentialPath_ExpiredToHardDeleted(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_seq",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusExpired,
	}).Error)

	// Step 1: expired → store_closed (day 14 post-expiry cron).
	require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusExpired,
		To:       subscription.StatusStoreClosed,
		Actor:    "system:cron:expiry_grace",
		Reason:   "day_14_post_expiry",
	}))
	assertStatus(t, db, storeID, subscription.StatusStoreClosed)

	// Step 2: store_closed → pending_hard_delete (day 90 post-expiry cron).
	require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusStoreClosed,
		To:       subscription.StatusPendingHardDelete,
		Actor:    "system:cron:deletion",
		Reason:   "day_90_post_expiry",
	}))
	assertStatus(t, db, storeID, subscription.StatusPendingHardDelete)

	// Step 3: pending_hard_delete → hard_deleted (deletion job).
	require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusPendingHardDelete,
		To:       subscription.StatusHardDeleted,
		Actor:    "system:cron:deletion",
		Reason:   "deletion_job",
	}))
	assertStatus(t, db, storeID, subscription.StatusHardDeleted)
}

// TestSequentialPath_NoShortcut asserts that the forbidden shortcut
// expired → pending_hard_delete is rejected with ErrInvalidTransition
// and that the row is left untouched.
func TestSequentialPath_NoShortcut(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_shortcut",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusExpired,
	}).Error)

	err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusExpired,
		To:       subscription.StatusPendingHardDelete,
		Actor:    "system:cron",
		Reason:   "naughty",
	})
	require.True(t, errors.Is(err, statemachine.ErrInvalidTransition),
		"shortcut expired → pending_hard_delete must be rejected")
	// Row must remain unchanged.
	assertStatus(t, db, storeID, subscription.StatusExpired)
}

// assertStatus re-reads the store_subscriptions row and asserts its current
// status equals want. It uses *gorm.DB directly to avoid any caching layer.
func assertStatus(t *testing.T, db *gorm.DB, storeID uuid.UUID, want subscription.SubscriptionStatus) {
	t.Helper()
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
	require.Equal(t, want, sub.Status)
}
