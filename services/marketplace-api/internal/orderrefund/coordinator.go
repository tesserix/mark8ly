package orderrefund

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// scopeIDPattern restricts RefundCommand.ScopeID to the charset gateways
// accept in idempotency keys (Razorpay's X-Refund-Idempotency requires
// [A-Za-z0-9_-]). Return IDs (uuids) and the "cancel" constant already
// satisfy this; the only untrusted value is admin's client-supplied
// refund_request_id, but validating centrally protects every caller.
var scopeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Refund ledger row statuses used by the saga.
const (
	refundStatusPending   = "pending"
	refundStatusSucceeded = "succeeded"
)

// RefundCommand is a request to refund all or part of an order's captured
// payment. Amount==nil means "refund the full remaining balance"
// (grand_total - refunded_amount).
type RefundCommand struct {
	OrderID uuid.UUID
	Amount  *decimal.Decimal // nil ⇒ full remaining
	Reason  string
	Actor   string
	ScopeID string // idempotency scope: return_id | "cancel" | refund_request_id ([A-Za-z0-9_-] only)
}

// RefundResult is what a successful (or already-completed) Refund call
// reports back to the caller.
type RefundResult struct {
	ProviderRefundID string
	Amount           decimal.Decimal
	PaymentStatus    order.PaymentStatus
	AlreadyDone      bool
}

// resolver is the narrow surface Coordinator needs from *Resolver — reading
// an order's captured payment context and constructing the matching
// payment.Gateway. Defined as an interface (rather than depending on
// *Resolver concretely) so integration tests can inject a fake gateway
// while still exercising the real PaymentContextForOrder DB read.
type resolver interface {
	PaymentContextForOrder(ctx context.Context, orderID uuid.UUID) (PaymentContext, error)
	GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error)
}

// Coordinator runs the refund saga: reserve a pending ledger row, call the
// gateway, then atomically finalize the ledger and bookkeep the order. Every
// trigger that can produce a refund (admin refund, return refund, paid
// cancel) calls Coordinator.Refund — this is the single place gateway money
// movement happens.
type Coordinator struct {
	db        *gorm.DB
	res       resolver
	pay       *payment.Service
	orders    *order.Service
	orderRepo order.Repository
	enabled   bool
}

// NewCoordinator constructs a Coordinator. enabled gates whether Refund is
// allowed to touch the gateway/DB at all — see the REFUND_GATEWAY_ENABLED
// kill switch.
func NewCoordinator(db *gorm.DB, res resolver, pay *payment.Service, orders *order.Service, orderRepo order.Repository, enabled bool) *Coordinator {
	return &Coordinator{db: db, res: res, pay: pay, orders: orders, orderRepo: orderRepo, enabled: enabled}
}

// DeriveStatus picks partially_refunded vs refunded from the post-refund
// total: refunded so far plus the amount about to be refunded, compared
// against the order's grand total.
func DeriveStatus(refunded, amount, grandTotal decimal.Decimal) order.PaymentStatus {
	if refunded.Add(amount).GreaterThanOrEqual(grandTotal) {
		return order.PaymentStatusRefunded
	}
	return order.PaymentStatusPartiallyRefunded
}

// idempotencyKey builds a provider-safe key. Charset is restricted to
// [A-Za-z0-9_-] (Razorpay's X-Refund-Idempotency requires this and min 10
// chars; Stripe/PayPal accept it). NO colons — Razorpay rejects them.
func idempotencyKey(orderID uuid.UUID, scopeID string) string {
	return "refund_" + orderID.String() + "_" + scopeID
}

// Refund runs the full saga for cmd. It never touches the gateway or the
// database when the coordinator is disabled.
func (c *Coordinator) Refund(ctx context.Context, cmd RefundCommand) (RefundResult, error) {
	if !c.enabled {
		return RefundResult{}, fmt.Errorf("orderrefund: gateway refunds disabled (REFUND_GATEWAY_ENABLED=false)")
	}
	if !scopeIDPattern.MatchString(cmd.ScopeID) {
		return RefundResult{}, apperrors.ValidationFailed("refund_request_id", "must match [A-Za-z0-9_-]")
	}

	o, _, _, err := c.orderRepo.GetByID(ctx, c.db, cmd.OrderID)
	if err != nil {
		return RefundResult{}, err
	}

	pc, err := c.res.PaymentContextForOrder(ctx, cmd.OrderID)
	if err != nil {
		return RefundResult{}, err
	}
	if !pc.Found {
		return RefundResult{}, apperrors.RefundUnavailable("no captured payment for this order")
	}

	remaining := o.GrandTotal.Sub(o.RefundedAmount)
	amount := remaining
	if cmd.Amount != nil {
		amount = *cmd.Amount
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return RefundResult{}, apperrors.ValidationFailed("amount", "refund amount must be positive")
	}

	refundCap := decimal.Min(o.GrandTotal, pc.CapturedTotal)
	if o.RefundedAmount.Add(amount).GreaterThan(refundCap) {
		return RefundResult{}, apperrors.ErrRefundExceedsTotal
	}

	target := DeriveStatus(o.RefundedAmount, amount, o.GrandTotal)
	key := idempotencyKey(cmd.OrderID, cmd.ScopeID)

	gw, err := c.res.GatewayFor(ctx, o.StoreID, pc.Provider)
	if err != nil {
		return RefundResult{}, err
	}

	// tx #1 — reserve pending ledger row (idempotent re-entry guard).
	var ledger *payment.RefundTransaction
	var created bool
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, wasCreated, rErr := c.pay.ReserveRefund(ctx, tx, payment.ReserveRefundInput{
			TenantID:          o.TenantID.String(),
			StoreID:           o.StoreID.String(),
			OrderID:           cmd.OrderID.String(),
			Provider:          pc.Provider,
			ProviderPaymentID: pc.ProviderPaymentID,
			Amount:            amount,
			CurrencyCode:      o.CurrencyCode,
			Reason:            cmd.Reason,
			IdempotencyKey:    key,
		})
		ledger = row
		created = wasCreated
		return rErr
	}); err != nil {
		return RefundResult{}, err
	}
	if !created {
		if ledger.Status == refundStatusSucceeded {
			return RefundResult{
				ProviderRefundID: ledger.ProviderRefundID,
				Amount:           amount,
				PaymentStatus:    target,
				AlreadyDone:      true,
			}, nil
		}
		// An in-flight or crashed prior attempt reserved this key and never
		// reached "succeeded". Do NOT re-run the gateway/bump refunded_amount
		// here — that risks a double bump under RecordRefund's cap-only
		// guard. Crash-recovery of stuck pending rows is handled by the
		// sweeper, not by re-running Refund.
		return RefundResult{}, apperrors.ErrIdempotencyConflict
	}

	// gateway call — real money, retry-safe via key. Deliberately outside
	// any transaction: if this fails or the process dies, the ledger row
	// stays pending and a sweeper (or the next retry with the same
	// ScopeID) picks it back up.
	ref, err := c.pay.ExecuteGatewayRefund(ctx, gw, payment.RefundInput{
		ProviderPaymentID: pc.ProviderPaymentID,
		Amount:            amount,
		CurrencyCode:      o.CurrencyCode,
		Reason:            cmd.Reason,
		IdempotencyKey:    key,
	})
	if err != nil {
		return RefundResult{}, err // ledger stays pending → sweeper retries
	}

	// tx #2 — finalize ledger + bookkeeping, atomic.
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.pay.FinalizeRefund(ctx, tx, ledger.ID, ref.ProviderRefundID, refundStatusSucceeded); err != nil {
			return err
		}
		return c.orders.RecordRefund(ctx, tx, cmd.OrderID, amount, target, cmd.Reason)
	}); err != nil {
		return RefundResult{}, err
	}

	return RefundResult{
		ProviderRefundID: ref.ProviderRefundID,
		Amount:           amount,
		PaymentStatus:    target,
	}, nil
}

// ResumePending re-drives refund ledger rows stuck in 'pending' — the never-lost
// guarantee. Safe to run repeatedly: the same idempotency_key makes the gateway
// call a no-op if money already moved. A 'pending' row means tx#2 never committed,
// so orders.refunded_amount was NOT bumped for it — re-running RecordRefund bumps
// it exactly once.
func (c *Coordinator) ResumePending(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	// The interval literal is built in Go (rather than concatenated in SQL
	// via `? || ' seconds'`) because pgx cannot infer a text type for a bare
	// numeric placeholder feeding string concatenation.
	cutoff := fmt.Sprintf("%d seconds", int(olderThan.Seconds()))

	// Claim a batch of pending row IDs with FOR UPDATE SKIP LOCKED so
	// overlapping sweep runs (Cloud Scheduler retry, manual re-trigger, a
	// prior run still mid-flight) don't even pick the same rows. This is a
	// best-effort reduction of duplicate gateway calls, not the correctness
	// guarantee — the row lock is released as soon as this short
	// transaction commits, well before the gateway call and finalize below.
	// The status-guarded UPDATE in the per-row loop is what makes
	// double-finalization impossible even if two sweeps do end up
	// processing the same row.
	var ids []string
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&payment.RefundTransaction{}).
			Select("id").
			Where("status = 'pending' AND created_at < now() - (?)::interval", cutoff).
			Order("created_at ASC").
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Limit(limit).
			Pluck("id", &ids).Error
	}); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	var rows []payment.RefundTransaction
	if err := c.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return 0, err
	}

	resumed := 0
	for i := range rows {
		row := rows[i]
		oid, err := uuid.Parse(row.OrderID)
		if err != nil {
			continue
		}
		o, _, _, err := c.orderRepo.GetByID(ctx, c.db, oid)
		if err != nil || o == nil {
			continue
		}
		gw, err := c.res.GatewayFor(ctx, o.StoreID, row.Provider)
		if err != nil {
			continue
		}
		ref, err := c.pay.ExecuteGatewayRefund(ctx, gw, payment.RefundInput{
			ProviderPaymentID: row.ProviderPaymentID, Amount: row.Amount,
			CurrencyCode: o.CurrencyCode, Reason: row.Reason, IdempotencyKey: row.IdempotencyKey,
		})
		if err != nil {
			continue // try again next sweep
		}
		target := DeriveStatus(o.RefundedAmount, row.Amount, o.GrandTotal)

		// Status-guarded finalize: the UPDATE only flips rows still
		// 'pending'. If a concurrent sweep run already finalized this row
		// (RowsAffected == 0), skip RecordRefund entirely — otherwise
		// orders.refunded_amount would be double-bumped for one real
		// refund, since RecordRefund's only guard is the grand_total cap,
		// not idempotency. Deliberately inlined here (rather than reusing
		// payment.Service.FinalizeRefund) to make the guard explicit and
		// keep the shared method's Task-6 contract untouched.
		finalized := false
		if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Exec(
				`UPDATE refund_transactions SET status = 'succeeded', provider_refund_id = ?, updated_at = now()
				  WHERE id = ? AND status = 'pending'`,
				ref.ProviderRefundID, row.ID,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				// Another sweep run already finalized this row — do NOT
				// bump refunded_amount again.
				return nil
			}
			finalized = true
			return c.orders.RecordRefund(ctx, tx, oid, row.Amount, target, row.Reason)
		}); err != nil {
			continue
		}
		if finalized {
			resumed++
		}
	}
	return resumed, nil
}
