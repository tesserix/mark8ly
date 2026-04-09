package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// AbandonedCart is a first-class row written by the storefront cart service.
// NOT a pending order — no order_number, no payment state, no fulfillment.
// Upserted by (store_id, cart_session_id) so a single browser keeps exactly
// one row regardless of how often the cart changes.
type AbandonedCart struct {
	ID               uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	CartSessionID    string          `gorm:"column:cart_session_id;type:varchar(100);not null"`
	CustomerEmail    *string         `gorm:"column:customer_email;type:varchar(320)"`
	CustomerName     *string         `gorm:"column:customer_name;type:varchar(200)"`
	CustomerID       *uuid.UUID      `gorm:"column:customer_id;type:uuid"`
	ItemCount        int             `gorm:"column:item_count;type:integer;not null"`
	Subtotal         decimal.Decimal `gorm:"column:subtotal;type:numeric(12,2);not null"`
	CurrencyCode     string          `gorm:"column:currency_code;type:char(3);not null"`
	ItemsSnapshot    datatypes.JSON  `gorm:"column:items_snapshot;type:jsonb;not null"`
	RecoveryURL      *string         `gorm:"column:recovery_url;type:text"`
	LastActiveAt     time.Time       `gorm:"column:last_active_at;not null"`
	RecoverySentAt   *time.Time      `gorm:"column:recovery_sent_at"`
	ConvertedOrderID *uuid.UUID      `gorm:"column:converted_order_id;type:uuid"`
	CreatedAt        time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (AbandonedCart) TableName() string { return "abandoned_carts" }
