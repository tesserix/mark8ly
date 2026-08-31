package storefront

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultTimelineDescription_PartiallyFulfilledHasHumanCopy pins task-2
// FIX 3: MarkPartiallyFulfilled (internal/order/service.go) writes a
// status-changed event with no "description" field in its payload, so
// every successful two-group order falls through to
// defaultTimelineDescription's kind-string fallback. Before this fix that
// switch had no case for "partially_fulfilled", so the customer-facing
// storefront timeline rendered the raw event kind string instead of human
// copy.
func TestDefaultTimelineDescription_PartiallyFulfilledHasHumanCopy(t *testing.T) {
	got := defaultTimelineDescription("partially_fulfilled")
	require.NotEqual(t, "partially_fulfilled", got,
		"the raw event kind must never reach the customer-facing timeline")
	require.Equal(t, "Part of your order has shipped.", got)
}

// TestPartiallyFulfilled_IsNotCustomerHidden guards the other half of FIX
// 3's instruction: a customer whose parcel shipped must SEE the event, not
// have it silently filtered by customerHiddenEventKinds the way
// merchant-only operational kinds are.
func TestPartiallyFulfilled_IsNotCustomerHidden(t *testing.T) {
	for _, hidden := range customerHiddenEventKinds {
		require.NotEqual(t, "partially_fulfilled", hidden,
			"partially_fulfilled must remain visible on the customer timeline")
	}
}
