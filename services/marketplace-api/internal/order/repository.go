package order

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for the order aggregate. Every
// mutating method takes an explicit *gorm.DB so callers can thread the
// caller's transaction in (cross-module tx pattern). Read methods take a
// context-bound *gorm.DB and may run inside or outside a tx — both work.
type Repository interface {
	// Create inserts a single orders row plus the supplied items, addresses,
	// and the initial order_events row, all in the provided tx. The caller
	// is responsible for opening / committing the tx (use order.Service.Unit).
	CreateInTx(tx *gorm.DB, o *Order, items []OrderItem, addrs []OrderAddress, initial OrderEvent) error

	// GetByID returns the order with its items and addresses populated, or
	// apperrors.ErrNotFound if missing.
	GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Order, []OrderItem, []OrderAddress, error)

	// GetByIdempotencyKey returns the order matching (store_id, key), or
	// (nil, nil, nil, nil) if no row exists. Used by Service.Create as the
	// happy-path retry lookup.
	GetByIdempotencyKey(ctx context.Context, db *gorm.DB, storeID uuid.UUID, key string) (*Order, []OrderItem, []OrderAddress, error)

	// UpdateOrderStatus / UpdatePaymentStatus / UpdateFulfillmentStatus each
	// run a targeted UPDATE inside the supplied tx. They do NOT enforce
	// transition legality — the service layer does that before calling.
	UpdateOrderStatus(tx *gorm.DB, id uuid.UUID, target OrderStatus) error
	UpdatePaymentStatus(tx *gorm.DB, id uuid.UUID, target PaymentStatus) error
	UpdateFulfillmentStatus(tx *gorm.DB, id uuid.UUID, target FulfillmentStatus) error

	// RecordRefund is a single atomic UPDATE that adds amount to
	// refunded_amount, fails if it would exceed grand_total, and updates
	// payment_status to the supplied target. Returns the new refunded_amount
	// and the (potentially) updated payment_status, or apperrors.ErrRefundExceedsTotal.
	RecordRefund(tx *gorm.DB, id uuid.UUID, amount decimal.Decimal, paymentStatusTarget PaymentStatus) (newRefunded decimal.Decimal, err error)

	// AppendEvent inserts an order_events row inside the supplied tx.
	AppendEvent(tx *gorm.DB, evt *OrderEvent) error
}

type gormRepository struct{}

// NewRepository constructs a stateless repository. The *gorm.DB is supplied
// per-call so the same instance can serve every transaction.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) CreateInTx(tx *gorm.DB, o *Order, items []OrderItem, addrs []OrderAddress, initial OrderEvent) error {
	if err := tx.Create(o).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].OrderID = o.ID
	}
	if len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
	}
	for i := range addrs {
		addrs[i].OrderID = o.ID
	}
	if len(addrs) > 0 {
		if err := tx.Create(&addrs).Error; err != nil {
			return err
		}
	}
	initial.OrderID = o.ID
	return tx.Create(&initial).Error
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Order, []OrderItem, []OrderAddress, error) {
	var o Order
	err := db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, apperrors.NotFound("order")
	}
	if err != nil {
		return nil, nil, nil, err
	}
	items, addrs, err := loadChildren(ctx, db, o.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	return &o, items, addrs, nil
}

func (gormRepository) GetByIdempotencyKey(ctx context.Context, db *gorm.DB, storeID uuid.UUID, key string) (*Order, []OrderItem, []OrderAddress, error) {
	var o Order
	err := db.WithContext(ctx).
		Where("store_id = ? AND idempotency_key = ? AND deleted_at IS NULL", storeID, key).
		First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	items, addrs, err := loadChildren(ctx, db, o.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	return &o, items, addrs, nil
}

func loadChildren(ctx context.Context, db *gorm.DB, orderID uuid.UUID) ([]OrderItem, []OrderAddress, error) {
	var items []OrderItem
	if err := db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	var addrs []OrderAddress
	if err := db.WithContext(ctx).Where("order_id = ?", orderID).Find(&addrs).Error; err != nil {
		return nil, nil, err
	}
	return items, addrs, nil
}

func (gormRepository) UpdateOrderStatus(tx *gorm.DB, id uuid.UUID, target OrderStatus) error {
	res := tx.Model(&Order{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"status": string(target), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("order")
	}
	return nil
}

func (gormRepository) UpdatePaymentStatus(tx *gorm.DB, id uuid.UUID, target PaymentStatus) error {
	res := tx.Model(&Order{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"payment_status": string(target), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("order")
	}
	return nil
}

func (gormRepository) UpdateFulfillmentStatus(tx *gorm.DB, id uuid.UUID, target FulfillmentStatus) error {
	updates := map[string]any{
		"fulfillment_status": string(target),
		"updated_at":         gorm.Expr("now()"),
	}
	if target == FulfillmentStatusFulfilled {
		updates["fulfilled_at"] = gorm.Expr("now()")
	}
	res := tx.Model(&Order{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("order")
	}
	return nil
}

// RecordRefund executes the atomic guard:
//
//	UPDATE orders
//	   SET refunded_amount = refunded_amount + $amount,
//	       payment_status  = $target,
//	       updated_at      = now()
//	 WHERE id = $id
//	   AND deleted_at IS NULL
//	   AND refunded_amount + $amount <= grand_total
//	 RETURNING refunded_amount;
//
// If zero rows are returned the refund would have exceeded grand_total
// (or the order does not exist) — we then look up the row to distinguish
// the two cases and return the appropriate apperrors.* sentinel.
func (gormRepository) RecordRefund(tx *gorm.DB, id uuid.UUID, amount decimal.Decimal, paymentStatusTarget PaymentStatus) (decimal.Decimal, error) {
	var newRefunded decimal.Decimal
	err := tx.Raw(
		`UPDATE orders
		    SET refunded_amount = refunded_amount + ?,
		        payment_status  = ?,
		        updated_at      = now()
		  WHERE id = ?
		    AND deleted_at IS NULL
		    AND refunded_amount + ? <= grand_total
		RETURNING refunded_amount`,
		amount, string(paymentStatusTarget), id, amount,
	).Scan(&newRefunded).Error
	if err != nil {
		return decimal.Zero, err
	}
	if newRefunded.IsZero() {
		// Either the order doesn't exist or the guard rejected the update.
		// Look up the row to disambiguate.
		var existing Order
		if lookupErr := tx.Where("id = ? AND deleted_at IS NULL", id).First(&existing).Error; lookupErr != nil {
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return decimal.Zero, apperrors.NotFound("order")
			}
			return decimal.Zero, lookupErr
		}
		// Order exists; the guard fired.
		return decimal.Zero, apperrors.RefundExceedsTotal(
			existing.GrandTotal.String(),
			amount.String(),
			existing.RefundedAmount.String(),
		)
	}
	return newRefunded, nil
}

func (gormRepository) AppendEvent(tx *gorm.DB, evt *OrderEvent) error {
	return tx.Create(evt).Error
}
