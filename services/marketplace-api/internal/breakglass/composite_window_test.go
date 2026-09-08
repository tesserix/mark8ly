package breakglass_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// stubWindow records what it was asked and can be made to fail.
type stubWindow struct {
	resets int
	err    error
}

func (s *stubWindow) RecordFailure(context.Context, breakglass.LoginKey) (int, error) {
	return 0, nil
}
func (s *stubWindow) Reset(context.Context, breakglass.LoginKey) error {
	s.resets++
	return s.err
}
func (s *stubWindow) Count(context.Context, breakglass.LoginKey) (int, error) { return 0, nil }

func key() breakglass.LoginKey {
	return breakglass.LoginKey{IPHash: []byte("0123456789abcdef-and-more")}
}

// The property clear-lockout depends on: EVERY window is cleared, not just
// until the first one errors. A partial clear is what leaves an operator
// believing an IP is unlocked while one window still refuses it.
func TestCompositeWindowResetsEveryWindowEvenAfterAFailure(t *testing.T) {
	failing := &stubWindow{err: errors.New("db down")}
	healthy := &stubWindow{}

	err := breakglass.NewCompositeWindow(failing, healthy).Reset(context.Background(), key())

	require.Error(t, err, "a failed window must be reported, never swallowed")
	require.Equal(t, 1, failing.resets)
	require.Equal(t, 1, healthy.resets,
		"a window after a failing one must still be cleared")
}

func TestCompositeWindowReportsSuccessWhenEveryWindowClears(t *testing.T) {
	a, b := &stubWindow{}, &stubWindow{}
	require.NoError(t, breakglass.NewCompositeWindow(a, b).Reset(context.Background(), key()))
	require.Equal(t, 1, a.resets)
	require.Equal(t, 1, b.resets)
}

// A nil window is dropped rather than panicking, so a caller can pass an
// optional window without a nil check at every construction site.
func TestCompositeWindowIgnoresNilWindows(t *testing.T) {
	only := &stubWindow{}
	require.NoError(t, breakglass.NewCompositeWindow(nil, only, nil).Reset(context.Background(), key()))
	require.Equal(t, 1, only.resets)
}

// Counting through the composite is refused rather than guessed at. If this
// ever starts returning a number, the login handler's choice of which window
// to believe has been silently taken away from it.
func TestCompositeWindowRefusesToCount(t *testing.T) {
	c := breakglass.NewCompositeWindow(&stubWindow{})

	_, err := c.RecordFailure(context.Background(), key())
	require.Error(t, err)

	_, err = c.Count(context.Background(), key())
	require.Error(t, err)
}

// The two implementations must agree on how a LoginKey maps to the in-memory
// bucket, or clear-lockout resets a bucket the login path never reads — the
// hazard LoginRateLimitKey was extracted to prevent.
func TestLoginKeyBucketMatchesLoginRateLimitKey(t *testing.T) {
	k := key()
	require.Equal(t, breakglass.LoginRateLimitKey(k.IPHash), k.Bucket())
}

// The exported interface methods, which the older table-driven tests reach
// through unexported helpers. Without this, a signature that compiles but
// counts wrongly would ship.
func TestInMemoryWindowSatisfiesTheInterfaceContract(t *testing.T) {
	var w breakglass.LoginWindow = breakglass.NewLoginRateLimiter()
	ctx := context.Background()

	n, err := w.RecordFailure(ctx, key())
	require.NoError(t, err)
	require.Equal(t, 1, n, "RecordFailure returns the count INCLUDING this failure")

	n, err = w.RecordFailure(ctx, key())
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = w.Count(ctx, key())
	require.NoError(t, err)
	require.Equal(t, 2, n)

	require.NoError(t, w.Reset(ctx, key()))
	n, err = w.Count(ctx, key())
	require.NoError(t, err)
	require.Zero(t, n)
}
