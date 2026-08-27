//go:build integration

package dispatch_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedStore inserts a minimal, valid row into the `stores` table so that a
// store_subscriptions row referencing it (store_id FK) can be created.
// store_subscriptions_store_id_fkey is a real, enforced constraint
// (migrations/000015_subscriptions.up.sql), so any test that creates a
// StoreSubscription must satisfy it. The row is deleted individually (not
// TRUNCATEd) on cleanup, since store_subscriptions is a child of stores and
// TRUNCATE ... CASCADE on stores would cascade into unrelated tables on this
// shared database.
func seedStore(t *testing.T, db *gorm.DB, storeID uuid.UUID) {
	t.Helper()
	secret := strings.Repeat("a", 64)
	err := db.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		VALUES (?, ?, ?, ?, 'US', 'USD', 'UTC', 'active', ?)`,
		storeID, uuid.New(), "store-"+storeID.String(), "Test Store "+storeID.String(), secret,
	).Error
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM stores WHERE id = ?`, storeID)
	})
}

func TestHandleCustomerUpdated_MirrorsEmail(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	seedStore(t, db, storeID)

	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: customerID,
	}
	require.NoError(t, db.Create(sub).Error)

	raw := []byte(`{"data":{"object":{"id":"` + customerID + `","email":"merchant@example.com","invoice_settings":{"default_payment_method":"pm_123"}}}}`)

	if err := dispatch.HandleCustomerUpdatedForTest(ctx, db, raw); err != nil {
		t.Fatalf("handleCustomerUpdated: %v", err)
	}

	var got subscription.StoreSubscription
	if err := db.Where("stripe_customer_id = ?", customerID).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Email == nil || *got.Email != "merchant@example.com" {
		t.Errorf("Email = %v, want merchant@example.com", got.Email)
	}
	if !got.HasDefaultPaymentMethod {
		t.Error("HasDefaultPaymentMethod regressed to false")
	}
}

// An event without an email must not blank an address we already have.
func TestHandleCustomerUpdated_AbsentEmailPreservesExisting(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	seedStore(t, db, storeID)

	existing := "keep@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Email:            &existing,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	raw := []byte(`{"data":{"object":{"id":"` + customerID + `","invoice_settings":{"default_payment_method":null}}}}`)

	if err := dispatch.HandleCustomerUpdatedForTest(ctx, db, raw); err != nil {
		t.Fatalf("handleCustomerUpdated: %v", err)
	}

	var got subscription.StoreSubscription
	if err := db.Where("stripe_customer_id = ?", customerID).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Email == nil || *got.Email != existing {
		t.Errorf("Email = %v, want it preserved as %q", got.Email, existing)
	}
}
