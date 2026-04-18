//go:build integration

package dunning_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestLadder_PastDue_TransitionsToExpired verifies end-to-end that a
// past_due subscription with UpdatedAt 8+ days ago (no FirstChargeAt) is
// advanced to expired by a single Run pass.
func TestLadder_PastDue_TransitionsToExpired(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "audit_logs")

	storeID := uuid.New()
	tenantID := uuid.New()
	eighteenDaysAgo := time.Now().UTC().Add(-18 * 24 * time.Hour)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		UpdatedAt:        eighteenDaysAgo,
		CreatedAt:        eighteenDaysAgo,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	clock := func() time.Time { return time.Now().UTC() }
	ladder := dunning.NewStepDailyLadder(db, nil, nil, nil, clock)

	if err := ladder.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var refreshed subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&refreshed).Error; err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.Status != subscription.StatusExpired {
		t.Fatalf("expected status expired, got %q", refreshed.Status)
	}
}

// TestLadder_PastDue_InRefundWindow_NotAdvanced verifies that a past_due
// subscription with a recent FirstChargeAt is NOT advanced regardless of
// how many days past_due it has been.
func TestLadder_PastDue_InRefundWindow_NotAdvanced(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "audit_logs")

	storeID := uuid.New()
	tenantID := uuid.New()
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour)
	sevenDaysAgo := time.Now().UTC().Add(-7 * 24 * time.Hour)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test2",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		FirstChargeAt:    &sevenDaysAgo, // within 14-day refund window
		UpdatedAt:        thirtyDaysAgo,
		CreatedAt:        thirtyDaysAgo,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	clock := func() time.Time { return time.Now().UTC() }
	ladder := dunning.NewStepDailyLadder(db, nil, nil, nil, clock)

	if err := ladder.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var refreshed subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&refreshed).Error; err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// Must remain past_due — refund window guard blocked transition.
	if refreshed.Status != subscription.StatusPastDue {
		t.Fatalf("expected status past_due (refund window guard), got %q", refreshed.Status)
	}
}
