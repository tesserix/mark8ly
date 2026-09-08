package breakglass

import (
	"context"
	"errors"
)

// CompositeWindow fans a LoginWindow operation across several windows.
//
// It exists for the CLEAR path, not the counting one (#846). Clearing a
// lockout has to clear every window that could re-earn it: with the durable
// window in place, clearing the lockout row while leaving the attempt rows
// behind means the very next failure counts as the fourth and re-locks the
// IP immediately — an operator would see clear-lockout succeed and the IP
// stay locked, which is the exact failure `LoginRateLimitKey`'s comment was
// written to prevent one level down.
//
// Counting deliberately does NOT go through this type. The login handler
// needs to know WHICH window answered so it can prefer the durable count and
// fall back to the in-memory one; a composite that merged them would have to
// invent a rule for disagreement, and the honest rule is the caller's.
type CompositeWindow struct{ windows []LoginWindow }

// NewCompositeWindow returns a window that applies Reset to each of windows,
// in order. Nil entries are dropped so a caller can pass an optional window
// without a nil check at every site.
func NewCompositeWindow(windows ...LoginWindow) *CompositeWindow {
	live := make([]LoginWindow, 0, len(windows))
	for _, w := range windows {
		if w != nil {
			live = append(live, w)
		}
	}
	return &CompositeWindow{windows: live}
}

// Reset clears every window, and does NOT stop at the first error.
//
// A partial clear is the dangerous outcome here — it is what leaves an
// operator believing an IP is unlocked while one window still refuses it —
// so every window is attempted and the errors are joined. The caller decides
// what a partial failure means; this type will not hide one.
func (c *CompositeWindow) Reset(ctx context.Context, k LoginKey) error {
	var errs []error
	for _, w := range c.windows {
		if err := w.Reset(ctx, k); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RecordFailure is refused rather than implemented.
//
// Stamping a failure on several windows and returning one number requires
// choosing which number is the answer, and that choice belongs to the login
// handler (see recordAcrossWindows), which has the context to prefer the
// durable count and log when it had to fall back. Returning, say, the max
// here would silently make a database outage look like a healthy count.
func (c *CompositeWindow) RecordFailure(context.Context, LoginKey) (int, error) {
	return 0, errors.New("breakglass: CompositeWindow is for Reset only; count through a specific window")
}

// Count is refused for the same reason RecordFailure is.
func (c *CompositeWindow) Count(context.Context, LoginKey) (int, error) {
	return 0, errors.New("breakglass: CompositeWindow is for Reset only; count through a specific window")
}

var _ LoginWindow = (*CompositeWindow)(nil)
