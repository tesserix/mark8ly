//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestE2E_Criterion46_CardDay45_ChargeDay90 asserts the spec §28 criterion #46
// narrative: a merchant who signs up on day 0 and adds their card on day 45
// gets their first charge deferred to day 90 (45 days in the future from the
// card-add moment), NOT charged immediately and NOT charged at day 135.
//
// The test drives the full trial.Subscribe flow end-to-end against the real
// advisory-lock + repo path, using a fake Stripe that records the trial_end
// sent in the CreateSubscription call. The assertion compares the recorded
// trial_end against the arithmetic anchor (signup_date + 90d), which must
// equal (card_add_day + 45d) when the card is added on day 45.
func TestE2E_Criterion46_CardDay45_ChargeDay90(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	// Day 0: merchant signs up. CreatedAt stands in for signup_date.
	day0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_criterion_46",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day0,
	})

	// Day 45: merchant returns and adds a card via POST /billing/subscription.
	// Subscribe is called at this moment; the implementation computes
	// trial_end from row.CreatedAt, not from now(), so the 45-day-late
	// card-add still charges on day 90.
	fake := defaultFakeStripe()
	subscriber := trial.NewSubscriber(db, fake, nil)

	res, err := subscriber.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)

	// Day 90 is the expected first-charge moment: signup + 90d.
	day90 := day0.Add(trial.TrialDays * 24 * time.Hour)

	// The trial_end value Stripe receives — and the one we return to the caller —
	// must equal day 90. This is the core criterion #46 assertion.
	assert.Equal(t, day90.Unix(), res.TrialEndUnix,
		"trial_end returned to caller must equal signup + 90d regardless of card-add timing")
	assert.Equal(t, day90.Unix(), fake.lastTrialEnd,
		"trial_end sent to Stripe must be signup + 90d, not card-add + 90d")

	// Defense-in-depth: confirm the Stripe call happened exactly once (no
	// immediate follow-up charge), and the store row was updated with the
	// returned Stripe subscription ID.
	assert.Equal(t, 1, fake.createSubCalls, "exactly one Stripe subscription created")

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", row.StoreID).First(&updated).Error)
	require.NotNil(t, updated.StripeSubscriptionID, "stripe_subscription_id persisted")
	assert.Equal(t, "sub_test_123", *updated.StripeSubscriptionID)

	// Row status stays at signup — Subscribe does not mutate status; the
	// webhook will transition signup → trialing when Stripe notifies us.
	assert.Equal(t, subscription.StatusSignup, updated.Status,
		"Subscribe must not flip status — webhook owns that transition")
}
