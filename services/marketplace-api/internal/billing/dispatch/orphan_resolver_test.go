//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestOrphanResolver_ResolvesStoreIDAndDispatches(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_orphan",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	raw := []byte(`{"id":"evt_o1","type":"checkout.session.completed","data":{"object":{"customer":"cus_orphan","subscription":"sub_o1","currency":"usd","metadata":{"plan":"starter","period":"monthly"}}}}`)
	inserted, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_o1", EventType: "checkout.session.completed", Payload: raw,
	})
	require.NoError(t, err)
	require.True(t, inserted)

	resolver := dispatch.NewOrphanResolver(dispatch.OrphanConfig{
		DB: db, Repo: repo, Dispatcher: dispatch.New(nil), MaxRetries: 6,
	})
	require.NoError(t, resolver.RunOnce(context.Background()))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.StripeSubscriptionID)
	require.Equal(t, "sub_o1", *sub.StripeSubscriptionID)

	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id=?", "evt_o1").Error)
	require.NotNil(t, e.ProcessedAt)
}

func TestOrphanResolver_HitsRetryCap_FlagsManualReview(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	raw := []byte(`{"id":"evt_lost","type":"checkout.session.completed","data":{"object":{"customer":"cus_missing"}}}`)
	_, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_lost", EventType: "checkout.session.completed", Payload: raw,
	})
	require.NoError(t, err)

	resolver := dispatch.NewOrphanResolver(dispatch.OrphanConfig{
		DB: db, Repo: repo, Dispatcher: dispatch.New(nil), MaxRetries: 3,
	})

	for i := 0; i < 3; i++ {
		require.NoError(t, resolver.RunOnce(context.Background()))
	}

	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id=?", "evt_lost").Error)
	require.True(t, e.ManualReviewRequired, "must be flagged after retry cap")
}
