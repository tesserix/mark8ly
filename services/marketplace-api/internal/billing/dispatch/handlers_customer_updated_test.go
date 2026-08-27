//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestHandleCustomerUpdated_MirrorsEmail(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         tenantID,
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

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	existing := "keep@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         tenantID,
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
