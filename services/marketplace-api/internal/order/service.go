package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service is the orchestration layer for order lifecycle operations.
//
// The service owns the transaction boundary: every public mutating method
// either opens its own transaction internally OR accepts a *gorm.DB tx
// supplied by another module that wants to share the transaction (the
// "cross-module tx" pattern — see return.Service).
//
// Domain events are written to two places inside the same tx as the
// mutation that triggers them:
//   1. order_events — append-only audit log scoped to the order itself.
//   2. outbox_events — products-owned shared table, picked up by
//      internal/outbox.Publisher and used to bump store_watermarks.orders_updated_at.
type Service struct {
	db      *gorm.DB
	repo    Repository
	outbox  outbox.Repository
}

// NewService constructs a Service. The db is used both for opening fresh
// transactions in single-method calls and as the read connection for
// idempotency lookups.
func NewService(db *gorm.DB, repo Repository, outboxRepo outbox.Repository) *Service {
	return &Service{db: db, repo: repo, outbox: outboxRepo}
}

// Unit runs fn inside a single transaction. Use this when an external caller
// (e.g. return.Service.Approve) needs to chain multiple order.Service calls
// together with its own writes.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// CreateInput is everything Service.Create needs to land an order.
// IdempotencyKey is required and unique per (StoreID, key); a duplicate
// returns the previously-created order with reused=true.
//
// OrderNumberSeq is the per-store sequence value to embed in the human
// order number. The caller obtains it from order.NextDocumentNumber inside
// the same Unit() that calls Create.
//
// StorePrefix is the 3-char prefix used in the human order number; it is
// derived from the store slug at the HTTP layer (M4) and passed in.
type CreateInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	StorePrefix    string
	OrderNumberSeq int64
	IdempotencyKey string
	CustomerID     *uuid.UUID
	CustomerEmail  string
	CustomerName   *string
	Items          []OrderItem
	Shipping       OrderAddress
	Billing        OrderAddress
	Subtotal       decimal.Decimal
	ShippingTotal  decimal.Decimal
	TaxTotal       decimal.Decimal
	DiscountTotal  decimal.Decimal
	GrandTotal     decimal.Decimal
	CurrencyCode   string
	// ShippingService and ShippingCarrier are the customer's checkout choice
	// — the carrier provider name (resolved from store config) and the
	// service level (standard / express / etc). Persisted so the admin
	// label-creation form can default to what the buyer paid for instead
	// of a hardcoded fallback.
	ShippingService *string
	ShippingCarrier *string
	Notes          *string
	PlacedAt       time.Time
}

// CreateResult holds the persisted order plus a Reused flag for the
// idempotency-replay path.
type CreateResult struct {
	Order  *Order
	Items  []OrderItem
	Addrs  []OrderAddress
	Reused bool
}

// Create inserts a new order, its items, addresses, the initial
// order_events row, and an outbox_events row — all in one tx. If a row
// already exists for (store_id, idempotency_key), the existing aggregate
// is returned and Reused is true.
func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	if in.IdempotencyKey == "" {
		return nil, apperrors.ValidationFailed("idempotency_key", "idempotency_key is required")
	}
	if in.PlacedAt.IsZero() {
		in.PlacedAt = time.Now().UTC()
	}

	// Pre-tx idempotency lookup. The UNIQUE constraint is the second line
	// of defense against concurrent racers.
	if existing, items, addrs, err := s.repo.GetByIdempotencyKey(ctx, s.db, in.StoreID, in.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return &CreateResult{Order: existing, Items: items, Addrs: addrs, Reused: true}, nil
	}

	// If no sequence was pre-allocated, allocate inside the Create
	// transaction below so the number is atomic with the order insert.
	// This prevents burned numbers on crashes between allocation and create.
	number := ""
	if in.OrderNumberSeq > 0 {
		number = FormatOrderNumber(in.StorePrefix, in.PlacedAt, in.OrderNumberSeq)
	}

	o := &Order{
		TenantID:          in.TenantID,
		StoreID:           in.StoreID,
		OrderNumber:       number,
		IdempotencyKey:    in.IdempotencyKey,
		CustomerID:        in.CustomerID,
		CustomerEmail:     in.CustomerEmail,
		CustomerName:      in.CustomerName,
		Status:            string(OrderStatusPending),
		PaymentStatus:     string(PaymentStatusPending),
		FulfillmentStatus: string(FulfillmentStatusUnfulfilled),
		Subtotal:          in.Subtotal,
		ShippingTotal:     in.ShippingTotal,
		TaxTotal:          in.TaxTotal,
		DiscountTotal:     in.DiscountTotal,
		GrandTotal:        in.GrandTotal,
		RefundedAmount:    decimal.Zero,
		CurrencyCode:      in.CurrencyCode,
		ShippingService:   in.ShippingService,
		ShippingCarrier:   in.ShippingCarrier,
		Notes:             in.Notes,
		PlacedAt:          in.PlacedAt,
	}

	addrs := []OrderAddress{in.Shipping, in.Billing}
	addrs[0].Kind = "shipping"
	addrs[1].Kind = "billing"

	initialEvent := OrderEvent{
		Kind: string(EventKindCreated),
		Payload: EncodeCreated(CreatedPayload{
			OrderNumber:  number,
			CurrencyCode: in.CurrencyCode,
			GrandTotal:   in.GrandTotal.String(),
		}),
	}

	var result *CreateResult
	err := s.Unit(ctx, func(tx *gorm.DB) error {
		// Allocate sequence inside the transaction if not pre-allocated,
		// so the number is atomic with the order insert.
		if number == "" {
			seq, err := NextDocumentNumber(ctx, tx, in.StoreID, "order")
			if err != nil {
				return err
			}
			number = FormatOrderNumber(in.StorePrefix, in.PlacedAt, seq)
			o.OrderNumber = number
			initialEvent.Payload = EncodeCreated(CreatedPayload{
				OrderNumber:  number,
				CurrencyCode: in.CurrencyCode,
				GrandTotal:   in.GrandTotal.String(),
			})
		}
		if err := s.repo.CreateInTx(tx, o, in.Items, addrs, initialEvent); err != nil {
			return err
		}
		if err := s.enqueueOutbox(ctx, tx, in.TenantID, outbox.AggregateOrder, o.ID, outbox.EventOrderPlaced, map[string]any{
			"store_id":     in.StoreID.String(),
			"order_id":     o.ID.String(),
			"order_number": number,
			"grand_total":  in.GrandTotal.String(),
			"currency":     in.CurrencyCode,
		}); err != nil {
			return err
		}
		result = &CreateResult{Order: o, Items: in.Items, Addrs: addrs, Reused: false}
		return nil
	})
	return result, err
}

// Confirm transitions an order from pending → confirmed and emits the
// confirmation event. Optionally updates payment_status if the caller is
// recording payment authorization in the same step.
func (s *Service) Confirm(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, paymentStatusTarget *PaymentStatus, reason string) error {
	return s.runMaybeOwnTx(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.loadForUpdate(tx, orderID)
		if err != nil {
			return err
		}
		if !OrderStatus(o.Status).CanTransitionTo(OrderStatusConfirmed) {
			return apperrors.InvalidTransition("order", o.Status, string(OrderStatusConfirmed))
		}
		if err := s.repo.UpdateOrderStatus(tx, orderID, OrderStatusConfirmed); err != nil {
			return err
		}
		if paymentStatusTarget != nil {
			if !PaymentStatus(o.PaymentStatus).CanTransitionTo(*paymentStatusTarget) {
				return apperrors.InvalidTransition("payment", o.PaymentStatus, string(*paymentStatusTarget))
			}
			if err := s.repo.UpdatePaymentStatus(tx, orderID, *paymentStatusTarget); err != nil {
				return err
			}
			if err := s.repo.AppendEvent(tx, &OrderEvent{
				OrderID: orderID,
				Kind:    string(EventKindPaymentRecorded),
				Payload: EncodePaymentRecorded(PaymentRecordedPayload{
					From:   o.PaymentStatus,
					To:     string(*paymentStatusTarget),
					Reason: reason,
				}),
			}); err != nil {
				return err
			}
		}
		if err := s.repo.AppendEvent(tx, &OrderEvent{
			OrderID: orderID,
			Kind:    string(EventKindStatusChanged),
			Payload: EncodeStatusChanged(StatusChangedPayload{
				Axis: "order",
				From: o.Status,
				To:   string(OrderStatusConfirmed),
			}),
		}); err != nil {
			return err
		}
		return s.enqueueOutbox(ctx, tx, o.TenantID, outbox.AggregateOrder, orderID, outbox.EventOrderConfirmed, map[string]any{
			"store_id": o.StoreID.String(),
			"order_id": orderID.String(),
		})
	})
}

// MarkFulfilled transitions order.status confirmed → fulfilled AND
// fulfillment_status → fulfilled in one tx.
func (s *Service) MarkFulfilled(ctx context.Context, tx *gorm.DB, orderID uuid.UUID) error {
	return s.runMaybeOwnTx(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.loadForUpdate(tx, orderID)
		if err != nil {
			return err
		}
		if !OrderStatus(o.Status).CanTransitionTo(OrderStatusFulfilled) {
			return apperrors.InvalidTransition("order", o.Status, string(OrderStatusFulfilled))
		}
		if !FulfillmentStatus(o.FulfillmentStatus).CanTransitionTo(FulfillmentStatusFulfilled) {
			return apperrors.InvalidTransition("fulfillment", o.FulfillmentStatus, string(FulfillmentStatusFulfilled))
		}
		if err := s.repo.UpdateOrderStatus(tx, orderID, OrderStatusFulfilled); err != nil {
			return err
		}
		if err := s.repo.UpdateFulfillmentStatus(tx, orderID, FulfillmentStatusFulfilled); err != nil {
			return err
		}
		if err := s.repo.AppendEvent(tx, &OrderEvent{
			OrderID: orderID,
			Kind:    string(EventKindFulfilled),
			Payload: EncodeStatusChanged(StatusChangedPayload{
				Axis: "fulfillment",
				From: o.FulfillmentStatus,
				To:   string(FulfillmentStatusFulfilled),
			}),
		}); err != nil {
			return err
		}
		return s.enqueueOutbox(ctx, tx, o.TenantID, outbox.AggregateOrder, orderID, outbox.EventOrderFulfilled, map[string]any{
			"store_id": o.StoreID.String(),
			"order_id": orderID.String(),
		})
	})
}

// Cancel transitions an order to cancelled. Reason is recorded in the
// order_events row.
func (s *Service) Cancel(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, reason string) error {
	return s.runMaybeOwnTx(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.loadForUpdate(tx, orderID)
		if err != nil {
			return err
		}
		if !OrderStatus(o.Status).CanTransitionTo(OrderStatusCancelled) {
			return apperrors.InvalidTransition("order", o.Status, string(OrderStatusCancelled))
		}
		res := tx.Model(&Order{}).
			Where("id = ? AND deleted_at IS NULL", orderID).
			Updates(map[string]any{
				"status":       string(OrderStatusCancelled),
				"cancelled_at": gorm.Expr("now()"),
				"updated_at":   gorm.Expr("now()"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return apperrors.NotFound("order")
		}
		if err := s.repo.AppendEvent(tx, &OrderEvent{
			OrderID: orderID,
			Kind:    string(EventKindCancelled),
			Payload: EncodeStatusChanged(StatusChangedPayload{
				Axis:   "order",
				From:   o.Status,
				To:     string(OrderStatusCancelled),
				Reason: reason,
			}),
		}); err != nil {
			return err
		}
		return s.enqueueOutbox(ctx, tx, o.TenantID, outbox.AggregateOrder, orderID, outbox.EventOrderCancelled, map[string]any{
			"store_id": o.StoreID.String(),
			"order_id": orderID.String(),
			"reason":   reason,
		})
	})
}

// RecordRefund delegates to the repository's atomic UPDATE and writes the
// matching order_events + outbox_events rows. paymentStatusTarget is
// usually PartiallyRefunded or Refunded — caller decides based on amount.
func (s *Service) RecordRefund(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, amount decimal.Decimal, paymentStatusTarget PaymentStatus, reason string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return apperrors.ValidationFailed("amount", "refund amount must be positive")
	}
	return s.runMaybeOwnTx(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.loadForUpdate(tx, orderID)
		if err != nil {
			return err
		}
		if !PaymentStatus(o.PaymentStatus).CanTransitionTo(paymentStatusTarget) {
			return apperrors.InvalidTransition("payment", o.PaymentStatus, string(paymentStatusTarget))
		}
		newRefunded, err := s.repo.RecordRefund(tx, orderID, amount, paymentStatusTarget)
		if err != nil {
			return err
		}
		if err := s.repo.AppendEvent(tx, &OrderEvent{
			OrderID: orderID,
			Kind:    string(EventKindRefunded),
			Payload: EncodeRefunded(RefundedPayload{
				Amount:          amount.String(),
				TotalRefunded:   newRefunded.String(),
				PaymentStatusTo: string(paymentStatusTarget),
				Reason:          reason,
			}),
		}); err != nil {
			return err
		}
		return s.enqueueOutbox(ctx, tx, o.TenantID, outbox.AggregateOrder, orderID, outbox.EventOrderRefunded, map[string]any{
			"store_id":         o.StoreID.String(),
			"order_id":         orderID.String(),
			"amount":           amount.String(),
			"refunded_total":   newRefunded.String(),
			"payment_status":   string(paymentStatusTarget),
		})
	})
}

// LinkAbandonedCart marks an open abandoned_carts row as converted to the
// given order. Storefront checkout calls this immediately after a fresh
// Create to close the recovery loop. The match is by (store_id,
// cart_session_id) so a session that switched stores cannot accidentally
// claim a cart from a different store. Setting converted_order_id is
// idempotent — a second call with the same arguments is a no-op.
//
// If tx is nil the method opens its own short tx; otherwise it runs
// inside the supplied one.
func (s *Service) LinkAbandonedCart(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, cartSessionID string, orderID uuid.UUID) error {
	if cartSessionID == "" {
		return apperrors.ValidationFailed("cart_session_id", "cart_session_id is required")
	}
	return s.runMaybeOwnTx(ctx, tx, func(tx *gorm.DB) error {
		res := tx.Exec(
			`UPDATE abandoned_carts
			    SET converted_order_id = ?,
			        updated_at         = now()
			  WHERE store_id           = ?
			    AND cart_session_id    = ?
			    AND converted_order_id IS NULL`,
			orderID, storeID, cartSessionID,
		)
		if res.Error != nil {
			return res.Error
		}
		// Zero rows affected is fine — the cart may have already been
		// linked, or there may be no abandoned cart row at all (the
		// customer typed in the URL directly). Return nil so callers
		// don't have to special-case the absence.
		return nil
	})
}

// RecordReturnEvent is the cross-module hook return.Service uses to drop
// a return_* row into order_events + outbox_events without owning the
// orders aggregate. The caller MUST pass an active tx; this method never
// opens its own.
func (s *Service) RecordReturnEvent(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, kind EventKind, eventType string, payload ReturnEventPayload) error {
	if tx == nil {
		return fmt.Errorf("order: RecordReturnEvent requires a transaction handle")
	}
	var o Order
	if err := tx.Where("id = ?", orderID).First(&o).Error; err != nil {
		return err
	}
	if err := s.repo.AppendEvent(tx, &OrderEvent{
		OrderID: orderID,
		Kind:    string(kind),
		Payload: EncodeReturnEvent(payload),
	}); err != nil {
		return err
	}
	return s.enqueueOutbox(ctx, tx, o.TenantID, outbox.AggregateReturn, orderID, eventType, map[string]any{
		"store_id":      o.StoreID.String(),
		"order_id":      orderID.String(),
		"return_id":     payload.ReturnID,
		"return_number": payload.ReturnNumber,
	})
}

// -----------------------------------------------------------------------------
// internal helpers
// -----------------------------------------------------------------------------

// runMaybeOwnTx runs fn inside the supplied tx if non-nil; otherwise opens
// a fresh tx via Unit. Lets every public method support both standalone
// and cross-module-shared tx callers without code duplication.
func (s *Service) runMaybeOwnTx(ctx context.Context, tx *gorm.DB, fn func(tx *gorm.DB) error) error {
	if tx != nil {
		return fn(tx)
	}
	return s.Unit(ctx, fn)
}

// loadForUpdate fetches the order with a row-level lock so the subsequent
// status update is consistent. Returns apperrors.NotFound on miss.
func (s *Service) loadForUpdate(tx *gorm.DB, id uuid.UUID) (*Order, error) {
	var o Order
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&o).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("order")
		}
		return nil, err
	}
	return &o, nil
}

// enqueueOutbox marshals payload to JSON and writes one outbox_events row
// inside the supplied tx. Every order-domain producer in the package goes
// through this helper so the JSON shape (always with store_id) stays
// consistent for the watermark publisher.
func (s *Service) enqueueOutbox(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, aggregate string, aggregateID uuid.UUID, eventType string, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("order: marshal outbox payload: %w", err)
	}
	return s.outbox.EnqueueInTx(ctx, tx, &outbox.OutboxEvent{
		TenantID:    tenantID.String(),
		Aggregate:   aggregate,
		AggregateID: aggregateID.String(),
		EventType:   eventType,
		Payload:     datatypes.JSON(b),
	})
}
