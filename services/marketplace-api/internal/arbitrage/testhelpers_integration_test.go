//go:build integration

package arbitrage_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return db
}

// seedPPPSubscription inserts a minimal StoreSubscription with PriceTierPPP
// and returns (tenantID, storeID, subscriptionID).
func seedPPPSubscription(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	return seedSubscriptionWithTier(t, db, subscription.PriceTierPPP)
}

// seedDevelopedSubscription inserts a minimal StoreSubscription with PriceTierDeveloped
// and returns (tenantID, storeID, subscriptionID).
func seedDevelopedSubscription(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	return seedSubscriptionWithTier(t, db, subscription.PriceTierDeveloped)
}

func seedSubscriptionWithTier(t *testing.T, db *gorm.DB, tier subscription.PriceTier) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	tenantID := uuid.New()
	storeID := uuid.New()
	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test_" + storeID.String()[:8],
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		PriceTier:        tier,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("id = ?", sub.ID).Delete(&subscription.StoreSubscription{})
		db.Unscoped().Where("subscription_id = ?", sub.ID).Delete(&arbitrage.SubscriptionArbitrageAudit{})
	})
	return tenantID, storeID, sub.ID
}
