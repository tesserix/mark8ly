package tenantdiscount

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NewSubscriptionInput identifies a subscription that has just been created in
// Stripe and not yet been offered the tenant's standing override.
type NewSubscriptionInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID

	// StripeSubscriptionID is the subscription Stripe just created. Blank is
	// refused: there is nothing to attach a discount to, and the caller
	// reaching here with one has a bug worth seeing rather than a no-op.
	StripeSubscriptionID string
}

// ApplyToNewSubscription attaches the tenant's live recorded override to a
// subscription that has just been created, and is the half of #660 that makes
// the grant cover FUTURE stores rather than only the ones that existed when an
// operator pressed the button.
//
// # The caller must treat every error as non-fatal
//
// This is called from the two subscription-creation paths
// (trial.Subscriber.subscribeInTx and
// planchange.Orchestrator.executeInitialSubscription). A discount that cannot
// be applied must never stop a merchant subscribing: the failure costs the
// tenant a discount they can be given again by hand, while a refusal costs
// them the subscription itself. Both callers log and continue, and nothing
// here returns an error the caller is expected to act on synchronously.
//
// # It touches no transaction of the caller's
//
// Every read and write below goes through the Service's own handle, never a
// transaction the caller passed in — deliberately, and not for tidiness. A
// failed statement poisons a Postgres transaction: every subsequent statement
// on it fails with "current transaction is aborted". Writing the audit row on
// the caller's transaction would therefore turn an audit failure into a failed
// subscription creation, which is exactly the outcome "non-fatal" forbids.
//
// The cost is stated plainly: the audit row here is NOT bound to the
// transaction that created the subscription, unlike the one Apply writes. If
// the caller's transaction rolls back after this returns, a row records a
// discount applied to a subscription whose local row never landed. The Stripe
// subscription itself is in the same position — CreateSubscription is not
// rolled back either — so the audit row describes the state Stripe is really
// in, which is the more useful of the two.
//
// # Idempotency
//
// Creation paths retry. A coupon already on the subscription reports
// OutcomeAlreadyApplied and sends nothing, exactly as Apply does.
func (s *Service) ApplyToNewSubscription(ctx context.Context, in NewSubscriptionInput) (Outcome, error) {
	if s == nil {
		return "", ErrNilService
	}
	if in.TenantID == uuid.Nil {
		return "", ErrNoTenant
	}
	if in.StripeSubscriptionID == "" {
		return "", ErrNoStripeSubscription
	}

	live, err := loadLiveOverride(ctx, s.db, in.TenantID)
	if err != nil {
		return "", err
	}
	if live == nil {
		// The common case by a wide margin: most tenants hold no override.
		// Reported under its own name rather than as "already applied" so a
		// caller's log can tell "nothing to do" from "already done".
		return OutcomeNoOverride, nil
	}

	sctx, cancel := context.WithTimeout(ctx, stripeCallTimeout)
	defer cancel()

	outcome, err := applyOp.run(sctx, s.stripe, in.StripeSubscriptionID, live.StripeCouponID)
	if err != nil {
		return "", fmt.Errorf("%w: apply coupon %s on subscription %s: %w",
			ErrStripeCall, live.StripeCouponID, in.StripeSubscriptionID, err)
	}

	res := StoreResult{
		StoreID:              in.StoreID,
		StripeSubscriptionID: in.StripeSubscriptionID,
		Outcome:              outcome,
	}
	auditErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.writeAudit(ctx, tx, Input{
			TenantID: in.TenantID,
			CouponID: live.StripeCouponID,
			Reason:   reasonSubscriptionCreated,
		}, applyOp, res)
	})
	if auditErr == nil {
		return outcome, nil
	}

	if !res.Changed() {
		// Nothing was sent to Stripe, so this is an ordinary write failure,
		// not the divergence. The outcome is still returned: the coupon is on
		// the subscription, which is what the caller asked about.
		return outcome, fmt.Errorf("tenantdiscount: write audit row for new subscription %s: %w",
			in.StripeSubscriptionID, auditErr)
	}

	// Stripe was changed and no audit row records it — the same divergence
	// ErrStripeChangedAuditWriteFailed names on the operator path, reached
	// here without an operator to report it to. Logged with the ids a human
	// needs, because the returned error goes to a caller whose contract is to
	// discard it.
	//
	// StripeCustomerID is absent from both the error and the log line: this
	// path is handed a subscription id by its caller and never reads the
	// local subscription row, so it does not know the customer. The
	// subscription id resolves to it in Stripe.
	div := &AuditDivergenceError{
		Op:                   applyOp.name,
		StoreID:              in.StoreID,
		CouponID:             live.StripeCouponID,
		StripeSubscriptionID: in.StripeSubscriptionID,
		Cause:                auditErr,
	}
	s.logger.Error("tenantdiscount: standing override applied to a new subscription but the audit row was not written",
		"coupon_id", live.StripeCouponID,
		"stripe_subscription_id", in.StripeSubscriptionID,
		"store_id", in.StoreID.String(),
		"tenant_id", in.TenantID.String(),
		"err", auditErr)
	return outcome, div
}

// reasonSubscriptionCreated is the reason recorded on an audit row this path
// writes. The operator endpoint makes a reason mandatory and passes the
// operator's own words; there is no operator here, so the row states what
// caused the application instead of leaving the field empty.
const reasonSubscriptionCreated = "the tenant's standing platform override, applied when this subscription was created"
