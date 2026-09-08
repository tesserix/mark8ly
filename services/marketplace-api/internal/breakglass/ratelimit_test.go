package breakglass

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter_CountsFailuresInWindow(t *testing.T) {
	rl := NewLoginRateLimiter()
	require.Equal(t, 1, rl.recordFailure("ip-a"))
	require.Equal(t, 2, rl.recordFailure("ip-a"))
	require.Equal(t, 3, rl.recordFailure("ip-a"))
	require.Equal(t, 3, rl.count("ip-a"))
}

func TestLoginRateLimiter_SeparateKeysSeparateCounts(t *testing.T) {
	rl := NewLoginRateLimiter()
	rl.recordFailure("ip-a")
	rl.recordFailure("ip-a")
	require.Equal(t, 1, rl.recordFailure("ip-b"))
	require.Equal(t, 2, rl.count("ip-a"))
	require.Equal(t, 1, rl.count("ip-b"))
}

func TestLoginRateLimiter_ResetClearsBucket(t *testing.T) {
	rl := NewLoginRateLimiter()
	rl.recordFailure("ip-a")
	rl.recordFailure("ip-a")
	rl.reset("ip-a")
	require.Equal(t, 0, rl.count("ip-a"))
	require.Equal(t, 1, rl.recordFailure("ip-a"))
}
