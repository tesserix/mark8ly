package tenantdiscount

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Errors returned by Apply and Remove themselves — the ones that refuse the
// whole fan-out before any store is touched. A per-store failure is NOT one of
// these: it lives in StoreResult.Err, because one store's failure must neither
// roll back nor block the others.
var (
	// ErrNoAuditWriter: constructed without an audit writer. tesserix-home#331
	// requires the audit row be written inside the transaction that applies
	// the change, so a Service that cannot write one has no legal behaviour —
	// "a write endpoint that cannot be attributed should not exist".
	ErrNoAuditWriter = errors.New("tenantdiscount: an audit writer is required")

	// ErrNoStripeClient: constructed without a Stripe client. Unlike
	// trial.Extender, where a nil Stripe field is a supported configuration
	// that refuses card-backed trials, every operation here IS a Stripe
	// operation; there is no local-only half to fall back to.
	ErrNoStripeClient = errors.New("tenantdiscount: a stripe client is required")

	ErrNoTenant = errors.New("tenantdiscount: a tenant id is required")
	ErrNoCoupon = errors.New("tenantdiscount: a coupon id is required")

	// ErrNoStores: the tenant owns no stores, so there is nothing to apply
	// the override to. Refused rather than returned as an empty success,
	// which would read to an operator as "done".
	ErrNoStores = errors.New("tenantdiscount: tenant owns no stores")

	// ErrStripeCall: Stripe was reached for and did not succeed, for ONE
	// store. That store's transaction rolled back and nothing was written for
	// it; its siblings are unaffected and the caller may retry.
	ErrStripeCall = errors.New("tenantdiscount: stripe call failed")

	// ErrStripeChangedAuditWriteFailed is the ONE divergence this design
	// accepts, and it is named here rather than left to fall out of ordering.
	//
	// The audit row is written LAST inside the store's transaction, so if
	// that insert (or the commit after it) fails, the transaction rolls back
	// — and rolling back cannot undo Stripe. The subscription keeps the
	// discount while no audit row records who applied it or why: a live
	// billing change with no attribution, which is precisely what
	// tesserix-home#331 exists to prevent.
	//
	// THE DECISION, STATED: an unattributable discount is rolled back
	// LOCALLY — the store is reported failed, not applied — rather than kept
	// silently by reporting success and letting the missing audit row go
	// unnoticed. We deliberately do NOT attempt to undo the Stripe side: a
	// compensating call is itself an unaudited billing change, it can fail
	// too, and it would delete a discount that a concurrent, correctly
	// audited apply may have just placed. Reconciliation is a human's, and
	// the log line and this error carry the three ids they need — the coupon,
	// the Stripe subscription and the Stripe customer.
	//
	// This is the same class as trial.ErrStripeAppliedLocalWriteFailed
	// (trial/extend.go:83-90) and must never be reported as a routine
	// database error.
	ErrStripeChangedAuditWriteFailed = errors.New("tenantdiscount: the stripe discount was changed but the audit row was not written")
)

// AuditDivergenceError carries the ids a human needs to reconcile a discount
// Stripe holds and no audit row explains. It exists for the same reason
// trial.TrialEndNotAfterStripeError does: the sentinel says WHICH failure this
// is, and the struct says which objects to go and look at.
type AuditDivergenceError struct {
	// Op is "apply" or "remove" — which direction Stripe moved in.
	Op string

	StoreID              uuid.UUID
	CouponID             string
	StripeSubscriptionID string
	StripeCustomerID     string

	// Cause is the audit insert's or the commit's own error.
	Cause error
}

func (e *AuditDivergenceError) Error() string {
	return fmt.Sprintf(
		"%s: %s coupon %s on stripe subscription %s (customer %s, store %s): %v",
		ErrStripeChangedAuditWriteFailed, e.Op, e.CouponID,
		e.StripeSubscriptionID, e.StripeCustomerID, e.StoreID, e.Cause)
}

// Unwrap returns both the sentinel and the cause so errors.Is finds either.
// A caller branching on ErrStripeChangedAuditWriteFailed gets its distinct
// status; a caller (or a test) looking for the underlying insert failure can
// still reach it.
func (e *AuditDivergenceError) Unwrap() []error {
	return []error{ErrStripeChangedAuditWriteFailed, e.Cause}
}
