package instore

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RefundService records refunds of in-store sales made outside mark8ly
// (#1110): the money went back on the shop's own terminal or in cash, so no
// payment provider is involved.
type RefundService struct {
	db     *gorm.DB
	orders *order.Service
}

func NewRefundService(db *gorm.DB) *RefundService {
	return &RefundService{db: db, orders: order.NewService(db, order.NewRepository(), outbox.NewRepository(db))}
}

// RefundItem puts quantity units of an order line back in stock.
type RefundItem struct {
	OrderItemID uuid.UUID
	Quantity    int
}

type RefundInput struct {
	StoreID    uuid.UUID
	OrderID    uuid.UUID
	RequestID  string           // reused verbatim on retry
	Amount     *decimal.Decimal // nil: the whole remaining balance
	Method     string           // how the money went back
	Reason     string
	RefundedBy string
	Items      []RefundItem
}

type RefundResult struct {
	PaymentStatus  string          `json:"payment_status"`
	RefundedAmount decimal.Decimal `json:"refunded_amount"`
	Reused         bool            `json:"reused"`
}

func validateRefund(in RefundInput) error {
	if id := strings.TrimSpace(in.RequestID); id == "" || len(id) > 64 {
		return apperrors.ValidationFailed("refund_request_id", "required, at most 64 characters")
	}
	switch in.Method {
	case PaymentCard, PaymentCash, PaymentTransfer, PaymentOther:
	default:
		return apperrors.ValidationFailed("method", "how the money went back is required")
	}
	if in.Amount != nil && !in.Amount.IsPositive() {
		return apperrors.ValidationFailed("amount", "must be more than zero")
	}
	for _, it := range in.Items {
		if it.Quantity <= 0 {
			return apperrors.ValidationFailed("items", "quantities must be more than zero")
		}
	}
	return nil
}

// Record refunds an in-store order and restocks the chosen lines in one
// transaction. A request id already recorded for the order returns the
// order's current state without writing anything.
func (s *RefundService) Record(ctx context.Context, in RefundInput) (*RefundResult, error) {
	if err := validateRefund(in); err != nil {
		return nil, err
	}
	var res RefundResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var o order.Order
		if err := tx.Raw(`SELECT * FROM orders WHERE id = ? AND store_id = ? AND deleted_at IS NULL FOR UPDATE`,
			in.OrderID, in.StoreID).Scan(&o).Error; err != nil {
			return err
		}
		if o.ID == uuid.Nil {
			return apperrors.NotFound("order")
		}
		var seen int64
		if err := tx.Raw(`SELECT count(*) FROM in_store_refunds WHERE order_id = ? AND request_id = ?`,
			in.OrderID, in.RequestID).Scan(&seen).Error; err != nil {
			return err
		}
		if seen > 0 {
			res = RefundResult{PaymentStatus: o.PaymentStatus, RefundedAmount: o.RefundedAmount, Reused: true}
			return nil
		}
		if o.Channel != order.ChannelInStore {
			return apperrors.ValidationFailed("order", "only in-store sales can be refunded outside mark8ly")
		}

		remaining := o.GrandTotal.Sub(o.RefundedAmount)
		amount := remaining
		if in.Amount != nil {
			amount = in.Amount.Round(2)
		}
		if !remaining.IsPositive() || amount.GreaterThan(remaining) {
			return apperrors.ValidationFailed("amount", fmt.Sprintf("at most %s can be refunded", remaining.StringFixed(2)))
		}
		target := order.PaymentStatusPartiallyRefunded
		if amount.Equal(remaining) {
			target = order.PaymentStatusRefunded
		}
		if err := s.orders.RecordRefund(ctx, tx, o.ID, amount, target, in.Reason); err != nil {
			return err
		}

		var refundID uuid.UUID
		if err := tx.Raw(`INSERT INTO in_store_refunds (order_id, request_id, amount, method, reason, refunded_by)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, '')) RETURNING id`,
			o.ID, in.RequestID, amount, in.Method, strings.TrimSpace(in.Reason), in.RefundedBy).Row().Scan(&refundID); err != nil {
			return err
		}
		for _, it := range in.Items {
			if err := s.restock(ctx, tx, &o, refundID, it); err != nil {
				return err
			}
		}
		res = RefundResult{PaymentStatus: string(target), RefundedAmount: o.RefundedAmount.Add(amount)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *RefundService) restock(ctx context.Context, tx *gorm.DB, o *order.Order, refundID uuid.UUID, it RefundItem) error {
	var line struct {
		VariantID *uuid.UUID
		Quantity  int
		Restocked int
	}
	if err := tx.Raw(`
		SELECT oi.variant_id, oi.quantity,
		       COALESCE((SELECT SUM(ri.quantity) FROM in_store_refund_items ri WHERE ri.order_item_id = oi.id), 0) AS restocked
		  FROM order_items oi WHERE oi.id = ? AND oi.order_id = ?`, it.OrderItemID, o.ID).Scan(&line).Error; err != nil {
		return err
	}
	if line.Quantity == 0 {
		return apperrors.ValidationFailed("items", "that line is not on this order")
	}
	if line.VariantID == nil {
		return apperrors.ValidationFailed("items", "custom items have no stock to put back")
	}
	if line.Restocked+it.Quantity > line.Quantity {
		return apperrors.ValidationFailed("items", fmt.Sprintf("only %d more can go back in stock", line.Quantity-line.Restocked))
	}
	if err := tx.Exec(`INSERT INTO in_store_refund_items (refund_id, order_item_id, quantity) VALUES (?, ?, ?)`,
		refundID, it.OrderItemID, it.Quantity).Error; err != nil {
		return err
	}

	// Put the units back where the sale took them from, location by
	// location, after what earlier refunds of this variant already put back.
	// A sale with no committed holds (continue-selling past zero) falls back
	// to where the variant is stocked, else the store's default warehouse.
	var holds []struct {
		LocationID uuid.UUID
		Qty        int
	}
	if err := tx.Raw(`
		SELECT location_id, qty FROM stock_holds
		 WHERE cart_token = ? AND variant_id = ? AND state = 'committed'
		 ORDER BY location_id`,
		SaleCartToken(o.StoreID, o.IdempotencyKey), *line.VariantID).Scan(&holds).Error; err != nil {
		return err
	}
	var earlier int
	if err := tx.Raw(`
		SELECT COALESCE(SUM(ri.quantity), 0) FROM in_store_refund_items ri
		  JOIN order_items oi ON oi.id = ri.order_item_id
		 WHERE oi.order_id = ? AND oi.variant_id = ? AND ri.refund_id <> ?`,
		o.ID, *line.VariantID, refundID).Row().Scan(&earlier); err != nil {
		return err
	}
	left := it.Quantity
	for _, h := range holds {
		taken := min(earlier, h.Qty) // already returned here by earlier refunds
		earlier -= taken
		n := min(left, h.Qty-taken)
		if n > 0 {
			if err := addStock(tx, *line.VariantID, h.LocationID, n); err != nil {
				return err
			}
			left -= n
		}
	}
	if left == 0 {
		return nil
	}
	var loc *uuid.UUID
	if err := tx.Raw(`
		SELECT COALESCE(
		  (SELECT vs.location_id FROM variant_stock vs WHERE vs.variant_id = ? ORDER BY vs.location_id LIMIT 1),
		  (SELECT w.id FROM warehouses w WHERE w.store_id = ? ORDER BY w.is_default DESC, w.created_at LIMIT 1))`,
		*line.VariantID, o.StoreID).Row().Scan(&loc); err != nil {
		return err
	}
	if loc == nil {
		return nil // nowhere stock is kept for this store: nothing to put back
	}
	return addStock(tx, *line.VariantID, *loc, left)
}

func addStock(tx *gorm.DB, variantID, locationID uuid.UUID, qty int) error {
	return tx.Exec(`
		INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES (?, ?, ?, now())
		ON CONFLICT (variant_id, location_id) DO UPDATE SET quantity = variant_stock.quantity + EXCLUDED.quantity, updated_at = now()`,
		variantID, locationID, qty).Error
}
