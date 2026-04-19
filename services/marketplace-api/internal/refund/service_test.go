package refund

import (
	"testing"
	"time"
)

// Tests for the 14-day cooling-off gate logic (§8 pattern E).
// Service integration is covered by integration tests (T15).

func TestCoolingOffWindow_Open(t *testing.T) {
	// Charge 13 days ago — window still open.
	chargeCreated := time.Now().UTC().Add(-13 * 24 * time.Hour)
	now := time.Now().UTC()
	closed := now.After(chargeCreated.Add(coolingOffWindow))
	if closed {
		t.Error("expected 14-day window to still be open for charge 13 days ago")
	}
}

func TestCoolingOffWindow_Closed(t *testing.T) {
	// Charge 15 days ago — window closed.
	chargeCreated := time.Now().UTC().Add(-15 * 24 * time.Hour)
	now := time.Now().UTC()
	closed := now.After(chargeCreated.Add(coolingOffWindow))
	if !closed {
		t.Error("expected 14-day window to be closed for charge 15 days ago")
	}
}

func TestCoolingOffWindow_ExactlyAtBoundary(t *testing.T) {
	// At exactly 14 days: now.After(chargeCreated + 14d) is false only if
	// now == chargeCreated+14d. In practice the sub-millisecond execution
	// means now is slightly after, so this tests boundary precision.
	// We use an explicit offset: charge at -(14d + 1s) → closed.
	chargeCreated := time.Now().UTC().Add(-(14*24*time.Hour + time.Second))
	now := time.Now().UTC()
	closed := now.After(chargeCreated.Add(coolingOffWindow))
	if !closed {
		t.Error("expected window to be closed when charge is 14d+1s old")
	}
}

func TestCoolingOffWindow_JustInsideBoundary(t *testing.T) {
	// Charge at -(14d - 1min) → still open.
	chargeCreated := time.Now().UTC().Add(-(14*24*time.Hour - time.Minute))
	now := time.Now().UTC()
	closed := now.After(chargeCreated.Add(coolingOffWindow))
	if closed {
		t.Error("expected window to still be open when charge is 14d-1min old")
	}
}
