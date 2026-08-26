// Package postcommit carries work that must run AFTER a database transaction
// commits, not inside it.
//
// It exists so that two packages which must not depend on each other — the
// webhook HTTP handlers that own the transaction boundary, and the dispatch
// handlers that run inside it — can hand work across that boundary. Neither
// imports the other; both import this.
//
// The API is deliberately generic: a unit of deferred work is a plain
// func(context.Context) error. Nothing here knows about email, Stripe, or
// any other specific side effect.
package postcommit

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// unitTimeout bounds one unit of deferred work.
//
// Run detaches from request cancellation (see Run), so without a deadline of
// its own a wedged unit would run forever on a goroutine nobody is waiting
// for. The budget covers what the motivating unit actually does: a SendGrid
// call (15s client timeout) plus a possible Resend fallback (another 15s),
// plus the claim round-trip, with headroom. It is a backstop, not a target —
// this package deliberately knows nothing about who the unit calls.
const unitTimeout = 45 * time.Second

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
	sends []func(context.Context) error
}

type deferredSendsKey struct{}

// WithDeferredSends returns a context carrying a fresh collector, plus the
// collector itself so the caller can drain it. Call it once per webhook,
// before entering the advisory lock.
//
// The collector is installed on the caller's own (cancelable) context on
// purpose: the transaction it guards must still abort when the request is
// cancelled. Only the drain phase is detached, and Run does that itself.
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
// taken. False means the work is NOT going to run here — either no collector
// was installed, or fn is nil. The caller owns that case and must decide what
// to do, which for anything user-visible means running the work inline rather
// than dropping it.
//
// The nil-fn case reports false for the same reason: (*DeferredSends).Add
// discards a nil unit, so claiming it was taken would be a lie a caller could
// act on.
func Add(ctx context.Context, fn func(context.Context) error) bool {
	d := fromContext(ctx)
	if d == nil || fn == nil {
		return false
	}
	d.Add(fn)
	return true
}

// Add registers fn to run at drain time. Safe for concurrent use. Producers
// holding only a context should call the package-level Add instead.
//
// fn takes the context Run hands it, NOT one captured from the request. That
// is the whole point of the signature: a captured request context is cancelled
// the moment the client hangs up, and every unit built over it would then fail
// at drain time — after the transaction has already committed and there is
// nothing left to roll back or retry.
func (d *DeferredSends) Add(fn func(context.Context) error) {
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
// Cancellation: ctx is used for its VALUES only. Each unit runs on
// context.WithoutCancel(ctx) plus a fresh unitTimeout, so a request cancelled
// between the commit and the drain — a Stripe or route timeout firing at T+30s
// on a transaction that committed at T+29.5s — cannot take the work with it.
// It must not: the unit is already off the queue, its transaction has already
// committed, and the redelivery that would recover it is suppressed by the
// event-level idempotency key. Consulting ctx.Err() here lost the send outright,
// and lost it precisely in the high-latency case this deferral exists to fix.
//
// One known limit, left alone deliberately: Add after Run is silently dropped.
// Run swaps the slice out, so work registered afterwards has no drain to run
// in. Register everything before the transaction commits.
func (d *DeferredSends) Run(ctx context.Context) []error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	pending := d.sends
	d.sends = nil
	d.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	// Values (trace ids, loggers) survive; cancellation and any inherited
	// deadline do not.
	base := context.WithoutCancel(ctx)

	var errs []error
	for _, fn := range pending {
		if err := runOne(base, fn); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// runOne calls fn on its own timeout-bounded context, converting a panic into
// an error. Without the recover a single panicking unit of work would abandon
// every sibling still queued — they are already off the queue and would never
// be run or reported — and would propagate out of Run to fail the caller's
// request, which is exactly the non-fatal guarantee this package makes.
func runOne(base context.Context, fn func(context.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(base, unitTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("postcommit: deferred work panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return fn(ctx)
}
