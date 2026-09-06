// Package tenantdiscount applies and removes a console-minted Stripe coupon
// across every store a tenant owns.
//
// The console mints the coupon and records the grant (tesserix-home#331,
// migration 0047); it cannot apply it, because the Stripe customer lives here.
// This package is the domain half of that: given a tenant and a coupon id, put
// the coupon on — or take it off — each of that tenant's store subscriptions,
// and report what happened to each one.
//
// # Why the fan-out is per store and not per tenant
//
// store_subscriptions is UNIQUE (store_id) with a separate tenant_id
// (migration 000015), so a tenant with several stores has several
// subscriptions and several Stripe customers. #660 says "the tenant's
// subscription customer", singular; it is not. The grant is a standing
// property of the TENANT, so it fans out over all of the tenant's stores.
//
// # Why each store gets its own transaction
//
// trial/extend.go puts the Stripe call INSIDE the transaction, holding
// SELECT ... FOR UPDATE on the subscription row across the network call and
// bounding it with a timeout. Its reasoning (extend.go:270-280) is that Stripe
// must move first and be the source of truth: a Stripe failure rolls back and
// writes nothing locally, while a failure AFTER Stripe leaves Stripe ahead,
// which is the safe direction.
//
// This package follows that, but PER STORE. One transaction spanning N stores
// would hold N row locks across N Stripe round-trips, and one store's failure
// would roll back the rest. trial/extend.go never faced this because it is
// single-store. Each store here gets its own transaction, so one store's
// failure neither rolls back nor blocks the others, and the per-store report
// is the honest unit.
package tenantdiscount

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Audit actions this package writes. Exported because the handler's tests and
// the console's audit filters both name them, and a literal duplicated in
// three places drifts.
//
// Present tense, deliberately, against the past-tense convention of
// "order.cancelled" and "subscription.plan_changed": those rows are only ever
// written when the thing happened. A row here is written for every store
// including the ones where nothing was applied — pending, no_subscription —
// so "applied" would be a false record. What happened is in
// metadata.outcome.
const (
	ActionApply  = "billing.tenant_discount.apply"
	ActionRemove = "billing.tenant_discount.remove"
)

// resourceType is the audit row's resource_type. The resource is the store's
// subscription; the row's resource_id is the store id, matching how the
// subscription events in internal/audit already identify one.
const resourceType = "subscription"

// stripeCallTimeout bounds how long a store_subscriptions row lock is held
// across the external call. Mirrors trial/extend.go:139 — the lock is
// deliberate (it removes the window in which the row could change while
// Stripe is in flight) and this ceiling keeps "deliberate" from becoming
// "indefinite". Per store: the locks are never held concurrently, so the
// ceiling is per round-trip, not per fan-out.
const stripeCallTimeout = 10 * time.Second

// StripeDiscounts is the subset of Stripe this package needs, declared here
// rather than imported as a concrete type so the fan-out can be tested without
// a live Stripe and so the dependency points inward. StripeAdapter implements
// it over *billingstripe.Client.
type StripeDiscounts interface {
	// SubscriptionHasDiscount reports whether the coupon is already on the
	// subscription. Asked separately because Add reports "attached it" and
	// "it was already there" identically, and those are different outcomes
	// in the report.
	SubscriptionHasDiscount(ctx context.Context, subID, couponID string) (bool, error)

	// AddSubscriptionDiscount attaches the coupon, preserving every discount
	// already on the subscription — including a merchant's own promo.
	AddSubscriptionDiscount(ctx context.Context, subID, couponID string) error

	// RemoveSubscriptionDiscount removes the discount created from the coupon
	// and leaves the rest of the array untouched.
	RemoveSubscriptionDiscount(ctx context.Context, subID, couponID string) error
}

// AuditWriter is the audit surface this package needs: an insert that joins
// the CALLER's transaction. *audit.Emitter satisfies it. Nothing here may use
// Emit or EmitSync — both write on the emitter's own handle, so their row
// commits whatever this transaction goes on to do, which is the property
// EmitTx exists to remove.
type AuditWriter interface {
	EmitTx(ctx context.Context, tx *gorm.DB, c *gin.Context, ev audit.Event) error
}

// Config groups Service construction params.
type Config struct {
	DB     *gorm.DB
	Stripe StripeDiscounts
	Audit  AuditWriter
	Logger *slog.Logger
}

// Service applies and removes a tenant-wide discount.
type Service struct {
	db     *gorm.DB
	stripe StripeDiscounts
	audit  AuditWriter
	logger *slog.Logger
}

// NewService constructs a Service, refusing a build that could not do its job.
//
// A TYPED nil — a nil *audit.Emitter or a nil *StripeAdapter assigned into the
// interface — is a non-nil interface value, so a plain `!= nil` check would
// pass it and the first method call would run inside an open transaction
// holding a row lock. trial.NewExtender guards the same shape
// (trial/extend.go:198) by normalising it to nil, because there a nil Stripe
// client is a supported configuration. Here neither dependency is optional, so
// the typed nil is normalised and then REFUSED.
func NewService(cfg Config) (*Service, error) {
	if isNilValue(cfg.Audit) {
		return nil, ErrNoAuditWriter
	}
	if isNilValue(cfg.Stripe) {
		return nil, ErrNoStripeClient
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Service{db: cfg.DB, stripe: cfg.Stripe, audit: cfg.Audit, logger: cfg.Logger}, nil
}

// isNilValue reports whether v is nil, including a nil pointer wrapped in a
// non-nil interface.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

// Input is one operator request. The same shape serves Apply and Remove.
type Input struct {
	TenantID uuid.UUID
	CouponID string

	// Reason is the operator's stated reason, recorded in every audit row
	// this call writes. The handler is responsible for making it mandatory
	// and for truncating it before it reaches here.
	Reason string

	// C is the operator's request context, forwarded to audit.EmitTx so
	// buildEntry can derive the operator id, capability and IP. It may be
	// nil — a background caller (T6's subscription-creation hook) has no
	// request — in which case the row records a system actor.
	C *gin.Context
}

// Apply puts the coupon on every store subscription the tenant owns.
//
// The returned error refuses the WHOLE request: bad input, or the tenant
// lookup itself failing. A single store failing is not an error here — it is a
// StoreResult with OutcomeFailed, because the point of a per-store transaction
// is that its siblings still committed.
func (s *Service) Apply(ctx context.Context, in Input) (Result, error) {
	return s.fanOut(ctx, in, applyOp)
}

// Remove takes the coupon off every store subscription the tenant owns,
// leaving every other discount — a merchant's own promo above all — in place.
// It is as audited as the apply (tesserix-home#331).
func (s *Service) Remove(ctx context.Context, in Input) (Result, error) {
	return s.fanOut(ctx, in, removeOp)
}

// operation is the difference between Apply and Remove, which is only the
// audit action and the Stripe call. Everything else — the store enumeration,
// the per-store transaction, the locking read, the guards, the audit write and
// the divergence handling — is shared, so it is written once.
type operation struct {
	name   string // "apply" | "remove", for the divergence error and logs
	action string // audit action

	// run performs the Stripe side and returns the outcome. It is only
	// reached for a store that has a Stripe subscription id.
	run func(ctx context.Context, sd StripeDiscounts, subID, couponID string) (Outcome, error)
}

var applyOp = operation{
	name:   "apply",
	action: ActionApply,
	run: func(ctx context.Context, sd StripeDiscounts, subID, couponID string) (Outcome, error) {
		// Read before write so "we attached it" and "it was already there"
		// are told apart. AddSubscriptionDiscount is idempotent on its own
		// and returns nil for both, which is the right contract for it and
		// not enough information for this report.
		has, err := sd.SubscriptionHasDiscount(ctx, subID, couponID)
		if err != nil {
			return "", err
		}
		if has {
			return OutcomeAlreadyApplied, nil
		}
		if err := sd.AddSubscriptionDiscount(ctx, subID, couponID); err != nil {
			return "", err
		}
		return OutcomeApplied, nil
	},
}

var removeOp = operation{
	name:   "remove",
	action: ActionRemove,
	run: func(ctx context.Context, sd StripeDiscounts, subID, couponID string) (Outcome, error) {
		has, err := sd.SubscriptionHasDiscount(ctx, subID, couponID)
		if err != nil {
			return "", err
		}
		if !has {
			return OutcomeNotApplied, nil
		}
		if err := sd.RemoveSubscriptionDiscount(ctx, subID, couponID); err != nil {
			return "", err
		}
		return OutcomeRemoved, nil
	},
}

func (s *Service) fanOut(ctx context.Context, in Input, op operation) (Result, error) {
	if s == nil {
		return Result{}, errors.New("tenantdiscount: nil service")
	}

	// Validated before the fan-out query, not inside the per-store loop: a
	// blank coupon id would otherwise be reported once per store as a Stripe
	// failure, after N transactions had been opened for it.
	in.CouponID = strings.TrimSpace(in.CouponID)
	if in.TenantID == uuid.Nil {
		return Result{}, ErrNoTenant
	}
	if in.CouponID == "" {
		return Result{}, ErrNoCoupon
	}

	storeIDs, err := s.tenantStores(ctx, in.TenantID)
	if err != nil {
		return Result{}, fmt.Errorf("tenantdiscount: list tenant stores: %w", err)
	}
	if len(storeIDs) == 0 {
		return Result{}, ErrNoStores
	}

	out := Result{TenantID: in.TenantID, CouponID: in.CouponID, Stores: make([]StoreResult, 0, len(storeIDs))}
	for _, storeID := range storeIDs {
		out.Stores = append(out.Stores, s.oneStore(ctx, in, op, storeID))
	}
	return out, nil
}

// tenantStores lists the tenant's stores, ordered so two runs of the same
// request report in the same order.
//
// Enumerating STORES rather than store_subscriptions rows is what makes "a
// store with no subscription" reportable at all: a fan-out over subscription
// rows cannot name a store that has none, and would skip it silently.
//
// No status filter. stores.status is one of active/suspended/archived
// (migration 000001), and a suspended store's subscription can still carry a
// discount; filtering here would silently drop stores from a report whose
// whole purpose is to be exhaustive.
func (s *Service) tenantStores(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.db.WithContext(ctx).
		Raw(`SELECT id FROM stores WHERE tenant_id = ? ORDER BY id`, tenantID).
		Scan(&ids).Error
	return ids, err
}

// oneStore runs the whole operation for a single store inside a single
// transaction, and never returns an error: a store's failure belongs in its
// own line of the report.
func (s *Service) oneStore(ctx context.Context, in Input, op operation, storeID uuid.UUID) StoreResult {
	res := StoreResult{StoreID: storeID}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub subscription.StoreSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("store_id = ?", storeID).First(&sub).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				res.FailureCode = FailureLoadSubscription
				return fmt.Errorf("tenantdiscount: load subscription for store %s: %w", storeID, err)
			}
			// No subscription row: nothing to lock and nothing to bill. The
			// audit row below is still written, so the operator's action
			// leaves a trail for this store rather than none.
			res.Outcome = OutcomeNoSubscription
			return s.writeAudit(ctx, tx, in, op, res)
		}

		res.SubscriptionID = sub.ID
		res.StripeCustomerID = sub.StripeCustomerID
		res.StripeSubscriptionID = stripeSubscriptionID(sub)

		switch {
		case res.StripeCustomerID == "":
			res.Outcome = OutcomeNoStripeCustomer
		case res.StripeSubscriptionID == "":
			res.Outcome = OutcomePending
		default:
			// The row lock taken above is held across this call, so the
			// subscription cannot change underneath it; stripeCallTimeout
			// bounds how long that lock can live. Stripe moves FIRST and is
			// the source of truth, per trial/extend.go:270-280: a Stripe
			// failure rolls this store back and writes nothing, while a
			// failure after it leaves Stripe ahead — which is what
			// ErrStripeChangedAuditWriteFailed is for.
			sctx, cancel := context.WithTimeout(ctx, stripeCallTimeout)
			defer cancel()

			outcome, err := op.run(sctx, s.stripe, res.StripeSubscriptionID, in.CouponID)
			if err != nil {
				res.FailureCode = FailureStripeCall
				return fmt.Errorf("%w: %s coupon %s on subscription %s: %w",
					ErrStripeCall, op.name, in.CouponID, res.StripeSubscriptionID, err)
			}
			res.Outcome = outcome
		}

		// LAST, and inside this store's transaction: the row and the change
		// it describes commit or roll back together.
		if err := s.writeAudit(ctx, tx, in, op, res); err != nil {
			res.FailureCode = FailureAuditWrite
			return err
		}
		return nil
	})
	if err == nil {
		return res
	}

	// The closure wrote into res before the rollback, so res still records
	// whether Stripe was actually changed. A rolled-back transaction undoes
	// our audit row; it cannot undo Stripe.
	//
	// This catches the COMMIT failing as well as the audit insert failing:
	// the audit row is the only thing this transaction carried, so a lost
	// commit loses exactly the same record, and both leave Stripe ahead.
	if res.Changed() {
		if res.FailureCode == "" {
			res.FailureCode = FailureCommit
		}
		div := &AuditDivergenceError{
			Op:                   op.name,
			StoreID:              storeID,
			CouponID:             in.CouponID,
			StripeSubscriptionID: res.StripeSubscriptionID,
			StripeCustomerID:     res.StripeCustomerID,
			Cause:                err,
		}
		// Logged as well as returned: the returned error travels to one
		// operator, and reconciling this needs whoever reads the logs to
		// have the three ids without that operator forwarding them.
		s.logger.Error("tenantdiscount: stripe discount changed but the audit row was not written",
			"op", op.name,
			"coupon_id", in.CouponID,
			"stripe_subscription_id", res.StripeSubscriptionID,
			"stripe_customer_id", res.StripeCustomerID,
			"store_id", storeID.String(),
			"tenant_id", in.TenantID.String(),
			"err", err)
		err = div
	}

	res.Outcome = OutcomeFailed
	res.Err = err
	return res
}

// writeAudit emits the store's audit row on the caller's transaction.
func (s *Service) writeAudit(ctx context.Context, tx *gorm.DB, in Input, op operation, res StoreResult) error {
	md := map[string]any{
		"coupon_id": in.CouponID,
		"outcome":   string(res.Outcome),
	}
	if in.Reason != "" {
		md["reason"] = in.Reason
	}
	if res.StripeSubscriptionID != "" {
		md["stripe_subscription_id"] = res.StripeSubscriptionID
	}
	if res.StripeCustomerID != "" {
		md["stripe_customer_id"] = res.StripeCustomerID
	}

	// Warning for the outcomes where the operator asked for a billing change
	// and did not get one, so those stand out in the console's audit filter
	// from the ones that did what was asked.
	severity := audit.SeverityInfo
	switch res.Outcome {
	case OutcomePending, OutcomeNoSubscription, OutcomeNoStripeCustomer:
		severity = audit.SeverityWarning
	}

	return s.audit.EmitTx(ctx, tx, in.C, audit.Event{
		Action:       op.action,
		ResourceType: resourceType,
		ResourceID:   res.StoreID.String(),
		Severity:     severity,
		Metadata:     md,
		TenantID:     in.TenantID,
		StoreID:      res.StoreID,
	})
}

// stripeSubscriptionID returns the subscription's Stripe id, or "" when it has
// none. A nil pointer and a pointer to "" mean the same thing — the store has
// no Stripe subscription — and collapsing them at one site keeps every caller
// from having to remember that. Same helper, same reasoning, as
// trial/extend.go:484.
func stripeSubscriptionID(sub subscription.StoreSubscription) string {
	if sub.StripeSubscriptionID == nil {
		return ""
	}
	return *sub.StripeSubscriptionID
}
