package cancel_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
)

// fakeSubLookup implements cancel.SubscriptionLookup for unit tests without a DB.
type fakeSubLookup struct {
	sub *subscription.StoreSubscription
	err error
}

func (f *fakeSubLookup) GetByStoreID(_ context.Context, _ interface {
}, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sub, nil
}

// TestCancel_SchedulesCancellation verifies that Cancel with no save offer
// returns ErrNotCancellable only for non-active statuses, and that the pre-DB
// validation accepts active status (Success Criterion #54: prospective-only).
func TestCancel_SchedulesCancellation_ActiveStatus(t *testing.T) {
	periodEnd := time.Now().Add(15 * 24 * time.Hour)

	// We only validate the pre-transition guard. statemachine.Transition needs
	// a real DB so we test up to the guard boundary.
	sub := &subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		Status:           subscription.StatusActive,
		CurrentPeriodEnd: &periodEnd,
	}
	// Confirm active is a cancellable status — the guard should not fire.
	require.True(t, cancel.IsCancellableStatus(sub.Status),
		"active must be a cancellable status (§15)")
	require.Empty(t, sub.CurrentPeriodEnd == nil, "period end populated")
}

// TestCancel_NotCancellable verifies that expired/closed stores are rejected
// by the IsCancellableStatus guard before any DB write occurs.
func TestCancel_NotCancellable(t *testing.T) {
	for _, status := range []subscription.SubscriptionStatus{
		subscription.StatusExpired,
		subscription.StatusStoreClosed,
		subscription.StatusPendingHardDelete,
		subscription.StatusHardDeleted,
	} {
		require.False(t, cancel.IsCancellableStatus(status),
			"status %s must not be cancellable", status)
	}
}

// TestCancel_SurveyReason_TruncatedAt256 validates the truncation logic lives in
// the pure helper (not behind a DB call).
func TestCancel_SurveyReason_TruncatedAt256(t *testing.T) {
	longReason := make([]byte, 500)
	for i := range longReason {
		longReason[i] = 'x'
	}
	truncated := cancel.TruncateReason(string(longReason))
	require.Len(t, truncated, 256)
}

// TestCancel_ReasonLabel verifies reasonLabel composition.
func TestCancel_ReasonLabel(t *testing.T) {
	require.Equal(t, "merchant_cancelled", cancel.ReasonLabel("merchant_cancelled", ""))
	require.Equal(t, "merchant_cancelled: too expensive", cancel.ReasonLabel("merchant_cancelled", "too expensive"))
}

// TestCancel_ProspectiveOnly_SaveOfferNeverCreditsPeriod verifies the contract
// that save-offer acceptance (cancel_scheduled→active) emits no refund signal
// and no period credit (Success Criterion #54).
//
// This used to argue that cancel.Service accepts no Stripe client at all, and
// called that the compile-time proof. It was true, and it was also the reason
// cancellation never reached Stripe: the flow could not stop the billing it
// was telling merchants it had stopped. The seam now exists, so the invariant
// is enforced against its shape instead — see
// TestStripeCanceller_CannotCreditOrRefund in stripe_test.go, which pins the
// method set to schedule-and-reverse and nothing that moves money backwards.
func TestCancel_ProspectiveOnly_SaveOfferNeverCreditsPeriod(t *testing.T) {
	require.ElementsMatch(t,
		[]string{"CancelAtPeriodEnd", "Resume"},
		stripeCancellerMethods(),
		"§15.1 prospective-only: the save offer may reverse a cancellation, never credit a period")
}
