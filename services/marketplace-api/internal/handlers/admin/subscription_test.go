package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TestComputeTrialCTA covers every branch of the CTA matrix that drives the
// admin trial banner. The matrix is small enough to enumerate exhaustively;
// the cases also document the contract for the frontend.
func TestComputeTrialCTA(t *testing.T) {
	cases := []struct {
		name    string
		status  subscription.SubscriptionStatus
		hasPM   bool
		days    int
		wantCTA string
	}{
		{"signup always pick_plan even with PM", subscription.StatusSignup, true, 80, "pick_plan"},
		{"signup always pick_plan w/o PM", subscription.StatusSignup, false, 30, "pick_plan"},
		{"trialing no PM mid-trial → add_card", subscription.StatusTrialing, false, 60, "add_card"},
		{"trialing no PM final day → add_card", subscription.StatusTrialing, false, 1, "add_card"},
		{"trialing has PM far from end → all_set", subscription.StatusTrialing, true, 60, "all_set"},
		{"trialing has PM 4d remaining → all_set", subscription.StatusTrialing, true, 4, "all_set"},
		{"trialing has PM 3d remaining → billing_imminent", subscription.StatusTrialing, true, 3, "billing_imminent"},
		{"trialing has PM 1d remaining → billing_imminent", subscription.StatusTrialing, true, 1, "billing_imminent"},
		{"trialing has PM 0d remaining → billing_imminent", subscription.StatusTrialing, true, 0, "billing_imminent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeTrialCTA(tc.status, tc.hasPM, tc.days); got != tc.wantCTA {
				t.Fatalf("computeTrialCTA(%q, %v, %d) = %q; want %q",
					tc.status, tc.hasPM, tc.days, got, tc.wantCTA)
			}
		})
	}
}

// TestEnrichTrialBanner_PopulatesFieldsForActiveTrial verifies that an
// in-trial subscription gets all banner fields populated and that
// days_remaining reflects the simulated wall-clock.
func TestEnrichTrialBanner_PopulatesFieldsForActiveTrial(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -75) // 75 days into a 90-day trial → 15 remaining

	sub := subscription.StoreSubscription{
		ID:                      uuid.New(),
		StoreID:                 uuid.New(),
		Plan:                    subscription.PlanStarter,
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: false,
		CreatedAt:               createdAt,
	}

	resp := SubscriptionResponse{}
	enrichTrialBanner(&resp, sub, now)

	if resp.TrialEndsAt == nil {
		t.Fatalf("trial_ends_at must be populated for trialing sub")
	}
	if resp.DaysRemainingInTrial == nil || *resp.DaysRemainingInTrial != 15 {
		t.Fatalf("days_remaining_in_trial: want 15, got %v", resp.DaysRemainingInTrial)
	}
	if resp.TrialCTA == nil || *resp.TrialCTA != "add_card" {
		t.Fatalf("trial_cta: want add_card, got %v", resp.TrialCTA)
	}
}

// TestEnrichTrialBanner_OmittedWhenNotInTrial verifies that subs in active /
// expired / past_due states don't get banner fields populated — the UI
// treats nil CTA as "hide banner", so populating these would falsely
// show countdowns to a user who's already on a paid plan.
func TestEnrichTrialBanner_OmittedWhenNotInTrial(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	for _, status := range []subscription.SubscriptionStatus{
		subscription.StatusActive,
		subscription.StatusExpired,
		subscription.StatusPastDue,
		subscription.StatusCancelScheduled,
		subscription.StatusStoreClosed,
	} {
		sub := subscription.StoreSubscription{
			ID:        uuid.New(),
			StoreID:   uuid.New(),
			Plan:      subscription.PlanStarter,
			Status:    status,
			CreatedAt: now.AddDate(0, 0, -45),
		}
		resp := SubscriptionResponse{}
		enrichTrialBanner(&resp, sub, now)
		if resp.TrialEndsAt != nil || resp.DaysRemainingInTrial != nil || resp.TrialCTA != nil {
			t.Fatalf("status %s: banner fields must be omitted, got ends=%v days=%v cta=%v",
				status, resp.TrialEndsAt, resp.DaysRemainingInTrial, resp.TrialCTA)
		}
	}
}

// TestEnrichTrialBanner_ClampsDaysRemainingAtZero verifies that an expired-
// but-not-yet-transitioned trial (race window between trial_end and the
// expiry cron) shows 0 days, never a negative count.
func TestEnrichTrialBanner_ClampsDaysRemainingAtZero(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -(trial.TrialDays + 5)) // expired 5 days ago

	sub := subscription.StoreSubscription{
		ID:        uuid.New(),
		StoreID:   uuid.New(),
		Plan:      subscription.PlanTrial,
		Status:    subscription.StatusTrialing, // expiry cron hasn't run yet
		CreatedAt: createdAt,
	}
	resp := SubscriptionResponse{}
	enrichTrialBanner(&resp, sub, now)

	if resp.DaysRemainingInTrial == nil || *resp.DaysRemainingInTrial != 0 {
		t.Fatalf("expected days_remaining clamped to 0, got %v", resp.DaysRemainingInTrial)
	}
}
