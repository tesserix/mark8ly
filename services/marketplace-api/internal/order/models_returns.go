package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Return is the return-lifecycle sibling of Order. RESTRICT on order_id at the
// DB layer means an order with any return cannot be hard-deleted — soft delete
// via Order.DeletedAt is the only delete path in slice 1.
type Return struct {
	ID           uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID        `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID        `gorm:"column:store_id;type:uuid;not null"`
	OrderID      uuid.UUID        `gorm:"column:order_id;type:uuid;not null"`
	ReturnNumber string           `gorm:"column:return_number;type:varchar(40);not null"`
	Status       string           `gorm:"column:status;type:varchar(20);not null;default:requested"`
	Reason       *string          `gorm:"column:reason;type:varchar(80)"`
	Notes        *string          `gorm:"column:notes;type:text"`
	RefundAmount *decimal.Decimal `gorm:"column:refund_amount;type:numeric(12,2)"`
	CurrencyCode string           `gorm:"column:currency_code;type:char(3);not null"`
	RequestedAt  time.Time        `gorm:"column:requested_at;not null;default:now()"`
	ReceivedAt   *time.Time       `gorm:"column:received_at"`
	RefundedAt   *time.Time       `gorm:"column:refunded_at"`
	CreatedAt    time.Time        `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt    time.Time        `gorm:"column:updated_at;not null;default:now()"`
}

func (Return) TableName() string { return "returns" }

// ReturnItem references OrderItem with ON DELETE RESTRICT at the DB layer.
// That RESTRICT is the lower half of the chain that makes hard-delete of an
// order with returns impossible.
type ReturnItem struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReturnID    uuid.UUID `gorm:"column:return_id;type:uuid;not null"`
	OrderItemID uuid.UUID `gorm:"column:order_item_id;type:uuid;not null"`
	Quantity    int       `gorm:"column:quantity;type:integer;not null"`
	Reason      *string   `gorm:"column:reason;type:varchar(80)"`
}

func (ReturnItem) TableName() string { return "return_items" }
