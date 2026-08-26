package dispatch

import (
	"context"
	"sync"
)

// DeferredSends collects work registered during webhook dispatch that must
// run AFTER the advisory-lock transaction commits.
//
// Why this exists: Dispatch runs inside subscription.WithAdvisoryLock, which
// holds pg_advisory_xact_lock on the store and one connection of a small pool
// for the whole callback. A provider HTTP call made in there (SendGrid, 15s,
// plus a possible Resend fallback) is charged against Stripe's 30s webhook
// budget and starves the pool. So handlers register the call here and the
// caller drains it once the transaction has committed.
//
// It is deliberately NOT a field on Dispatcher: one Dispatcher is shared by
// every concurrent webhook, so pending sends must be per-request. The
// collector travels in the request context instead.
type DeferredSends struct {
	mu    sync.Mutex
	sends []func() error
}

type deferredSendsKey struct{}

// WithDeferredSends returns a context carrying a fresh collector, plus the
// collector itself so the caller can drain it. Call it once per webhook,
// before entering the advisory lock.
func WithDeferredSends(ctx context.Context) (context.Context, *DeferredSends) {
	d := &DeferredSends{}
	return context.WithValue(ctx, deferredSendsKey{}, d), d
}

// deferredSendsFrom returns the collector carried by ctx, or nil when none
// was installed. Nil is a supported state: handlers fall back to sending
// inline rather than dropping the send on the floor.
func deferredSendsFrom(ctx context.Context) *DeferredSends {
	d, _ := ctx.Value(deferredSendsKey{}).(*DeferredSends)
	return d
}

// Add registers fn to run at drain time. Safe for concurrent use.
func (d *DeferredSends) Add(fn func() error) {
	if d == nil || fn == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sends = append(d.sends, fn)
}

// Run executes every registered send in registration order and returns the
// errors. It does not stop on the first error — one failed email must not
// suppress another — and it drains the queue so a second call is a no-op.
//
// Every error returned here is non-fatal by contract: callers log them and
// carry on. Returning one to Stripe would trigger a retry that re-fires
// every other side effect of the event.
func (d *DeferredSends) Run(ctx context.Context) []error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	pending := d.sends
	d.sends = nil
	d.mu.Unlock()

	var errs []error
	for _, fn := range pending {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := fn(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
