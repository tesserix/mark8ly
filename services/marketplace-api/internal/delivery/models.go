// Package delivery holds a store's own flat-rate delivery options and its
// own couriers, used when stores.shipping_mode is "own".
package delivery

import (
	"time"

	"github.com/shopspring/decimal"
)

// Option is one flat-rate delivery choice shown at checkout.
type Option struct {
	ID        string          `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  string          `gorm:"column:tenant_id;type:uuid;not null" json:"tenant_id"`
	StoreID   string          `gorm:"column:store_id;type:uuid;not null" json:"store_id"`
	Name      string          `gorm:"column:name;type:varchar(40);not null" json:"name"`
	Price     decimal.Decimal `gorm:"column:price;type:numeric(12,2);not null" json:"price"`
	Estimate  *string         `gorm:"column:estimate;type:varchar(40)" json:"estimate"`
	Enabled   bool            `gorm:"column:enabled;not null;default:true" json:"enabled"`
	Position  int             `gorm:"column:position;not null;default:0" json:"position"`
	CreatedAt time.Time       `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time       `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Option) TableName() string { return "store_delivery_options" }

// Courier is a delivery person or company the store uses itself.
type Courier struct {
	ID                  string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID            string    `gorm:"column:tenant_id;type:uuid;not null" json:"tenant_id"`
	StoreID             string    `gorm:"column:store_id;type:uuid;not null" json:"store_id"`
	Name                string    `gorm:"column:name;type:varchar(40);not null" json:"name"`
	TrackingURLTemplate *string   `gorm:"column:tracking_url_template;type:varchar(500)" json:"tracking_url_template"`
	CreatedAt           time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Courier) TableName() string { return "store_couriers" }
