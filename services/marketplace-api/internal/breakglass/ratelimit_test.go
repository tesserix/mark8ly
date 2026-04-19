package breakglass

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter_CountsFailuresInWindow(t *testing.T) {
	rl := NewLoginRateLimiter()
	require.Equal(t, 1, rl.RecordFailure("ip-a"))
	require.Equal(t, 2, rl.RecordFailure("ip-a"))
	require.Equal(t, 3, rl.RecordFailure("ip-a"))
	require.Equal(t, 3, rl.Count("ip-a"))
}

func TestLoginRateLimiter_SeparateKeysSeparateCounts(t *testing.T) {
	rl := NewLoginRateLimiter()
	rl.RecordFailure("ip-a")
	rl.RecordFailure("ip-a")
	require.Equal(t, 1, rl.RecordFailure("ip-b"))
	require.Equal(t, 2, rl.Count("ip-a"))
	require.Equal(t, 1, rl.Count("ip-b"))
}

func TestLoginRateLimiter_ResetClearsBucket(t *testing.T) {
	rl := NewLoginRateLimiter()
	rl.RecordFailure("ip-a")
	rl.RecordFailure("ip-a")
	rl.Reset("ip-a")
	require.Equal(t, 0, rl.Count("ip-a"))
	require.Equal(t, 1, rl.RecordFailure("ip-a"))
}
