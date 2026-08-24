package platformadmin

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/stretchr/testify/require"
)

// TestConsoleHiddenStatusesIsExactlyTheTwoTeardownStates makes any change
// to consoleHiddenStatuses a deliberate, visible edit: adding a status to
// the deny list without updating this test fails it, and so does removing
// one of the two teardown states from the deny list.
func TestConsoleHiddenStatusesIsExactlyTheTwoTeardownStates(t *testing.T) {
	require.Equal(t, map[subscription.SubscriptionStatus]bool{
		subscription.StatusPendingHardDelete: true,
		subscription.StatusHardDeleted:       true,
	}, consoleHiddenStatuses)
}
