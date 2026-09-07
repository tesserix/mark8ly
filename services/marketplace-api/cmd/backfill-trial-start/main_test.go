package main

import (
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestBackfilledTrialEnd_DatesFromTheStoreNotTheBackfill(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	signedUp := now.AddDate(0, 0, -30)

	end, expired := backfilledTrialEnd(signedUp, now)

	want := trial.EndsAt(subscription.StoreSubscription{CreatedAt: signedUp})
	if !end.Equal(want) {
		t.Fatalf("end = %s, want %s", end, want)
	}
	if expired {
		t.Error("a store 30 days old was reported as already expired")
	}
	// The failure this guards: a fresh 90 days handed to an account that
	// signed up a month ago.
	if end.Equal(trial.EndsAt(subscription.StoreSubscription{CreatedAt: now})) {
		t.Fatal("trial was dated from the backfill instead of from the store")
	}
}

// The expensive direction to get wrong silently: an estate full of dormant
// accounts each granted a brand-new free trial.
func TestBackfilledTrialEnd_ReportsAnOldStoreAsExpired(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	signedUp := now.AddDate(-1, 0, 0)

	end, expired := backfilledTrialEnd(signedUp, now)

	if !expired {
		t.Fatal("a store a year old was not reported as expired")
	}
	if !end.Before(now) {
		t.Errorf("end = %s, want a date in the past", end)
	}
}

// The boundary has to match trial.Extendable's, which uses !After(now) — an
// end exactly at now has passed. If these disagree, a backfilled row reads as
// live here and is refused there.
func TestBackfilledTrialEnd_AnEndExactlyNowHasPassed(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	signedUp := now.Add(-trial.TrialDays * 24 * time.Hour) // exactly one trial length ago

	end, expired := backfilledTrialEnd(signedUp, now)

	if !end.Equal(now) {
		t.Fatalf("end = %s, want exactly now", end)
	}
	if !expired {
		t.Error("an end exactly at now was reported as still running")
	}
}
