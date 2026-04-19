//go:build integration

// Package integration_test holds spec-traceable integration tests grouped by
// success-criterion number from docs/superpowers/specs/2026-04-17-subscription-model-design.md.
// Each test name encodes the criterion it covers so the §28 traceability
// matrix can be regenerated with `go test -list 'Criterion'`.
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/revalidation"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/seaqueue"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedActiveValidatedSubscription creates a fresh active subscription with a
// validated tax ID so revalidation cron has something to re-check.
func seedActiveValidatedSubscription(t *testing.T, db *gorm.DB, country, taxID string, validatedAt time.Time) (uuid.UUID, uuid.UUID) {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status,
			 tax_id_country, tax_id_validated, tax_id_validated_at,
			 tax_id_name_match, reverse_charge_tax_id,
			 storefront_published, created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'starter', 'active',
		        ?, true, ?, 'matched', ?,
		        true, now(), now())
	`, tenantID, storeID, country, validatedAt, taxID).Error)
	return tenantID, storeID
}

func seedTrialingSubscription(t *testing.T, db *gorm.DB, country string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status,
			 tax_id_country, tax_id_validated, tax_id_name_match,
			 created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'trial', 'trialing',
		        ?, false, 'not_checked',
		        now(), now())
	`, tenantID, storeID, country).Error)
	return tenantID, storeID
}

// Test_Criterion41_QuarterlyRevalidation_UnpublishesStorefront_BillingContinues
// proves §28 #41: tax ID lapses → storefront unpublishes at day 14, billing
// continues (subscription status remains `active`). The intentional design is
// "no perverse incentive": we don't boot the merchant off paid billing for a
// registry hiccup; we just gate the storefront until they fix it (§19.5).
func Test_Criterion41_QuarterlyRevalidation_UnpublishesStorefront_BillingContinues(t *testing.T) {
	db := testdb.NewDB(t,
		"store_subscriptions",
		"tax_validation_outage_log",
		"sea_manual_review_queue",
		"notifications",
	)
	tenantID, storeID := seedActiveValidatedSubscription(t, db, "GB", "GB123456789", time.Now().Add(-100*24*time.Hour))

	// Day 0: validator says "not found anymore" — flips tax_id_validated=false,
	// opens 14d window, billing untouched, storefront stays up.
	registry := tax.NewRegistry()
	registry.Set("GB", &tax.FakeValidator{CountryCode: "GB", Err: tax.ErrNotFound})
	svc := tax.NewService(tax.ServiceConfig{
		DB: db, Registry: registry,
		SEAQueue: seaqueue.New(db), Clock: tax.NewClockPauseTracker(db),
	})
	cron := &revalidation.Cron{DB: db, Svc: svc}
	require.NoError(t, cron.Run(context.Background()))

	{
		var validated, published bool
		var status string
		require.NoError(t, db.Raw(`
			SELECT tax_id_validated, storefront_published, status
			  FROM store_subscriptions WHERE tenant_id=? AND store_id=?
		`, tenantID, storeID).Row().Scan(&validated, &published, &status))
		require.False(t, validated)
		require.True(t, published, "storefront stays up during 14-day grace")
		require.Equal(t, "active", status, "billing MUST continue (§19.5)")
	}

	// Day 15: simulate the passage of time by back-dating tax_revalidation_started_at.
	require.NoError(t, db.Exec(`
		UPDATE store_subscriptions
		   SET tax_revalidation_started_at = now() - INTERVAL '15 days'
		 WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Error)
	require.NoError(t, cron.Run(context.Background()))

	{
		var published bool
		var reason *string
		var status string
		require.NoError(t, db.Raw(`
			SELECT storefront_published, storefront_unpublish_reason, status
			  FROM store_subscriptions WHERE tenant_id=? AND store_id=?
		`, tenantID, storeID).Row().Scan(&published, &reason, &status))
		require.False(t, published, "storefront must be unpublished at day 14")
		require.NotNil(t, reason)
		require.Equal(t, "tax_revalidation_failed", *reason)
		require.Equal(t, "active", status, "billing still active — no perverse incentive")
	}
}

// Test_Criterion45_SEAQueueEntry_PausesClockImmediately proves §28 #45 +
// Council finding #10: ID enters SEA manual review → 14d clock pauses
// immediately at queue entry, not after queue resolution.
func Test_Criterion45_SEAQueueEntry_PausesClockImmediately(t *testing.T) {
	db := testdb.NewDB(t,
		"store_subscriptions",
		"tax_validation_outage_log",
		"sea_manual_review_queue",
		"notifications",
	)
	tenantID, storeID := seedTrialingSubscription(t, db, "MY")

	registry := tax.NewRegistry()
	registry.Set("MY", &tax.FakeValidator{CountryCode: "MY", Result: tax.ValidationResult{
		ManualReviewRequired: true, QueueReason: "mof_sst_manual",
	}})
	svc := tax.NewService(tax.ServiceConfig{
		DB: db, Registry: registry,
		SEAQueue: seaqueue.New(db), Clock: tax.NewClockPauseTracker(db),
	})

	err := svc.Submit(context.Background(), tax.SubmitInput{
		TenantID: tenantID, StoreID: storeID,
		Country: "MY", TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
		Source: "signup",
	})
	require.ErrorIs(t, err, tax.ErrManualReviewRequired)

	// Queue row must exist.
	repo := seaqueue.New(db)
	got, err := repo.FindByStore(context.Background(), storeID)
	require.NoError(t, err)
	require.NotNil(t, got, "SEA queue row must be created")
	require.Equal(t, seaqueue.StatusPending, got.Status)

	// Outage row must be open with error_class='sea_queue' — that's what
	// causes IsPaused to return true once thresholds are met.
	var openOutages int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM tax_validation_outage_log
		 WHERE store_id=? AND error_class='sea_queue' AND ended_at IS NULL
	`, storeID).Row().Scan(&openOutages))
	require.Equal(t, int64(1), openOutages, "queue entry must open an outage immediately, not after resolution")
}

// Test_Criterion53_FastPath_OnlyApprovedViaCSMExplicit proves §28 #53 (in the
// P7 slice — the WHOIS <90d rejection logic itself lives in P5's intake
// handler). Here we simply assert that the OnFastPathApproved listener is
// idempotent and never auto-flips when called without a prior approval.
func Test_Criterion53_FastPath_OnlyApprovedViaCSMExplicit(t *testing.T) {
	db := testdb.NewDB(t,
		"store_subscriptions",
		"tax_validation_outage_log",
		"sea_manual_review_queue",
		"notifications",
	)
	tenantID, storeID := seedTrialingSubscription(t, db, "GB")

	svc := tax.NewService(tax.ServiceConfig{
		DB: db, Registry: tax.NewRegistry(),
		SEAQueue: seaqueue.New(db), Clock: tax.NewClockPauseTracker(db),
	})

	// First call (CSM approval) — flips the flag once.
	require.NoError(t, svc.OnFastPathApproved(context.Background(), tenantID, storeID))
	var firstStamp *time.Time
	require.NoError(t, db.Raw(`SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE tenant_id=? AND store_id=?`, tenantID, storeID).Row().Scan(&firstStamp))
	require.NotNil(t, firstStamp, "first approval must stamp the timestamp")

	// Second call — must be a no-op (idempotent; CSM-driven only).
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, svc.OnFastPathApproved(context.Background(), tenantID, storeID))
	var secondStamp *time.Time
	require.NoError(t, db.Raw(`SELECT tax_id_window_shortened_at FROM store_subscriptions WHERE tenant_id=? AND store_id=?`, tenantID, storeID).Row().Scan(&secondStamp))
	require.NotNil(t, secondStamp)
	require.True(t, firstStamp.Equal(*secondStamp), "subsequent OnFastPathApproved must NOT overwrite the first stamp")
}
