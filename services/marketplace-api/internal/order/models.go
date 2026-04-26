package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// Order is the root aggregate. One row per checkout.
//
// Invariants (see doc.go and migration 000002_orders_initial):
//   - Status never includes 'refunded'; money state lives on PaymentStatus.
//   - RefundedAmount is written atomically via UPDATE ... WHERE in M2.
//   - IdempotencyKey is required on every row and UNIQUE per (store_id, key).
type Order struct {
	ID                uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	OrderNumber       string          `gorm:"column:order_number;type:varchar(40);not null"`
	IdempotencyKey    string          `gorm:"column:idempotency_key;type:varchar(100);not null"`
	CustomerID        *uuid.UUID      `gorm:"column:customer_id;type:uuid"`
	CustomerEmail     string          `gorm:"column:customer_email;type:varchar(320);not null"`
	CustomerName      *string         `gorm:"column:customer_name;type:varchar(200)"`
	Status            string          `gorm:"column:status;type:varchar(20);not null;default:pending"`
	PaymentStatus     string          `gorm:"column:payment_status;type:varchar(20);not null;default:pending"`
	FulfillmentStatus string          `gorm:"column:fulfillment_status;type:varchar(20);not null;default:unfulfilled"`
	Subtotal          decimal.Decimal `gorm:"column:subtotal;type:numeric(12,2);not null"`
	ShippingTotal     decimal.Decimal `gorm:"column:shipping_total;type:numeric(12,2);not null;default:0"`
	TaxTotal          decimal.Decimal `gorm:"column:tax_total;type:numeric(12,2);not null;default:0"`
	DiscountTotal     decimal.Decimal `gorm:"column:discount_total;type:numeric(12,2);not null;default:0"`
	GrandTotal        decimal.Decimal `gorm:"column:grand_total;type:numeric(12,2);not null"`
	RefundedAmount    decimal.Decimal `gorm:"column:refunded_amount;type:numeric(12,2);not null;default:0"`
	CurrencyCode      string          `gorm:"column:currency_code;type:char(3);not null"`
	PaymentProvider   *string         `gorm:"column:payment_provider;type:varchar(40)"`
	PaymentIntentID   *string         `gorm:"column:payment_intent_id;type:varchar(200)"`
	ShippingService   *string         `gorm:"column:shipping_service;type:varchar(40)"`
	ShippingCarrier   *string         `gorm:"column:shipping_carrier;type:varchar(40)"`
	Notes             *string         `gorm:"column:notes;type:text"`
	PlacedAt          time.Time       `gorm:"column:placed_at;not null;default:now()"`
	CancelledAt       *time.Time      `gorm:"column:cancelled_at"`
	FulfilledAt       *time.Time      `gorm:"column:fulfilled_at"`
	CreatedAt         time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt         time.Time       `gorm:"column:updated_at;not null;default:now()"`
	DeletedAt         *time.Time      `gorm:"column:deleted_at"`
}

func (Order) TableName() string { return "orders" }

// OrderItem is a price snapshot. product_id/variant_id are deliberately NOT FKs.
type OrderItem struct {
	ID            uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID       uuid.UUID       `gorm:"column:order_id;type:uuid;not null"`
	ProductID     *uuid.UUID      `gorm:"column:product_id;type:uuid"`
	VariantID     *uuid.UUID      `gorm:"column:variant_id;type:uuid"`
	TitleSnapshot string          `gorm:"column:title_snapshot;type:varchar(300);not null"`
	SKUSnapshot   string          `gorm:"column:sku_snapshot;type:varchar(100);not null"`
	OptionSummary *string         `gorm:"column:option_summary;type:varchar(300)"`
	UnitPrice     decimal.Decimal `gorm:"column:unit_price;type:numeric(12,2);not null"`
	Quantity      int             `gorm:"column:quantity;type:integer;not null"`
	LineTotal     decimal.Decimal `gorm:"column:line_total;type:numeric(12,2);not null"`
	CurrencyCode  string          `gorm:"column:currency_code;type:char(3);not null"`
	ImageURL      *string         `gorm:"column:image_url;type:text"`
	CreatedAt     time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (OrderItem) TableName() string { return "order_items" }

// OrderAddress is an immutable snapshot. No UpdatedAt.
type OrderAddress struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID     uuid.UUID `gorm:"column:order_id;type:uuid;not null"`
	Kind        string    `gorm:"column:kind;type:varchar(10);not null"` // 'shipping' | 'billing'
	Name        string    `gorm:"column:name;type:varchar(200);not null"`
	Line1       string    `gorm:"column:line1;type:varchar(300);not null"`
	Line2       *string   `gorm:"column:line2;type:varchar(300)"`
	City        string    `gorm:"column:city;type:varchar(200);not null"`
	Region      *string   `gorm:"column:region;type:varchar(200)"`
	PostalCode  *string   `gorm:"column:postal_code;type:varchar(40)"`
	CountryCode string    `gorm:"column:country_code;type:char(2);not null"`
	Phone       *string   `gorm:"column:phone;type:varchar(40)"`
}

func (OrderAddress) TableName() string { return "order_addresses" }

// OrderEvent is append-only. Payload is opaque JSONB.
type OrderEvent struct {
	ID         uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID    uuid.UUID      `gorm:"column:order_id;type:uuid;not null"`
	Kind       string         `gorm:"column:kind;type:varchar(40);not null"`
	ActorID    *uuid.UUID     `gorm:"column:actor_id;type:uuid"`
	ActorEmail *string        `gorm:"column:actor_email;type:varchar(320)"`
	Payload    datatypes.JSON `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt  time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (OrderEvent) TableName() string { return "order_events" }
