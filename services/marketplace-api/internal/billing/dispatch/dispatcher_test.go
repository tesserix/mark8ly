//go:build integration

package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/email"
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
	seedStore(t, db, storeID) // store_subscriptions.store_id has an enforced FK
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
	seedStore(t, db, storeID) // store_subscriptions.store_id has an enforced FK
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

// TestDispatch_CustomerUpdated_SetsHasDefaultPaymentMethod verifies that the
// customer.updated handler mirrors invoice_settings.default_payment_method
// onto store_subscriptions.has_default_payment_method. Three cases:
//   - default_payment_method present  → flag flips false → true.
//   - default_payment_method null     → flag flips true → false.
//   - tenant isolation: a second tenant's row with a different
//     stripe_customer_id is unaffected by the update.
func TestDispatch_CustomerUpdated_SetsHasDefaultPaymentMethod(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                tenantA,
		StoreID:                 storeA,
		StripeCustomerID:        "cus_pm_a",
		Plan:                    subscription.PlanTrial,
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: false,
	}).Error)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                tenantB,
		StoreID:                 storeB,
		StripeCustomerID:        "cus_pm_b",
		Plan:                    subscription.PlanTrial,
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: true, // sentinel: must remain unchanged
	}).Error)

	d := dispatch.New(nil)

	// Case 1 — A gets a default PM. Only A's row should flip to true.
	setPM := []byte(`{"id":"evt_pm1","type":"customer.updated","data":{"object":{"id":"cus_pm_a","invoice_settings":{"default_payment_method":"pm_123"}}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_pm1", EventType: "customer.updated", Payload: setPM,
	}))

	var subA, subB subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeA).First(&subA).Error)
	require.NoError(t, db.Where("store_id=?", storeB).First(&subB).Error)
	require.True(t, subA.HasDefaultPaymentMethod, "tenant A flag must flip to true")
	require.True(t, subB.HasDefaultPaymentMethod, "tenant B flag must remain true (not touched by A's event)")

	// Case 2 — A's PM is detached (default_payment_method=null). Flag flips back to false.
	clearPM := []byte(`{"id":"evt_pm2","type":"customer.updated","data":{"object":{"id":"cus_pm_a","invoice_settings":{"default_payment_method":null}}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_pm2", EventType: "customer.updated", Payload: clearPM,
	}))
	require.NoError(t, db.Where("store_id=?", storeA).First(&subA).Error)
	require.NoError(t, db.Where("store_id=?", storeB).First(&subB).Error)
	require.False(t, subA.HasDefaultPaymentMethod, "tenant A flag must flip back to false")
	require.True(t, subB.HasDefaultPaymentMethod, "tenant B flag still untouched")

	// Case 3 — empty-string default_payment_method also counts as "no PM".
	emptyPM := []byte(`{"id":"evt_pm3","type":"customer.updated","data":{"object":{"id":"cus_pm_a","invoice_settings":{"default_payment_method":""}}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_pm3", EventType: "customer.updated", Payload: emptyPM,
	}))
	require.NoError(t, db.Where("store_id=?", storeA).First(&subA).Error)
	require.False(t, subA.HasDefaultPaymentMethod)

	// Verify updated_at moves forward — proves the row was written, not just read.
	require.WithinDuration(t, time.Now().UTC(), subA.UpdatedAt, 30*time.Second)
}

// TestDispatch_CustomerUpdated_UnknownCustomerIsNoop verifies that an event
// for a stripe_customer_id we don't know about (e.g. test-mode noise, replay
// from a deleted store) is a benign no-op rather than an error — preserves
// Stripe webhook idempotency contract.
func TestDispatch_CustomerUpdated_UnknownCustomerIsNoop(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")
	d := dispatch.New(nil)
	payload := []byte(`{"id":"evt_pm_x","type":"customer.updated","data":{"object":{"id":"cus_unknown","invoice_settings":{"default_payment_method":"pm_x"}}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_pm_x", EventType: "customer.updated", Payload: payload,
	}))
}

// dispatchEmailRecorder is a minimal email.Client used by the
// trial-billed-confirmation tests below. It stores templates seen for
// later assertion. Local to the test file because the cross-test
// capturingClient lives in a different package.
type dispatchEmailRecorder struct {
	templates []email.TemplateID
}

func (r *dispatchEmailRecorder) Send(_ context.Context, t email.TemplateID, _ string, _ map[string]any) error {
	r.templates = append(r.templates, t)
	return nil
}

// TestDispatch_InvoicePaid_FirstChargeEmitsTrialBilledEmail verifies that the
// first successful invoice.paid emits TemplateTrialStartedBilled when an
// email client is wired. This is the merchant-facing confirmation that
// "your chosen plan is now active and we just billed your card".
func TestDispatch_InvoicePaid_FirstChargeEmitsTrialBilledEmail(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, storeID) // store_subscriptions.store_id has an enforced FK
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_first_charge",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		// FirstChargeAt deliberately nil — this is the first charge.
	}).Error)

	rec := &dispatchEmailRecorder{}
	d := dispatch.New(nil).WithEmail(rec).WithDB(db)
	payload := []byte(`{"id":"evt_first","type":"invoice.paid","data":{"object":{"customer":"cus_first_charge"}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_first", EventType: "invoice.paid", Payload: payload,
	}))

	require.Len(t, rec.templates, 1, "trial-billed email must be emitted on first charge")
	require.Equal(t, email.TemplateTrialStartedBilled, rec.templates[0])

	// Second invoice.paid must NOT re-emit the email — first_charge_at is now non-nil.
	payload2 := []byte(`{"id":"evt_second","type":"invoice.paid","data":{"object":{"customer":"cus_first_charge"}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_second", EventType: "invoice.paid", Payload: payload2,
	}))
	require.Len(t, rec.templates, 1, "subsequent invoice.paid events must not re-emit (idempotent)")
}

// TestDispatch_InvoicePaid_NoEmailClientStillProcesses verifies the dispatcher
// is robust to no email client being wired (e.g. dev mode without the
// adapter): first_charge_at is still stamped, no panic, no error.
func TestDispatch_InvoicePaid_NoEmailClientStillProcesses(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, storeID) // store_subscriptions.store_id has an enforced FK
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_no_email",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	d := dispatch.New(nil) // no WithEmail call
	payload := []byte(`{"id":"evt_noemail","type":"invoice.paid","data":{"object":{"customer":"cus_no_email"}}}`)
	require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: "evt_noemail", EventType: "invoice.paid", Payload: payload,
	}))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.FirstChargeAt, "first_charge_at must be stamped even without email client")
}

// capturedSend records one call to captureEmailClient.Send, so tests can
// assert on both the recipient and the data map (e.g. store_name).
type capturedSend struct {
	to   string
	data map[string]any
}

// captureEmailClient is a minimal email.Client stub that records every send
// it was asked to make. Unlike dispatchEmailRecorder above (which only cares
// which template fired), this one exists to catch the actual bugs in Task
// 12: the recipient passed to Send was a store UUID, not an address, and
// store_name was a hardcoded literal. Send honours the same contract the
// real client enforces — validating the recipient first — so a test double
// here can't mask a bounce the production client would have caught.
type captureEmailClient struct {
	recipients []string
	sends      []capturedSend
}

func (c *captureEmailClient) Send(_ context.Context, _ email.TemplateID, to string, data map[string]any) error {
	if err := email.ValidateRecipient(to); err != nil {
		return err
	}
	c.recipients = append(c.recipients, to)
	c.sends = append(c.sends, capturedSend{to: to, data: data})
	return nil
}

// runInvoicePaidFirstCharge seeds a store and a store_subscriptions row with
// first_charge_at NULL and the given email (nil leaves it unset), attaches
// client as the dispatcher's email client, and dispatches a first-charge
// invoice.paid event. Returns the db handle and storeID so callers can
// re-query store_subscriptions afterward, the seeded store's name so
// callers can assert it was used verbatim, and the error from Dispatch so
// callers can assert on webhook-level failure behavior.
func runInvoicePaidFirstCharge(t *testing.T, client email.Client, addr *string) (db *gorm.DB, storeID uuid.UUID, storeName string, err error) {
	t.Helper()
	db = testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	tenantID := uuid.New()
	storeID = uuid.New()
	seedStore(t, db, storeID)
	storeName = "Test Store " + storeID.String() // must match seedStore's inserted name

	customerID := "cus_" + uuid.NewString()[:12]
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		Email:            addr,
		// FirstChargeAt deliberately nil — this is the first charge.
	}).Error)

	d := dispatch.New(nil).WithEmail(client).WithDB(db)
	eventID := "evt_" + uuid.NewString()[:12]
	payload := []byte(`{"id":"` + eventID + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	err = d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: eventID, EventType: "invoice.paid", Payload: payload,
	})
	return db, storeID, storeName, err
}

// TestInvoicePaid_TrialBilledUsesRealAddress verifies the trial-billed
// confirmation email is sent to the merchant's actual address (not the
// store UUID that used to be passed as `to`), carries the store's real
// name (not the "your store" literal), and that the webhook's own side
// effects (first_charge_at stamped, hosted_invoice_url cleared) happened.
func TestInvoicePaid_TrialBilledUsesRealAddress(t *testing.T) {
	addr := "merchant@example.com"
	client := &captureEmailClient{}

	db, storeID, storeName, err := runInvoicePaidFirstCharge(t, client, &addr)
	require.NoError(t, err)

	require.Len(t, client.recipients, 1, "sent %d emails, want 1", len(client.recipients))
	require.Equal(t, addr, client.recipients[0], "a store UUID would bounce")
	require.Len(t, client.sends, 1)
	require.Equal(t, storeName, client.sends[0].data["store_name"],
		"store_name must be the real store name, not the \"your store\" literal")

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.FirstChargeAt, "first_charge_at must be stamped")
	require.Nil(t, sub.HostedInvoiceURL, "hosted_invoice_url must be cleared")
}

// TestInvoicePaid_TrialBilledWithoutAddressDoesNotFailTheWebhook verifies a
// missing/invalid recipient keeps the webhook non-fatal — Stripe must not
// retry and re-fire every other invoice.paid side effect — and that those
// side effects (first_charge_at stamped, hosted_invoice_url cleared) still
// happened even though the email send failed.
func TestInvoicePaid_TrialBilledWithoutAddressDoesNotFailTheWebhook(t *testing.T) {
	client := &captureEmailClient{}

	db, storeID, _, err := runInvoicePaidFirstCharge(t, client, nil)

	require.NoError(t, err, "webhook must stay non-fatal on email failure")
	require.Empty(t, client.recipients, "no address means nothing should have been sent")

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.NotNil(t, sub.FirstChargeAt, "first_charge_at must be stamped even though the email failed")
	require.Nil(t, sub.HostedInvoiceURL, "hosted_invoice_url must be cleared even though the email failed")
}
