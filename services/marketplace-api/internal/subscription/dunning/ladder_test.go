package dunning_test

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
)

// TestDecideStep exercises the full decision matrix for every status and
// boundary condition without requiring a database.

func TestDecideStep_PastDueDay0_Skip(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusPastDue, 0, false)
	if !d.Skip {
		t.Fatal("expected Skip at day 0")
	}
}

func TestDecideStep_PastDueDay7_Skip(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusPastDue, 7, false)
	if !d.Skip {
		t.Fatal("expected Skip at day 7 (threshold is day 8)")
	}
}

func TestDecideStep_PastDueDay8_NotInWindow_TransitionExpired(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusPastDue, 8, false)
	if d.Skip {
		t.Fatal("expected no Skip at day 8, not in refund window")
	}
	if d.Target != subscription.StatusExpired {
		t.Fatalf("expected target Expired, got %q", d.Target)
	}
	if d.Reason != "dunning_day_8" {
		t.Fatalf("expected reason dunning_day_8, got %q", d.Reason)
	}
}

func TestDecideStep_PastDueDay8_InWindow_SuppressRefundWindow(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusPastDue, 8, true)
	if !d.Skip {
		t.Fatal("expected Skip when in refund window")
	}
	if !d.SuppressRefundWindow {
		t.Fatal("expected SuppressRefundWindow=true when in refund window")
	}
}

func TestDecideStep_PastDueDay30_InWindow_StillSuppress(t *testing.T) {
	// Refund window guard takes priority over day count.
	d := dunning.DecideStep(subscription.StatusPastDue, 30, true)
	if !d.Skip {
		t.Fatal("expected Skip: refund window beats day count")
	}
	if !d.SuppressRefundWindow {
		t.Fatal("expected SuppressRefundWindow=true at day 30 in window")
	}
}

func TestDecideStep_ExpiredDay13_Skip(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusExpired, 13, false)
	if !d.Skip {
		t.Fatal("expected Skip at expired day 13 (threshold is day 14)")
	}
}

func TestDecideStep_ExpiredDay14_TransitionStoreClosed(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusExpired, 14, false)
	if d.Skip {
		t.Fatal("expected no Skip at expired day 14")
	}
	if d.Target != subscription.StatusStoreClosed {
		t.Fatalf("expected target StoreClosed, got %q", d.Target)
	}
	if d.Reason != "dunning_day_22" {
		t.Fatalf("expected reason dunning_day_22, got %q", d.Reason)
	}
}

func TestDecideStep_StoreClosedDay89_Skip(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusStoreClosed, 89, false)
	if !d.Skip {
		t.Fatal("expected Skip at store_closed day 89 (threshold is day 90)")
	}
}

func TestDecideStep_StoreClosedDay90_TransitionPendingDelete(t *testing.T) {
	d := dunning.DecideStep(subscription.StatusStoreClosed, 90, false)
	if d.Skip {
		t.Fatal("expected no Skip at store_closed day 90")
	}
	if d.Target != subscription.StatusPendingHardDelete {
		t.Fatalf("expected target PendingHardDelete, got %q", d.Target)
	}
	if d.Reason != "dunning_day_112" {
		t.Fatalf("expected reason dunning_day_112, got %q", d.Reason)
	}
}

func TestDecideStep_OtherStatus_Skip(t *testing.T) {
	statuses := []subscription.SubscriptionStatus{
		subscription.StatusActive,
		subscription.StatusTrialing,
		subscription.StatusSignup,
		subscription.StatusCancelScheduled,
		subscription.StatusPaymentActionRequired,
		subscription.StatusPendingHardDelete,
	}
	for _, s := range statuses {
		d := dunning.DecideStep(s, 999, false)
		if !d.Skip {
			t.Fatalf("expected Skip for status %q", s)
		}
	}
}
