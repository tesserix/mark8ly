//go:build integration

package tax_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/seaqueue"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestService_ValidUK_FlipsRow_WritesMatched(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue")
	registry := tax.NewRegistry()
	registry.Set("GB", &tax.FakeValidator{CountryCode: "GB", Result: tax.ValidationResult{Valid: true, RegistryName: "Acme Widgets Ltd"}})

	svc := tax.NewService(tax.ServiceConfig{
		DB:       db,
		Registry: registry,
		SEAQueue: seaqueue.New(db),
		Clock:    tax.NewClockPauseTracker(db),
	})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status, tax_id_country, created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'trial', 'trialing', 'GB', now(), now())
	`, tenantID, storeID).Error)

	require.NoError(t, svc.Submit(context.Background(), tax.SubmitInput{
		TenantID: tenantID, StoreID: storeID,
		Country: "GB", TaxID: "GB123456789", BusinessName: "Acme Widgets Ltd",
		Source: "signup",
	}))

	var (
		validated bool
		match     string
	)
	require.NoError(t, db.Raw(`
		SELECT tax_id_validated, tax_id_name_match
		  FROM store_subscriptions
		 WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Row().Scan(&validated, &match))
	require.True(t, validated)
	require.Equal(t, string(tax.NameMatched), match)
}

func TestService_SEAManualReview_EnqueuesAndPausesClockImmediately(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue")
	registry := tax.NewRegistry()
	registry.Set("MY", &tax.FakeValidator{CountryCode: "MY", Result: tax.ValidationResult{
		ManualReviewRequired: true, QueueReason: "mof_sst_manual",
	}})

	svc := tax.NewService(tax.ServiceConfig{
		DB:       db,
		Registry: registry,
		SEAQueue: seaqueue.New(db),
		Clock:    tax.NewClockPauseTracker(db),
	})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status, tax_id_country, created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'trial', 'trialing', 'MY', now(), now())
	`, tenantID, storeID).Error)

	err := svc.Submit(context.Background(), tax.SubmitInput{
		TenantID: tenantID, StoreID: storeID,
		Country: "MY", TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
		Source: "signup",
	})
	require.ErrorIs(t, err, tax.ErrManualReviewRequired)

	// SEA queue row created.
	repo := seaqueue.New(db)
	got, err := repo.FindByStore(context.Background(), storeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, seaqueue.StatusPending, got.Status)

	// Outage log immediate pause-marker present.
	var openOutages int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM tax_validation_outage_log
		 WHERE store_id=? AND error_class='sea_queue' AND ended_at IS NULL
	`, storeID).Row().Scan(&openOutages))
	require.Equal(t, int64(1), openOutages, "SEA queue entry must open an outage row immediately")
}

func TestService_RegistryUnavailable_LogsOutage(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue")
	registry := tax.NewRegistry()
	registry.Set("GB", &tax.FakeValidator{CountryCode: "GB", Err: tax.ErrRegistryUnavailable})

	svc := tax.NewService(tax.ServiceConfig{
		DB:       db,
		Registry: registry,
		SEAQueue: seaqueue.New(db),
		Clock:    tax.NewClockPauseTracker(db),
	})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status, tax_id_country, created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'trial', 'trialing', 'GB', now(), now())
	`, tenantID, storeID).Error)

	err := svc.Submit(context.Background(), tax.SubmitInput{
		TenantID: tenantID, StoreID: storeID,
		Country: "GB", TaxID: "GB123456789", BusinessName: "Acme Ltd",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)

	var open int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM tax_validation_outage_log
		 WHERE store_id=? AND error_class='outage' AND ended_at IS NULL
	`, storeID).Row().Scan(&open))
	require.Equal(t, int64(1), open)
}

func TestService_NZDisabled_ReturnsErrValidatorDisabled(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	registry := tax.NewRegistry()
	registry.Set("NZ", &tax.FakeValidator{CountryCode: "NZ", Err: tax.ErrValidatorDisabled})

	svc := tax.NewService(tax.ServiceConfig{
		DB:       db,
		Registry: registry,
		SEAQueue: seaqueue.New(db),
		Clock:    tax.NewClockPauseTracker(db),
	})

	err := svc.Submit(context.Background(), tax.SubmitInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Country: "NZ", TaxID: "123-456-789",
	})
	require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}

func TestService_OnFastPathApproved_FlipsWindowShortenedAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	svc := tax.NewService(tax.ServiceConfig{
		DB:       db,
		Registry: tax.NewRegistry(),
		SEAQueue: seaqueue.New(db),
		Clock:    tax.NewClockPauseTracker(db),
		NowFunc:  func() time.Time { return time.Now().UTC() },
	})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status, tax_id_country, created_at, updated_at)
		VALUES (?, ?, 'cus_test', 'trial', 'trialing', 'GB', now(), now())
	`, tenantID, storeID).Error)

	require.NoError(t, svc.OnFastPathApproved(context.Background(), tenantID, storeID))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("tenant_id=? AND store_id=?", tenantID, storeID).First(&sub).Error)
	require.NotNil(t, sub.TaxIDWindowShortenedAt)
}
