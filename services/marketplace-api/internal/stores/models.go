// Package stores holds the local read-only projection of the authoritative
// store table owned by platform-api. Populated via lazy pull-through from
// StoreMiddleware (see spec §14.7). Never written outside of middleware.
package stores

import "time"

// Store is the marketplace-api view of a tenant's storefront.
// The canonical source is platform-api's stores table. This projection
// exists so StoreMiddleware can look up store metadata without an HTTP
// round-trip on every admin request (db-f1-micro 5-conn pool).
type Store struct {
	ID           string    `gorm:"primaryKey;column:id;type:uuid"                          json:"id"`
	TenantID     string    `gorm:"column:tenant_id;type:uuid;not null"                     json:"tenant_id"`
	Slug         string    `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"       json:"slug"`
	Name         string    `gorm:"column:name;type:varchar(200);not null"                  json:"name"`
	CountryCode  string    `gorm:"column:country_code;type:char(2);not null"               json:"country_code"`
	CurrencyCode string    `gorm:"column:currency_code;type:char(3);not null"              json:"currency_code"`
	Timezone     string    `gorm:"column:timezone;type:varchar(64);not null"               json:"timezone"`
	Status       string    `gorm:"column:status;type:varchar(20);not null"                 json:"status"`
	SyncedAt     time.Time `gorm:"column:synced_at;not null;default:now()"                 json:"synced_at"`
}

func (Store) TableName() string { return "stores" }

// StoreWatermark is bumped asynchronously by the outbox publisher after
// any product/variant/media/category mutation. Storefront ETag reads
// from this table, not from stores itself — the separation eliminates
// the hot-row lock on the authoritative store row (spec §14.1).
type StoreWatermark struct {
	StoreID            string    `gorm:"primaryKey;column:store_id;type:uuid"             json:"store_id"`
	ProductsUpdatedAt  time.Time `gorm:"column:products_updated_at;not null;default:now()" json:"products_updated_at"`
}

func (StoreWatermark) TableName() string { return "store_watermarks" }

// Status constants match the CHECK constraint in migration 000001.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
