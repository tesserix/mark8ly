//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedFirstChargeSub inserts a store + a subscription with first_charge_at
// NULL, returning the db handle, the row and its Stripe customer id.
func seedFirstChargeSub(t *testing.T) (*gorm.DB, subscription.StoreSubscription, string) {
	t.Helper()
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	storeID := uuid.New()
	seedStore(t, db, storeID)
	addr := "merchant@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		Email:            &addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	return db, sub, customerID
}

func dispatchInvoicePaid(t *testing.T, d *dispatch.Dispatcher, db *gorm.DB, customerID string) error {
	t.Helper()
	eventID := "evt_" + uuid.NewString()[:12]
	payload := []byte(`{"id":"` + eventID + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	return d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID: eventID, EventType: "invoice.paid", Payload: payload,
	})
}

// The confirmation is sent inside the locked webhook transaction, and
// invoice.paid runs a handler chain — a later handler error, or a failed
// commit, rolls first_charge_at back to NULL while the email has already
// left. Stripe then retries and sees a "first charge" again. The claim is
// written on a non-transactional handle precisely so it survives that
// rollback; this test reproduces the rollback by resetting first_charge_at.
func TestInvoicePaid_TrialBilled_NotResentAfterRollback(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))
	require.Len(t, client.recipients, 1, "first charge must send one confirmation")

	// Simulate the transaction rolling back after the email went out.
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET first_charge_at = NULL WHERE id = ?`, sub.ID).Error)

	// Stripe retries (or redelivers as a fresh event).
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Len(t, client.recipients, 1,
		"a second trial-billed confirmation was sent after a rollback")
}

// The claim is the guarantee, so it must exist after a successful send —
// keyed by the constant first_charge period, i.e. once per subscription.
func TestInvoicePaid_TrialBilled_ClaimsRow(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	d := dispatch.New(nil).WithEmail(&captureEmailClient{}).WithDB(db)
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM billing_email_sends
		 WHERE subscription_id = ? AND template_key = ? AND period_key = 'first_charge'`,
		sub.ID, string(email.TemplateTrialStartedBilled)).Scan(&n).Error)
	require.EqualValues(t, 1, n, "no claim row written — at-most-once is unenforced")
}

// Without a claim store there is no way to make the send at-most-once, so it
// is skipped rather than risked. The webhook's own side effects still run.
func TestInvoicePaid_TrialBilled_NoClaimStoreSkipsSend(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client) // no WithDB
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Empty(t, client.recipients, "sent without a claim store — cannot be at-most-once")

	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.FirstChargeAt, "webhook side effects must still run")
}
