//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// trialEndCaptureStripe is a StripeAPI test double dedicated to this file.
// It is named distinctly from the package's existing fakeStripe (declared in
// subscribe_integration_test.go under the same build tag) so both types can
// coexist without a redeclaration error.
type trialEndCaptureStripe struct {
	lastTrialEnd int64
}

func (f *trialEndCaptureStripe) CreateSubscription(_ context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	f.lastTrialEnd = in.TrialEnd
	return &billingstripe.Subscription{ID: "sub_trialend_capture"}, nil
}

func (f *trialEndCaptureStripe) PriceIDFor(_ context.Context, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string, _ subscription.PriceTier) (string, error) {
	return "price_trialend_capture", nil
}

// TestSubscribe_TrialEndSentToStripeIsEffectiveEnd proves the trial_end wired
// into the Stripe CreateSubscription call by the subscribe path is the
// EFFECTIVE end (trial.EndsAt), not created_at + TrialDays. This is a
// real-money value: Stripe bills the merchant on whatever date this call
// sends, so the assertion checks the exact integer rather than merely that
// CreateSubscription was called.
//
// This is the only assertion anywhere on the value sent to Stripe by the
// subscribe path. It requires the `integration` build tag and a real
// Postgres at TEST_DATABASE_URL (see pkg/testdb) — a plain `go test ./...`
// does not build or run it.
func TestSubscribe_TrialEndSentToStripeIsEffectiveEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	// seedExpiringRow sets CreatedAt = trialEndsAt - TrialDays, so the
	// derived (created_at + TrialDays) end would equal the argument below —
	// we then override TrialEndsAt to a clearly distinct value via opts so
	// the two calculations diverge and the test actually exercises EndsAt.
	derivedAnchor := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	extended := time.Date(2027, 1, 17, 8, 30, 0, 0, time.UTC)
	require.NotEqual(t, derivedAnchor.Unix(), extended.Unix(),
		"fixture must distinguish the extended value from the derived one")

	row := seedExpiringRow(t, db, derivedAnchor, func(r *subscription.StoreSubscription) {
		r.Status = subscription.StatusSignup
		r.StripeCustomerID = "cus_trialend_capture"
		r.TrialEndsAt = &extended
	})

	fake := &trialEndCaptureStripe{}
	s := trial.NewSubscriber(db, fake, nil)

	res, err := s.Subscribe(context.Background(), trial.SubscribeInput{
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		Plan:     subscription.PlanStarter,
		Period:   subscription.PeriodMonthly,
		Currency: "usd",
	})
	require.NoError(t, err)

	want := trial.EndsAt(row).Unix()
	require.Equal(t, extended.UTC().Unix(), want,
		"fixture sanity check: EndsAt must resolve to the extended override")
	require.Equal(t, want, res.TrialEndUnix)
	require.Equal(t, want, fake.lastTrialEnd,
		"trial_end sent to Stripe must be the effective end (trial.EndsAt), not created_at + TrialDays")
}
