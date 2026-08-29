//go:build integration

package reconciliation

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestRunOnce_SelectsSignupRowWithNullSubscriptionID proves the widened batch
// query reaches the rows #425 leaves behind. Before this change the WHERE
// clause was `status = 'active' AND stripe_subscription_id IS NOT NULL`, which
// excluded exactly the row an orphaned Stripe subscription belongs to, so the
// drift was structurally undetectable.
func TestRunOnce_SelectsSignupRowWithNullSubscriptionID(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_reconcile",
		// stripe_subscription_id deliberately NULL: the local write that
		// would have recorded it rolled back after Stripe created it.
		Plan:               subscription.PlanTrial,
		Status:             subscription.StatusSignup,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	}).Error)

	stub := &stripeStub{
		list: []map[string]any{
			stripeSubJSON("sub_orphan_int", "trialing", "price_starter_monthly",
				map[string]string{"mark8ly_store_id": storeID.String()}),
		},
	}
	r := New(db, stub.client(t), nil, nil)

	count, err := r.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count, "the orphaned Stripe subscription must be reported as drift")
}
