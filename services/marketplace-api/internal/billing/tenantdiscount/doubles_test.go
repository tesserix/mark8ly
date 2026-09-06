package tenantdiscount_test

import (
	"context"
	"errors"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// fakeStripe is a hand-rolled stand-in for the StripeDiscounts port, matching
// this repository's convention of stubs written per package rather than a
// generated-mock framework (internal/billing/trial's *_test.go files do the
// same).
//
// It models what Stripe actually stores — a SET of coupons per subscription —
// rather than a call log, because "already attached" is a property of that
// state and every test that asserts an already_applied outcome depends on the
// double behaving idempotently the way the real helpers do.
type fakeStripe struct {
	mu sync.Mutex

	// attached[subID] is the set of coupon ids on that subscription.
	attached map[string]map[string]bool

	// failFor[subID] makes every call for that subscription fail. One
	// subscription failing while its siblings succeed is the per-store
	// isolation these tests exist to pin.
	failFor map[string]error

	adds    []string // "<subID>/<couponID>", in call order
	removes []string
	reads   []string
}

func newFakeStripe() *fakeStripe {
	return &fakeStripe{
		attached: map[string]map[string]bool{},
		failFor:  map[string]error{},
	}
}

// attach seeds a coupon as already present, standing in for a discount the
// subscription carried before this service ever ran — a merchant's own promo,
// or an earlier apply that this one is retrying.
func (f *fakeStripe) attach(subID, couponID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set(subID)[couponID] = true
}

func (f *fakeStripe) fail(subID string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failFor[subID] = err
}

func (f *fakeStripe) has(subID, couponID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.set(subID)[couponID]
}

func (f *fakeStripe) coupons(subID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.set(subID)))
	for id := range f.set(subID) {
		out = append(out, id)
	}
	return out
}

// set returns the coupon set for subID, creating it on first use. Callers
// must already hold f.mu.
func (f *fakeStripe) set(subID string) map[string]bool {
	if f.attached[subID] == nil {
		f.attached[subID] = map[string]bool{}
	}
	return f.attached[subID]
}

func (f *fakeStripe) SubscriptionHasDiscount(_ context.Context, subID, couponID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, subID+"/"+couponID)
	if err := f.failFor[subID]; err != nil {
		return false, err
	}
	return f.set(subID)[couponID], nil
}

func (f *fakeStripe) AddSubscriptionDiscount(_ context.Context, subID, couponID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, subID+"/"+couponID)
	if err := f.failFor[subID]; err != nil {
		return err
	}
	f.set(subID)[couponID] = true
	return nil
}

func (f *fakeStripe) RemoveSubscriptionDiscount(_ context.Context, subID, couponID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes = append(f.removes, subID+"/"+couponID)
	if err := f.failFor[subID]; err != nil {
		return err
	}
	delete(f.set(subID), couponID)
	return nil
}

// errStripeDown is the failure the fake injects. It carries no meaning to the
// service beyond "the Stripe call did not succeed".
var errStripeDown = errors.New("stripe is down")

// recordingAudit satisfies AuditWriter without a database. It is used by the
// unit tests, which never reach a transaction.
type recordingAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingAudit) EmitTx(_ context.Context, _ *gorm.DB, _ *gin.Context, ev audit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

// nilAuditWriter exists only so a TYPED nil can be assigned into the
// AuditWriter interface. Its method is never called — the constructor must
// refuse the value before anything can call it.
type nilAuditWriter struct{}

func (*nilAuditWriter) EmitTx(context.Context, *gorm.DB, *gin.Context, audit.Event) error {
	panic("nilAuditWriter.EmitTx must never be called")
}
