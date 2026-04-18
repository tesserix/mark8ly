package dunning_test

import (
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
)

func ptr[T any](v T) *T { return &v }

var now = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

func TestIsInRefundWindow_NilFirstCharge_ReturnsFalse(t *testing.T) {
	sub := &subscription.StoreSubscription{FirstChargeAt: nil}
	if dunning.IsInRefundWindow(sub, now) {
		t.Fatal("expected false for nil FirstChargeAt")
	}
}

func TestIsInRefundWindow_NilSub_ReturnsFalse(t *testing.T) {
	if dunning.IsInRefundWindow(nil, now) {
		t.Fatal("expected false for nil sub")
	}
}

func TestIsInRefundWindow_WithinWindow_True(t *testing.T) {
	firstCharge := now.Add(-7 * 24 * time.Hour)
	sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
	if !dunning.IsInRefundWindow(sub, now) {
		t.Fatal("expected true for 7 days since first charge (within 14-day window)")
	}
}

func TestIsInRefundWindow_OnBoundary_Excluded(t *testing.T) {
	// Exactly 14 days is NOT in the window (<14d required).
	firstCharge := now.Add(-dunning.RefundWindowDays * 24 * time.Hour)
	sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
	if dunning.IsInRefundWindow(sub, now) {
		t.Fatal("expected false at exactly 14-day boundary (< 14d required)")
	}
}

func TestIsInRefundWindow_BeyondWindow_False(t *testing.T) {
	firstCharge := now.Add(-30 * 24 * time.Hour)
	sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
	if dunning.IsInRefundWindow(sub, now) {
		t.Fatal("expected false for 30 days since first charge")
	}
}

func TestIsInRefundWindow_FutureFirstCharge_True(t *testing.T) {
	// Clock skew: firstChargeAt is in the future — duration is negative,
	// which is still < RefundWindowDays*24h.
	firstCharge := now.Add(1 * time.Hour)
	sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
	if !dunning.IsInRefundWindow(sub, now) {
		t.Fatal("expected true for future FirstChargeAt (clock-skew scenario, within window)")
	}
}
