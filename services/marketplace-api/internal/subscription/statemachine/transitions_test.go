package statemachine_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

func TestValidTransitions_MatchSpecExactly(t *testing.T) {
	want := []struct {
		from, to subscription.SubscriptionStatus
	}{
		{subscription.StatusSignup, subscription.StatusTrialing},
		{subscription.StatusTrialing, subscription.StatusActive},
		{subscription.StatusTrialing, subscription.StatusExpired},
		{subscription.StatusActive, subscription.StatusPastDue},
		{subscription.StatusActive, subscription.StatusPaymentActionRequired},
		{subscription.StatusActive, subscription.StatusCancelScheduled},
		{subscription.StatusPastDue, subscription.StatusActive},
		{subscription.StatusPastDue, subscription.StatusExpired},
		{subscription.StatusPaymentActionRequired, subscription.StatusActive},
		{subscription.StatusPaymentActionRequired, subscription.StatusPastDue},
		{subscription.StatusCancelScheduled, subscription.StatusActive},
		{subscription.StatusCancelScheduled, subscription.StatusExpired},
		{subscription.StatusExpired, subscription.StatusActive},
		{subscription.StatusExpired, subscription.StatusStoreClosed},
		{subscription.StatusStoreClosed, subscription.StatusActive},
		{subscription.StatusStoreClosed, subscription.StatusPendingHardDelete},
		{subscription.StatusPendingHardDelete, subscription.StatusHardDeleted},
	}

	for _, tc := range want {
		require.True(t, statemachine.IsValidTransition(tc.from, tc.to),
			"expected %s → %s to be valid", tc.from, tc.to)
	}

	require.Len(t, statemachine.AllValidTransitions(), len(want),
		"transition table has extra/missing transitions vs §17.2")
}

func TestInvalidTransitions_Rejected(t *testing.T) {
	// The spec forbids direct expired → pending_hard_delete (must go via store_closed).
	require.False(t, statemachine.IsValidTransition(
		subscription.StatusExpired, subscription.StatusPendingHardDelete))

	// No path back from hard_deleted.
	require.False(t, statemachine.IsValidTransition(
		subscription.StatusHardDeleted, subscription.StatusActive))

	// No signup → active shortcut (trial gate is deliberate).
	require.False(t, statemachine.IsValidTransition(
		subscription.StatusSignup, subscription.StatusActive))
}

func TestPaymentActionRequired_IsNotInReadOnlySet(t *testing.T) {
	// Council finding #3: payment_action_required merchants keep full admin.
	require.NotContains(t,
		statemachine.ReadOnlyStatuses(),
		subscription.StatusPaymentActionRequired)
}

func TestReadOnlyStatusSet_MatchesSpec(t *testing.T) {
	require.ElementsMatch(t,
		[]subscription.SubscriptionStatus{
			subscription.StatusExpired,
			subscription.StatusStoreClosed,
			subscription.StatusPendingHardDelete,
		},
		statemachine.ReadOnlyStatuses())
}
