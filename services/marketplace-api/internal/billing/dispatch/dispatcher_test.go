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

func TestDispatch_CheckoutSessionCompleted_BindsBillingCurrency(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	payload := []byte(`{
        "id":"evt_1","type":"checkout.session.completed",
        "data":{"object":{
            "id":"cs_x","customer":"cus_x","mode":"subscription",
            "subscription":"sub_x","currency":"gbp",
            "metadata":{"plan":"starter","period":"monthly"}
        }}
    }`)

	d := dispatch.New()
	err := d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_1", EventType: "checkout.session.completed", Payload: payload,
	})
	require.NoError(t, err)

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.BillingCurrency)
	require.Equal(t, "GBP", *sub.BillingCurrency)
	require.NotNil(t, sub.StripeSubscriptionID)
	require.Equal(t, "sub_x", *sub.StripeSubscriptionID)
	require.Equal(t, subscription.StatusTrialing, sub.Status)
}

func TestDispatch_UnknownEventType_ReturnsError(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	d := dispatch.New()
	err := d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_x", EventType: "unknown", Payload: []byte(`{}`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no handler")
}

func TestDispatch_SubscriptionDeleted_StatusExpired(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_del",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	payload := []byte(`{"id":"evt_d","type":"customer.subscription.deleted","data":{"object":{"customer":"cus_del"}}}`)
	d := dispatch.New()
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_d", EventType: "customer.subscription.deleted", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusExpired, sub.Status)
}

func TestDispatch_InvoicePaymentActionRequired_StatusUpdated(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_par",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	payload := []byte(`{"id":"evt_par","type":"invoice.payment_action_required","data":{"object":{"customer":"cus_par"}}}`)
	d := dispatch.New()
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_par", EventType: "invoice.payment_action_required", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusPaymentActionRequired, sub.Status)
}

func TestHandleCheckoutSessionCompleted_BillingCurrencyLockedAfterFirstBind(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	bc := "GBP"
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_lock",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		BillingCurrency:  &bc,
	}).Error)

	// Second checkout with different currency — should be ignored by COALESCE.
	raw := []byte(`{"data":{"object":{"customer":"cus_lock","subscription":"sub_x","currency":"eur","metadata":{"plan":"starter","period":"monthly"}}}}`)
	require.NoError(t, dispatch.HandleCheckoutSessionCompletedForTesting(context.Background(), db, raw))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.BillingCurrency)
	require.Equal(t, "GBP", *sub.BillingCurrency, "billing_currency must remain GBP after second bind attempt")
}
