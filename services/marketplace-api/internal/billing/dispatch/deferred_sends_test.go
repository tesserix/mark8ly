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
	"github.com/mark8ly/marketplace-api/internal/postcommit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// visibilityEmailClient is an email.Client stub that, when Send is invoked,
// asks a SEPARATE connection whether store_subscriptions.first_charge_at is
// visible yet.
//
// That question is the whole test. probe is the pool handle, not the locked
// transaction, so it reads its own MVCC snapshot: while the webhook
// transaction is still open the stamped first_charge_at is uncommitted and
// therefore invisible, and it only becomes visible once the transaction
// commits. "Was it visible at Send time?" is thus a direct, non-circular
// observation of which side of the commit the send happened on — an
// assertion that the send merely happened would pass either way.
type visibilityEmailClient struct {
	probe *gorm.DB
	subID uuid.UUID

	calls           int
	committedAtSend bool
	probeErr        error
}

func (c *visibilityEmailClient) Send(_ context.Context, _ email.TemplateID, to string, _ map[string]any) error {
	if err := email.ValidateRecipient(to); err != nil {
		return err
	}
	c.calls++
	var n int64
	// A plain SELECT never blocks on the writer's row lock — it just reads
	// the pre-transaction snapshot — so this cannot deadlock the test.
	if err := c.probe.Raw(
		`SELECT count(*) FROM store_subscriptions WHERE id = ? AND first_charge_at IS NOT NULL`,
		c.subID).Scan(&n).Error; err != nil {
		c.probeErr = err
		return nil
	}
	c.committedAtSend = n == 1
	return nil
}

// TestInvoicePaid_TrialBilled_SentAfterTransactionCommits pins that the
// provider HTTP call is made outside the advisory-lock transaction. Inside
// it, the call holds pg_advisory_xact_lock on the store and one connection
// of a small pool across a 15s SendGrid timeout, against Stripe's 30s
// webhook budget.
func TestInvoicePaid_TrialBilled_SentAfterTransactionCommits(t *testing.T) {
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
		// FirstChargeAt deliberately nil — this is the first charge.
	}
	require.NoError(t, db.Create(&sub).Error)

	client := &visibilityEmailClient{probe: db, subID: sub.ID}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	// Exactly the production shape: collector installed before the lock,
	// drained after it returns.
	ctx, deferred := postcommit.WithDeferredSends(context.Background())
	eventID := "evt_" + uuid.NewString()[:12]
	payload := []byte(`{"id":"` + eventID + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	require.NoError(t, subscription.WithAdvisoryLock(ctx, db, storeID, func(tx *gorm.DB) error {
		return d.Dispatch(ctx, tx, webhookevents.StripeWebhookEvent{
			EventID: eventID, EventType: "invoice.paid", Payload: payload,
		})
	}))

	require.Zero(t, client.calls,
		"the provider call was made inside the advisory-lock transaction")

	require.Empty(t, deferred.Run(ctx), "the deferred send must succeed")
	require.NoError(t, client.probeErr)
	require.Equal(t, 1, client.calls, "draining the collector must send exactly one email")
	require.True(t, client.committedAtSend,
		"first_charge_at was still invisible to a second connection at Send time, "+
			"so the send is still inside the webhook transaction")
}
