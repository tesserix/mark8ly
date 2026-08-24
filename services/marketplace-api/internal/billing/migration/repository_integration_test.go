//go:build integration

package migration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// openIntegrationDB connects to the database specified by TEST_DATABASE_URL —
// the same env var every other integration test in this repo reads (see
// pkg/testdb.NewDB). This file previously gated on the nonstandard
// TEST_DB_DSN, which meant these tests silently skipped under the variable
// everyone else sets (#317's env-var-split family of traps).
// Tests that call this are skipped when the env var is absent.
func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn, ok := os.LookupEnv("TEST_DATABASE_URL")
	if !ok || dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open integration DB")
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return db
}

// seedStoreForReview inserts the parent stores row that
// store_subscriptions.store_id requires via foreign key (see
// seedSubscriptionForStore below). marketplace-api's stores table is a local
// projection of platform-api's (migrations/000001_products_initial.up.sql) —
// plain country_code/currency_code/timezone columns with no reference-data
// foreign keys, only stores_slug_unique and the stores_status_valid CHECK
// (status IN ('active','suspended','archived')). So the country/currency/
// timezone values here need only be plausible strings; GB/GBP/Europe/London
// are used for consistency with other fixtures in this repo (e.g.
// seedStoreForCSV in internal/handlers/platformadmin/health_checks_integration_test.go),
// not because any FK or seeded reference row requires them.
func seedStoreForReview(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	s := &stores.Store{
		ID:           storeID.String(),
		TenantID:     tenantID.String(),
		Slug:         "migration-fastpath-" + storeID.String()[:8],
		Name:         "Migration Fast-Path Test Store",
		CountryCode:  "GB",
		CurrencyCode: "GBP",
		Timezone:     "Europe/London",
		Status:       stores.StatusActive,
	}
	require.NoError(t, db.Create(s).Error)
	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID.String())
	})
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

// seedSubscriptionForStore inserts the parent stores row (required by the
// store_subscriptions.store_id foreign key) and a minimal store_subscriptions
// row for the given store_id so the Approve path can stamp
// tax_id_window_shortened_at.
func seedSubscriptionForStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	seedStoreForReview(t, db, tenantID, storeID)
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
	updated, err := repo.Approve(context.Background(), row.ID, reviewerID, "Shopify export verified.")
	require.NoError(t, err)
	require.NotNil(t, updated, "Approve must return the updated review row")
	assert.Equal(t, tenantID, updated.TenantID, "returned row must carry the review's real tenant id")
	assert.Equal(t, storeID, updated.StoreID, "returned row must carry the review's real store id")

	// Verify tax_id_window_shortened_at was stamped via raw query (the Go model
	// doesn't have this column yet — it was added in migration 053).
	// sql.NullTime, not *time.Time: GORM's Raw().Scan() into a bare *time.Time
	// destination errors ("unsupported Scan ... storing driver.Value type
	// <nil>") the moment the column is actually NULL — this only surfaces
	// once a test exercises the NULL branch, which TestRepo_Reject_NoSideEffect
	// below is the first to do.
	var shortenedAt sql.NullTime
	db.Raw("SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE store_id = ?", storeID).
		Scan(&shortenedAt)
	assert.True(t, shortenedAt.Valid, "tax_id_window_shortened_at should be non-NULL after approval")

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
	updated, err := repo.Reject(context.Background(), row.ID, reviewerID, "Cannot verify evidence URL.")
	require.NoError(t, err)
	require.NotNil(t, updated, "Reject must return the updated review row")
	assert.Equal(t, tenantID, updated.TenantID, "returned row must carry the review's real tenant id")
	assert.Equal(t, storeID, updated.StoreID, "returned row must carry the review's real store id")

	// tax_id_window_shortened_at must remain NULL — reject has no side effect.
	var shortenedAt sql.NullTime
	db.Raw("SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE store_id = ?", storeID).
		Scan(&shortenedAt)
	assert.False(t, shortenedAt.Valid, "tax_id_window_shortened_at should remain NULL after rejection")

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
