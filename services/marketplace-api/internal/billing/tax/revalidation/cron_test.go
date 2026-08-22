//go:build integration

package revalidation_test

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
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingNotifier captures Create() calls.
type recordingNotifier struct{ calls []notification.Notification }

func (r *recordingNotifier) Create(_ context.Context, n *notification.Notification) error {
	r.calls = append(r.calls, *n)
	return nil
}

func seedValidatedSubscription(t *testing.T, db *gorm.DB, country, taxID, businessName string, validatedAt time.Time) (uuid.UUID, uuid.UUID) {
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

func newSvcWithFake(db *gorm.DB, country string, fakeResult tax.ValidationResult, fakeErr error) *tax.Service {
	registry := tax.NewRegistry()
	registry.Set(country, &tax.FakeValidator{CountryCode: country, Result: fakeResult, Err: fakeErr})
	return tax.NewService(tax.ServiceConfig{
		DB: db, Registry: registry,
		SEAQueue: seaqueue.New(db), Clock: tax.NewClockPauseTracker(db),
	})
}

func TestRevalidation_StillValid_NoStateChange(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue", "notifications")
	tenantID, storeID := seedValidatedSubscription(t, db, "GB", "GB123456789", "Acme Ltd", time.Now().Add(-100*24*time.Hour))

	c := &revalidation.Cron{
		DB:  db,
		Svc: newSvcWithFake(db, "GB", tax.ValidationResult{Valid: true, RegistryName: "Acme Ltd"}, nil),
	}
	require.NoError(t, c.Run(context.Background()))

	var validated bool
	var attempted *time.Time
	require.NoError(t, db.Raw(`
		SELECT tax_id_validated, revalidation_attempted_at FROM store_subscriptions WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Row().Scan(&validated, &attempted))
	require.True(t, validated, "still-valid ID must remain validated")
	require.NotNil(t, attempted, "revalidation_attempted_at must be stamped")
}

func TestRevalidation_GoneInvalid_FlipsAndNotifies_KeepsBilling(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue", "notifications")
	tenantID, storeID := seedValidatedSubscription(t, db, "GB", "GB123456789", "Acme Ltd", time.Now().Add(-100*24*time.Hour))

	notif := &recordingNotifier{}
	c := &revalidation.Cron{
		DB:       db,
		Svc:      newSvcWithFake(db, "GB", tax.ValidationResult{}, tax.ErrNotFound),
		Notifier: notif,
	}
	require.NoError(t, c.Run(context.Background()))

	var (
		validated bool
		started   *time.Time
		published bool
		statusStr string
	)
	require.NoError(t, db.Raw(`
		SELECT tax_id_validated, tax_revalidation_started_at, storefront_published, status
		  FROM store_subscriptions WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Row().Scan(&validated, &started, &published, &statusStr))

	require.False(t, validated, "definitive failure must flip tax_id_validated=false")
	require.NotNil(t, started, "tax_revalidation_started_at must open the 14-day window")
	require.True(t, published, "storefront stays up during 14-day grace window")
	require.Equal(t, "active", statusStr, "subscription status remains active — billing continues per §19.5")
	require.Len(t, notif.calls, 1, "merchant must be notified")
}

func TestRevalidation_OutageTransient_NoFlip(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue", "notifications")
	tenantID, storeID := seedValidatedSubscription(t, db, "GB", "GB123456789", "Acme Ltd", time.Now().Add(-100*24*time.Hour))

	c := &revalidation.Cron{
		DB:  db,
		Svc: newSvcWithFake(db, "GB", tax.ValidationResult{}, tax.ErrRegistryUnavailable),
	}
	require.NoError(t, c.Run(context.Background()))

	var validated bool
	require.NoError(t, db.Raw(`SELECT tax_id_validated FROM store_subscriptions WHERE tenant_id=? AND store_id=?`, tenantID, storeID).Row().Scan(&validated))
	require.True(t, validated, "transient outage must NOT flip the row")
}

func TestRevalidation_Day14_UnpublishesStorefront_BillingStillActive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "tax_validation_outage_log", "sea_manual_review_queue", "notifications")
	tenantID, storeID := seedValidatedSubscription(t, db, "GB", "GB123456789", "Acme Ltd", time.Now().Add(-100*24*time.Hour))

	// Manually put the row past day 14 of the revalidation window.
	require.NoError(t, db.Exec(`
		UPDATE store_subscriptions
		   SET tax_id_validated = false,
		       tax_revalidation_started_at = now() - INTERVAL '15 days'
		 WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Error)

	c := &revalidation.Cron{
		DB:  db,
		Svc: newSvcWithFake(db, "GB", tax.ValidationResult{}, tax.ErrNotFound),
	}
	require.NoError(t, c.Run(context.Background()))

	var (
		published bool
		reason    *string
		statusStr string
	)
	require.NoError(t, db.Raw(`
		SELECT storefront_published, storefront_unpublish_reason, status
		  FROM store_subscriptions WHERE tenant_id=? AND store_id=?
	`, tenantID, storeID).Row().Scan(&published, &reason, &statusStr))

	require.False(t, published, "storefront must be unpublished at day 14")
	require.NotNil(t, reason)
	require.Equal(t, "tax_revalidation_failed", *reason)
	require.Equal(t, "active", statusStr, "billing continues — no perverse incentive (§19.5)")
}
