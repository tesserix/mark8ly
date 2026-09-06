//go:build integration

package trial_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// mark8ly#660 T6, the "future stores" half. A tenant granted a platform
// override before this store had a Stripe subscription must have it applied
// when the subscription is finally created — otherwise the discount silently
// stops covering the tenant as they grow.
//
// These use testdb (TEST_DATABASE_URL) rather than this file's neighbours'
// openIntegrationDB (TEST_DB_DSN). `make test-int` exports only the former, so
// a test written against the latter never runs there.

// stubDiscounter stands in for tenantdiscount.Service at the port trial
// declares. Hand-rolled per this repository's convention.
type stubDiscounter struct {
	calls   []tenantdiscount.NewSubscriptionInput
	outcome tenantdiscount.Outcome
	err     error
}

func (s *stubDiscounter) ApplyToNewSubscription(_ context.Context, in tenantdiscount.NewSubscriptionInput) (tenantdiscount.Outcome, error) {
	s.calls = append(s.calls, in)
	return s.outcome, s.err
}

func TestSubscribe_OffersTheNewSubscriptionToTheTenantsStandingOverride(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	seedSubscription(t, db, subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test",
		Status:           subscription.StatusSignup,
	})

	disc := &stubDiscounter{outcome: tenantdiscount.OutcomeApplied}
	sub := trial.NewSubscriber(db, defaultFakeStripe(), nil).WithTenantDiscount(disc)

	res, err := sub.Subscribe(context.Background(), buildInput(tenantID, storeID))
	require.NoError(t, err)
	require.Equal(t, "sub_test_123", res.StripeSubscriptionID)

	require.Len(t, disc.calls, 1, "the newly created subscription must be offered to the override hook")
	require.Equal(t, tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: storeID, StripeSubscriptionID: "sub_test_123",
	}, disc.calls[0])
}

// THE LOAD-BEARING ONE. A discount that cannot be applied costs the tenant a
// discount an operator can re-apply by hand; a discount that BLOCKS the call
// costs the merchant their subscription. The first is recoverable and the
// second is not, so this failure must never propagate.
func TestSubscribe_ADiscountFailureDoesNotBlockTheSubscription(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	seedSubscription(t, db, subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test",
		Status:           subscription.StatusSignup,
	})

	disc := &stubDiscounter{err: errors.New("stripe is down")}
	sub := trial.NewSubscriber(db, defaultFakeStripe(), nil).WithTenantDiscount(disc)

	res, err := sub.Subscribe(context.Background(), buildInput(tenantID, storeID))
	require.NoError(t, err, "a discount failure must never fail the subscription")
	require.Equal(t, "sub_test_123", res.StripeSubscriptionID)

	// And the subscription really landed, rather than being reported and
	// rolled back: without this the assertion above would pass for a
	// transaction that discarded everything.
	var got subscription.StoreSubscription
	require.NoError(t, db.Where("tenant_id = ? AND store_id = ?", tenantID, storeID).First(&got).Error)
	require.NotNil(t, got.StripeSubscriptionID)
	require.Equal(t, "sub_test_123", *got.StripeSubscriptionID)
}

// The hook is optional wiring: without Stripe billing configured there is no
// tenantdiscount.Service to pass, and Subscribe must behave exactly as it did
// before T6.
func TestSubscribe_WithNoDiscounterWiredSubscribesUnchanged(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	seedSubscription(t, db, subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test",
		Status:           subscription.StatusSignup,
	})

	res, err := trial.NewSubscriber(db, defaultFakeStripe(), nil).
		Subscribe(context.Background(), buildInput(tenantID, storeID))
	require.NoError(t, err)
	require.Equal(t, "sub_test_123", res.StripeSubscriptionID)
}
