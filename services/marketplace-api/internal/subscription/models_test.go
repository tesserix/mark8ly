//go:build integration

package subscription_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestStoreSubscription_V23ColumnsRoundTrip verifies that a StoreSubscription
// populated with the v2.3 fields (tax ID, multi-currency, add-on flags) can be
// persisted and re-loaded without losing data. Skipped automatically when
// TEST_DATABASE_URL is unset — the build must still succeed so CI can verify
// the schema/struct alignment independently of DB availability.
func TestStoreSubscription_V23ColumnsRoundTrip(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	sub := subscription.StoreSubscription{
		TenantID:              tenantID,
		StoreID:               storeID,
		StripeCustomerID:      "cus_test",
		Plan:                  subscription.PlanStudio,
		Status:                subscription.StatusSignup,
		ReverseChargeTaxID:    strPtr("GB123456789"),
		TaxIDCountry:          strPtr("GB"),
		TaxIDValidated:        true,
		BillingCurrency:       strPtr("GBP"),
		PriceTier:             subscription.PriceTierDeveloped,
		HasWhiteLabelAppAddOn: false,
		ArbitrageFlag:         false,
		TaxIDNameMatch:        subscription.TaxIDNameMatchMatched,
	}
	require.NoError(t, db.Create(&sub).Error)

	var got subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&got).Error)

	require.Equal(t, subscription.PlanStudio, got.Plan)
	require.Equal(t, subscription.StatusSignup, got.Status)
	require.Equal(t, "GBP", *got.BillingCurrency)
	require.Equal(t, subscription.PriceTierDeveloped, got.PriceTier)
	require.True(t, got.TaxIDValidated)
	require.Equal(t, subscription.TaxIDNameMatchMatched, got.TaxIDNameMatch)
}

func strPtr(s string) *string { return &s }
