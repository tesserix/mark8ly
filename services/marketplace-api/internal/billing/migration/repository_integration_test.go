//go:build integration

package migration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// openIntegrationDB connects to the database specified by TEST_DB_DSN.
// Tests that call this are skipped when the env var is absent.
func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn, ok := os.LookupEnv("TEST_DB_DSN")
	if !ok || dsn == "" {
		t.Skip("TEST_DB_DSN not set; skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open integration DB")
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return db
}

// seedReview inserts a migration_fast_path_reviews row and registers cleanup.
func seedReview(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) *migration.Review {
	t.Helper()
	repo := migration.NewRepository(db)
	row, err := repo.CreatePending(context.Background(), migration.CreatePendingInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		EvidenceType:  "platform_screenshot",
		EvidenceURL:   "https://example.com/screenshot.png",
		PriorPlatform: "shopify",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec("DELETE FROM migration_fast_path_reviews WHERE id = ?", row.ID)
	})
	return row
}

// seedSubscriptionForStore inserts a minimal store_subscriptions row for
// the given store_id so the Approve path can stamp tax_id_window_shortened_at.
func seedSubscriptionForStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	sub := subscription.StoreSubscription{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_test_" + storeID.String()[:8],
		Plan:               subscription.PlanStarter,
		Status:             subscription.StatusSignup,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	require.NoError(t, db.Create(&sub).Error)
	t.Cleanup(func() {
		db.Exec("DELETE FROM store_subscriptions WHERE store_id = ?", storeID)
	})
}

func TestRepo_CreatePending_HappyPath(t *testing.T) {
	db := openIntegrationDB(t)
	repo := migration.NewRepository(db)

	tenantID := uuid.New()
	storeID := uuid.New()

	row, err := repo.CreatePending(context.Background(), migration.CreatePendingInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		EvidenceType:  "platform_screenshot",
		EvidenceURL:   "https://example.com/ss.png",
		PriorPlatform: "woocommerce",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec("DELETE FROM migration_fast_path_reviews WHERE id = ?", row.ID)
	})

	assert.NotEqual(t, uuid.Nil, row.ID)
	assert.Equal(t, "pending", row.Status)
	assert.Equal(t, storeID, row.StoreID)
	assert.Equal(t, tenantID, row.TenantID)
}

func TestRepo_CreatePending_OnlyOneOpenPerStore(t *testing.T) {
	db := openIntegrationDB(t)

	tenantID := uuid.New()
	storeID := uuid.New()
	first := seedReview(t, db, tenantID, storeID)
	_ = first

	repo := migration.NewRepository(db)
	_, err := repo.CreatePending(context.Background(), migration.CreatePendingInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		EvidenceType: "whois_domain",
		EvidenceURL:  "https://example.com/whois.txt",
		WhoisDomain:  "oldstore.example.com",
	})

	assert.ErrorIs(t, err, migration.ErrAlreadyPending)
}

func TestRepo_Approve_ShortensTaxWindow(t *testing.T) {
	db := openIntegrationDB(t)

	tenantID := uuid.New()
	storeID := uuid.New()

	seedSubscriptionForStore(t, db, tenantID, storeID)
	row := seedReview(t, db, tenantID, storeID)

	reviewerID := uuid.New()
	repo := migration.NewRepository(db)
	err := repo.Approve(context.Background(), row.ID, reviewerID, "Shopify export verified.")
	require.NoError(t, err)

	// Verify tax_id_window_shortened_at was stamped via raw query (the Go model
	// doesn't have this column yet — it was added in migration 053).
	var shortenedAt *time.Time
	db.Raw("SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE store_id = ?", storeID).
		Scan(&shortenedAt)
	assert.NotNil(t, shortenedAt, "tax_id_window_shortened_at should be non-NULL after approval")

	// Verify review row is now approved.
	approved, err := repo.Get(context.Background(), row.ID)
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	assert.Equal(t, reviewerID, *approved.ReviewerID)
}

func TestRepo_Reject_NoSideEffect(t *testing.T) {
	db := openIntegrationDB(t)

	tenantID := uuid.New()
	storeID := uuid.New()

	seedSubscriptionForStore(t, db, tenantID, storeID)
	row := seedReview(t, db, tenantID, storeID)

	reviewerID := uuid.New()
	repo := migration.NewRepository(db)
	err := repo.Reject(context.Background(), row.ID, reviewerID, "Cannot verify evidence URL.")
	require.NoError(t, err)

	// tax_id_window_shortened_at must remain NULL — reject has no side effect.
	var shortenedAt *time.Time
	db.Raw("SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE store_id = ?", storeID).
		Scan(&shortenedAt)
	assert.Nil(t, shortenedAt, "tax_id_window_shortened_at should remain NULL after rejection")

	// Verify review row is now rejected.
	rejected, err := repo.Get(context.Background(), row.ID)
	require.NoError(t, err)
	assert.Equal(t, "rejected", rejected.Status)
	assert.Equal(t, reviewerID, *rejected.ReviewerID)
}

func TestRepo_Get_NotFound_ReturnsErrNotFound(t *testing.T) {
	db := openIntegrationDB(t)
	repo := migration.NewRepository(db)

	_, err := repo.Get(context.Background(), uuid.New())

	assert.ErrorIs(t, err, migration.ErrNotFound)
}
