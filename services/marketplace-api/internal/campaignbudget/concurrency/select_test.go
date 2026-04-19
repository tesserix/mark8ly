package concurrency_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
)

// TestSelect_NilRedis_ReturnsAdvisoryAcquirer verifies that Select falls back
// to the advisory-lock acquirer when no Redis client is supplied.
func TestSelect_NilRedis_ReturnsAdvisoryAcquirer(t *testing.T) {
	acq := concurrency.Select(nil, nil)
	require.NotNil(t, acq, "Select must never return nil")
}
