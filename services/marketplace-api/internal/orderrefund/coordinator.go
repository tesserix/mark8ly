package orderrefund

import (
	"context"
	"errors"
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

// refundReplay classifies a same-ScopeID re-entry detected during reservation.
type refundReplay int

const (
	replayNone      refundReplay = iota
	replaySucceeded              // prior attempt already moved money → idempotent success
	replayPending                // prior attempt reserved but never finalized → conflict
)

// Refund runs the full saga for cmd. It never touches the gateway or the
// database when the coordinator is disabled.
func (c *Coordinator) Refund(ctx context.Context, cmd RefundCommand) (RefundResult, error) {
	if !c.enabled {
		return RefundResult{}, apperrors.ErrRefundsDisabled
	}
	if !scopeIDPattern.MatchString(cmd.ScopeID) {
		return RefundResult{}, apperrors.ValidationFailed("refund_request_id", "must match [A-Za-z0-9_-]")
	}

	pc, err := c.res.PaymentContextForOrder(ctx, cmd.OrderID)
	if err != nil {
		return RefundResult{}, err
	}
	if !pc.Found {
		return RefundResult{}, apperrors.RefundUnavailable("no captured payment for this order")
	}

	// Read once for the immutable routing fields (store + provider) needed to
	// build the gateway. The authoritative balance for the cap check is
	// re-read under a row lock in tx #1 below.
	o, _, _, err := c.orderRepo.GetByID(ctx, c.db, cmd.OrderID)
	if err != nil {
		return RefundResult{}, err
	}

	gw, err := c.res.GatewayFor(ctx, o.StoreID, pc.Provider)
	if err != nil {
		return RefundResult{}, err
	}

	key := idempotencyKey(cmd.OrderID, cmd.ScopeID)
	refundCap := decimal.Min(o.GrandTotal, pc.CapturedTotal)

	// tx #1 — serialize on the order row, validate against the authoritative
	// balance INCLUDING in-flight pending refunds, then reserve the ledger
	// row. Locking the order (FOR UPDATE) is what makes concurrent refunds
	// with DIFFERENT ScopeIDs safe: the second blocks until the first's
	// pending row is committed, then counts it in the cap check. Without the
	// lock + pending-sum, two distinct-scope refunds could each clear the cap
	// on a stale refunded_amount and both move real money on the gateway.
	var (
		ledger        *payment.RefundTransaction
		amount        decimal.Decimal
		target        order.PaymentStatus
		currentStatus order.PaymentStatus
		replay        refundReplay
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, lErr := lockOrder(tx, cmd.OrderID)
		if lErr != nil {
			return lErr
		}
		currentStatus = order.PaymentStatus(locked.PaymentStatus)

		amount = locked.GrandTotal.Sub(locked.RefundedAmount) // full remaining
		if cmd.Amount != nil {
			amount = *cmd.Amount
		}
		if amount.LessThanOrEqual(decimal.Zero) {
			return apperrors.ValidationFailed("amount", "refund amount must be positive")
		}
		target = DeriveStatus(locked.RefundedAmount, amount, locked.GrandTotal)

		row, wasCreated, rErr := c.pay.ReserveRefund(ctx, tx, payment.ReserveRefundInput{
			TenantID:          locked.TenantID.String(),
			StoreID:           locked.StoreID.String(),
			OrderID:           cmd.OrderID.String(),
			Provider:          pc.Provider,
			ProviderPaymentID: pc.ProviderPaymentID,
			Amount:            amount,
			CurrencyCode:      locked.CurrencyCode,
			Reason:            cmd.Reason,
			IdempotencyKey:    key,
		})
		if rErr != nil {
			return rErr
		}
		ledger = row

		if !wasCreated {
			// Same-ScopeID replay: a succeeded row is an idempotent success; a
			// still-pending row is a conflict (crash-recovery of stuck rows is
			// the sweeper's job, not Refund's — re-running here risks a double
			// bump under RecordRefund's cap-only guard).
			if row.Status == refundStatusSucceeded {
				replay = replaySucceeded
			} else {
				replay = replayPending
			}
			return nil
		}

		// Cap check on committed balance + all in-flight pending refunds for
		// this order (our just-inserted row included). Exceeding the cap rolls
		// back this tx, removing the reservation we just made.
		pendingSum, pErr := sumPendingRefunds(tx, cmd.OrderID)
		if pErr != nil {
			return pErr
		}
		if locked.RefundedAmount.Add(pendingSum).GreaterThan(refundCap) {
			return apperrors.ErrRefundExceedsTotal
		}
		return nil
	}); err != nil {
		return RefundResult{}, err
	}

	switch replay {
	case replaySucceeded:
		return RefundResult{
			ProviderRefundID: ledger.ProviderRefundID,
			Amount:           ledger.Amount,
			PaymentStatus:    currentStatus,
			AlreadyDone:      true,
		}, nil
	case replayPending:
		return RefundResult{}, apperrors.ErrIdempotencyConflict
	}

	// gateway call — real money, retry-safe via key. Deliberately outside any
	// transaction: on a transient failure the ledger row stays pending and the
	// sweeper (or a same-ScopeID retry) picks it back up. On a PERMANENT
	// failure (bad request, invalid id, already refunded) the row is moved to
	// 'failed' so it is never re-driven forever — it surfaces for manual
	// reconciliation instead.
	ref, err := c.pay.ExecuteGatewayRefund(ctx, gw, payment.RefundInput{
		ProviderPaymentID: pc.ProviderPaymentID,
		Amount:            amount,
		CurrencyCode:      o.CurrencyCode,
		Reason:            cmd.Reason,
		IdempotencyKey:    key,
	})
	if err != nil {
		if payment.IsPermanentGatewayError(err) {
			_ = c.pay.MarkRefundFailed(ctx, c.db, ledger.ID)
		}
		return RefundResult{}, err
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

// lockOrder reads an order FOR UPDATE, serialising concurrent refunds on the
// same order so their cap checks can't race on a stale refunded_amount.
func lockOrder(tx *gorm.DB, id uuid.UUID) (*order.Order, error) {
	var o order.Order
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&o).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("order")
		}
		return nil, err
	}
	return &o, nil
}

// sumPendingRefunds totals the amount of all still-pending refund ledger rows
// for an order. These are refunds that have reserved a row but not yet
// finalized (so orders.refunded_amount does not yet reflect them); the cap
// check must count them to prevent concurrent distinct-scope over-refunds.
func sumPendingRefunds(tx *gorm.DB, orderID uuid.UUID) (decimal.Decimal, error) {
	var result struct{ Sum decimal.Decimal }
	err := tx.Model(&payment.RefundTransaction{}).
		Where("order_id = ? AND status = ?", orderID, refundStatusPending).
		Select("COALESCE(SUM(amount), 0) AS sum").
		Scan(&result).Error
	return result.Sum, err
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
			// Permanent failure will never succeed on retry — move it to
			// 'failed' so subsequent sweeps skip it (the claim query only
			// selects 'pending'). Transient failures are left pending to
			// retry on the next sweep.
			if payment.IsPermanentGatewayError(err) {
				_ = c.pay.MarkRefundFailed(ctx, c.db, row.ID)
			}
			continue
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
