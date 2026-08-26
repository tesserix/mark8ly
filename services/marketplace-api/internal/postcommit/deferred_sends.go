// Package postcommit carries work that must run AFTER a database transaction
// commits, not inside it.
//
// It exists so that two packages which must not depend on each other — the
// webhook HTTP handlers that own the transaction boundary, and the dispatch
// handlers that run inside it — can hand work across that boundary. Neither
// imports the other; both import this.
//
// The API is deliberately generic: a unit of deferred work is a plain
// func() error. Nothing here knows about email, Stripe, or any other
// specific side effect.
package postcommit

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

// DeferredSends collects work registered while a transaction is open that
// must run only once that transaction has committed.
//
// The motivating case: Stripe webhook dispatch runs inside
// subscription.WithAdvisoryLock, which holds pg_advisory_xact_lock on the
// store and one connection of a small pool for the whole callback. A provider
// HTTP call made in there (SendGrid, 15s, plus a possible Resend fallback) is
// charged against Stripe's 30s webhook budget and starves the pool. So the
// handler registers the call here and the transaction's owner drains it after
// the commit.
//
// It is deliberately NOT a field on any long-lived struct: dispatchers and
// handlers are shared by every concurrent request, so pending work must be
// per-request. The collector travels in the request context instead.
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

// fromContext returns the collector carried by ctx, or nil when none was
// installed. It stays unexported: registering work is what producers need,
// and Add below is the whole of that API.
func fromContext(ctx context.Context) *DeferredSends {
	d, _ := ctx.Value(deferredSendsKey{}).(*DeferredSends)
	return d
}

// Add registers fn on the collector carried by ctx and reports whether it was
// taken. False means no collector was installed — the caller owns that case
// and must decide what to do, which for anything user-visible means running
// the work inline rather than dropping it.
func Add(ctx context.Context, fn func() error) bool {
	d := fromContext(ctx)
	if d == nil {
		return false
	}
	d.Add(fn)
	return true
}

// Add registers fn to run at drain time. Safe for concurrent use. Producers
// holding only a context should call the package-level Add instead.
func (d *DeferredSends) Add(fn func() error) {
	if d == nil || fn == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sends = append(d.sends, fn)
}

// Run executes every registered unit of work in registration order and
// returns the errors. It does not stop on the first error — one failure must
// not suppress the rest — and it drains the queue so a second call is a no-op.
//
// Every error returned here is non-fatal by contract: the transaction has
// already committed, so there is nothing left to roll back. Callers log them
// and carry on — surfacing one as a request failure would, in the webhook
// case, trigger a Stripe retry that re-fires every other side effect. Run
// upholds that contract absolutely: a panicking unit of work is recovered and
// reported as an error rather than being allowed to escape and fail the
// request, and its siblings still run.
//
// Two known limits, both unreachable with a single registered unit of work
// and both left alone deliberately — tightening the queue semantics would
// cost more than it buys:
//
//   - Add after Run is silently dropped. Run swaps the slice out, so work
//     registered afterwards has no drain to run in. Register everything
//     before the transaction commits.
//   - The ctx.Err() break abandons the units not yet reached, and reports a
//     single context error however many were skipped. They were already
//     removed from the queue, so they are lost rather than deferred again.
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
		if err := runOne(fn); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// runOne calls fn, converting a panic into an error. Without this a single
// panicking unit of work would abandon every sibling still queued — they are
// already off the queue and would never be run or reported — and would
// propagate out of Run to fail the caller's request, which is exactly the
// non-fatal guarantee this package makes.
func runOne(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("postcommit: deferred work panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}
