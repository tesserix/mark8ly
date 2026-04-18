//go:build integration

package dispatch_test

import (
	"context"
	"testing"
	"time"

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

	d := dispatch.New(nil)
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
	d := dispatch.New(nil)
	err := d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_x", EventType: "unknown", Payload: []byte(`{}`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no handler")
}

// TestDispatch_SubscriptionDeleted_StatusExpired seeds the subscription in
// past_due (a valid From per §17.2 past_due → expired) and expects the
// handler to advance status to expired via statemachine.Transition.
// Note: active → expired is NOT a valid §17.2 transition, so the old seed
// of StatusActive has been changed to StatusPastDue.
func TestDispatch_SubscriptionDeleted_StatusExpired(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_del",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue, // was StatusActive; active→expired is not a valid move
	}).Error)

	payload := []byte(`{"id":"evt_d","type":"customer.subscription.deleted","data":{"object":{"customer":"cus_del"}}}`)
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_d", EventType: "customer.subscription.deleted", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusExpired, sub.Status)
}

// TestDispatch_SubscriptionDeleted_ActiveIsNoOp verifies that a
// customer.subscription.deleted event arriving while the subscription is still
// active is treated as a benign no-op (active → expired is not in §17.2).
func TestDispatch_SubscriptionDeleted_ActiveIsNoOp(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_del_active",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	payload := []byte(`{"id":"evt_d2","type":"customer.subscription.deleted","data":{"object":{"customer":"cus_del_active"}}}`)
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_d2", EventType: "customer.subscription.deleted", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	// Status must remain active — no direct active→expired transition.
	require.Equal(t, subscription.StatusActive, sub.Status)
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
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_par", EventType: "invoice.payment_action_required", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusPaymentActionRequired, sub.Status)
}

// TestDispatch_InvoicePaymentFailed_ActiveToPastDue verifies that
// invoice.payment_failed on an active subscription advances it to past_due.
func TestDispatch_InvoicePaymentFailed_ActiveToPastDue(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_pf",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	payload := []byte(`{"id":"evt_pf","type":"invoice.payment_failed","data":{"object":{"customer":"cus_pf"}}}`)
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_pf", EventType: "invoice.payment_failed", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusPastDue, sub.Status)
}

// TestDispatch_InvoicePaymentActionRequired_PersistsHostedInvoiceURL verifies
// that dispatching invoice.payment_action_required with a hosted_invoice_url
// saves the URL on the subscription row.
func TestDispatch_InvoicePaymentActionRequired_PersistsHostedInvoiceURL(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_sca_url",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	stripeURL := "https://invoice.stripe.com/i/acct_123/test_xxx"
	payload := []byte(`{"id":"evt_sca_url","type":"invoice.payment_action_required","data":{"object":{"customer":"cus_sca_url","hosted_invoice_url":"` + stripeURL + `"}}}`)
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_sca_url", EventType: "invoice.payment_action_required", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.HostedInvoiceURL)
	require.Equal(t, stripeURL, *sub.HostedInvoiceURL)
	require.Equal(t, subscription.StatusPaymentActionRequired, sub.Status)
}

// TestDispatch_InvoicePaid_StampsFirstChargeAt_ClearsHostedURL verifies that
// dispatching invoice.paid stamps first_charge_at and clears hosted_invoice_url.
func TestDispatch_InvoicePaid_StampsFirstChargeAt_ClearsHostedURL(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	hostedURL := "https://invoice.stripe.com/i/acct_123/test_yyy"
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_paid",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		HostedInvoiceURL: &hostedURL,
	}).Error)

	payload := []byte(`{"id":"evt_paid","type":"invoice.paid","data":{"object":{"customer":"cus_paid"}}}`)
	d := dispatch.New(nil)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_paid", EventType: "invoice.paid", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.FirstChargeAt, "first_charge_at must be stamped after invoice.paid")
	require.Nil(t, sub.HostedInvoiceURL, "hosted_invoice_url must be cleared after invoice.paid")
}

// TestDispatch_InvoicePaid_FirstChargeAtIdempotent_SecondEventDoesNotAdvance
// verifies COALESCE semantics: a second invoice.paid does not overwrite
// the already-stamped first_charge_at value.
func TestDispatch_InvoicePaid_FirstChargeAtIdempotent_SecondEventDoesNotAdvance(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_paid2",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	d := dispatch.New(nil)
	dispatch1 := func(evtID string) {
		payload := []byte(`{"id":"` + evtID + `","type":"invoice.paid","data":{"object":{"customer":"cus_paid2"}}}`)
		require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
			EventID: evtID, EventType: "invoice.paid", Payload: payload,
		}))
	}

	dispatch1("evt_paid2a")
	var sub1 subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub1).Error)
	require.NotNil(t, sub1.FirstChargeAt)
	first := *sub1.FirstChargeAt

	dispatch1("evt_paid2b")
	var sub2 subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub2).Error)
	require.NotNil(t, sub2.FirstChargeAt)
	require.Equal(t, first.UTC().Truncate(time.Second), sub2.FirstChargeAt.UTC().Truncate(time.Second),
		"first_charge_at must not advance on second invoice.paid")
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
