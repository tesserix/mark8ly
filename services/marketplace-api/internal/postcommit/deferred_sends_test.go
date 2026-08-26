package postcommit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// A panicking unit of work must not escape Run — Run's contract is that its
// caller (a webhook handler whose transaction has already committed) can
// never be failed by deferred work. It must also not take its siblings down
// with it: they are already off the queue, so abandoning them loses them
// permanently and silently.
func TestRun_PanicIsRecoveredAndSiblingsStillRun(t *testing.T) {
	ctx, d := WithDeferredSends(context.Background())

	ran := 0
	d.Add(func() error { ran++; panic("provider client blew up") })
	d.Add(func() error { ran++; return errors.New("second failed") })
	d.Add(func() error { ran++; return nil })

	var errs []error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Run let a panic escape: %v", r)
			}
		}()
		errs = d.Run(ctx)
	}()

	require.Equal(t, 3, ran, "a panicking unit of work must not abandon its siblings")
	require.Len(t, errs, 2, "want the panic and the failure, got %v", errs)
	require.Contains(t, errs[0].Error(), "panicked", "the panic must be reported as an error")
	require.EqualError(t, errs[1], "second failed", "the sibling's own error must survive")
}

// Run drains the queue, so a second call does nothing. Documented in Run's
// doc comment as a known limit; pinned here so it stays intentional.
func TestRun_IsIdempotent(t *testing.T) {
	ctx, d := WithDeferredSends(context.Background())
	ran := 0
	d.Add(func() error { ran++; return nil })

	d.Run(ctx)
	d.Run(ctx)

	require.Equal(t, 1, ran, "work ran more than once across two Run calls")
}

// Add reports whether a collector was present, so producers can tell the
// difference between "deferred" and "nobody is going to drain this".
func TestAdd_ReportsWhetherACollectorWasInstalled(t *testing.T) {
	require.False(t, Add(context.Background(), func() error { return nil }),
		"Add claimed a bare context carried a collector")

	ctx, d := WithDeferredSends(context.Background())
	require.True(t, Add(ctx, func() error { return nil }),
		"Add did not find the installed collector")
	require.Empty(t, d.Run(ctx))
}

// A nil collector and a nil unit of work are both supported states.
func TestNilSafety(t *testing.T) {
	var d *DeferredSends
	d.Add(func() error { return nil })
	require.Empty(t, d.Run(context.Background()), "nil collector must be a no-op")

	_, live := WithDeferredSends(context.Background())
	live.Add(nil)
	require.Empty(t, live.Run(context.Background()), "a nil unit of work must be ignored")
}
