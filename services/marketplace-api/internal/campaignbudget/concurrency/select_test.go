package concurrency_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
)

// The acquirer is constructed directly now that mark8ly#234 removed the
// Redis implementation and the Select() indirection that chose between them.
func TestNewAdvisoryLockAcquirer_ReturnsAcquirer(t *testing.T) {
	var acq concurrency.SlotAcquirer = concurrency.NewAdvisoryLockAcquirer(nil)
	require.NotNil(t, acq, "constructor must never return nil")
}
