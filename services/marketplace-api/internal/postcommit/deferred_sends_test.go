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
	d.Add(func(context.Context) error { ran++; panic("provider client blew up") })
	d.Add(func(context.Context) error { ran++; return errors.New("second failed") })
	d.Add(func(context.Context) error { ran++; return nil })

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
	d.Add(func(context.Context) error { ran++; return nil })

	d.Run(ctx)
	d.Run(ctx)

	require.Equal(t, 1, ran, "work ran more than once across two Run calls")
}

// Add reports whether a collector was present, so producers can tell the
// difference between "deferred" and "nobody is going to drain this".
func TestAdd_ReportsWhetherACollectorWasInstalled(t *testing.T) {
	require.False(t, Add(context.Background(), func(context.Context) error { return nil }),
		"Add claimed a bare context carried a collector")

	ctx, d := WithDeferredSends(context.Background())
	require.True(t, Add(ctx, func(context.Context) error { return nil }),
		"Add did not find the installed collector")
	require.Empty(t, d.Run(ctx))
}

// A nil collector and a nil unit of work are both supported states.
func TestNilSafety(t *testing.T) {
	var d *DeferredSends
	d.Add(func(context.Context) error { return nil })
	require.Empty(t, d.Run(context.Background()), "nil collector must be a no-op")

	_, live := WithDeferredSends(context.Background())
	live.Add(nil)
	require.Empty(t, live.Run(context.Background()), "a nil unit of work must be ignored")
}

// Run must NOT consult the request context. Both drain sites pass the Gin
// request context, and a transaction that commits at T+29.5s under lock
// contention is drained after Stripe's 30s budget has already cancelled it.
// The unit is off the queue by then, its transaction has committed, and the
// redelivery that would recover it is suppressed by the event-level
// idempotency key — so honouring the cancellation loses the work outright,
// in exactly the high-latency case the deferral was built for.
func TestRun_DrainsAfterTheRequestContextIsCancelled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, d := WithDeferredSends(parent)

	ran := false
	var unitCtxErr error
	var hadDeadline bool
	require.True(t, Add(ctx, func(c context.Context) error {
		ran = true
		unitCtxErr = c.Err()
		_, hadDeadline = c.Deadline()
		return nil
	}))

	// The request dies between the commit and the drain.
	cancel()

	errs := d.Run(ctx)

	require.True(t, ran, "a cancelled request silently dropped already-committed work")
	require.Empty(t, errs, "Run reported the request cancellation as a deferred-work failure")
	require.NoError(t, unitCtxErr, "the unit was handed the cancelled request context")
	require.True(t, hadDeadline, "the detached context must still be bounded by unitTimeout")
}

// Values must survive the detach — only cancellation and the deadline are
// dropped — so tracing and logging carried on the request context still reach
// the deferred unit.
func TestRun_DetachedContextKeepsValues(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "trace-1"))
	ctx, d := WithDeferredSends(parent)

	var got any
	d.Add(func(c context.Context) error { got = c.Value(key{}); return nil })
	cancel()

	require.Empty(t, d.Run(ctx))
	require.Equal(t, "trace-1", got, "the detach dropped the request's values")
}

// Add must not claim it took work it discarded: (*DeferredSends).Add drops a
// nil unit, so the package-level Add reporting true would tell a producer its
// user-visible work is scheduled when nothing will ever run it.
func TestAdd_ReportsFalseForANilUnit(t *testing.T) {
	ctx, d := WithDeferredSends(context.Background())
	require.False(t, Add(ctx, nil), "Add claimed a nil unit of work was taken")
	require.Empty(t, d.Run(ctx))
}
