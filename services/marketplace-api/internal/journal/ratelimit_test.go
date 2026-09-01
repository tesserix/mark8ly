package journal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/journal"
)

func TestRateLimiter_AllowsUpToMaxWithinWindow(t *testing.T) {
	l := journal.NewRateLimiter()
	for i := 0; i < journal.SubscribeRateMax; i++ {
		require.True(t, l.Allow("1.2.3.4"), "attempt %d should be allowed", i+1)
	}
}

func TestRateLimiter_BlocksOnceMaxExceeded(t *testing.T) {
	l := journal.NewRateLimiter()
	for i := 0; i < journal.SubscribeRateMax; i++ {
		require.True(t, l.Allow("1.2.3.4"))
	}
	require.False(t, l.Allow("1.2.3.4"), "the request beyond the max must be blocked")
}

func TestRateLimiter_KeysAreIndependent(t *testing.T) {
	l := journal.NewRateLimiter()
	for i := 0; i < journal.SubscribeRateMax; i++ {
		require.True(t, l.Allow("1.2.3.4"))
	}
	require.False(t, l.Allow("1.2.3.4"))
	// A different client IP has its own, untouched bucket.
	require.True(t, l.Allow("5.6.7.8"))
}
