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
	ID           string `gorm:"primaryKey;column:id;type:uuid"                          json:"id"`
	TenantID     string `gorm:"column:tenant_id;type:uuid;not null"                     json:"tenant_id"`
	Slug         string `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"       json:"slug"`
	Name         string `gorm:"column:name;type:varchar(200);not null"                  json:"name"`
	CountryCode  string `gorm:"column:country_code;type:char(2);not null"               json:"country_code"`
	CurrencyCode string `gorm:"column:currency_code;type:char(3);not null"              json:"currency_code"`
	Timezone     string `gorm:"column:timezone;type:varchar(64);not null"               json:"timezone"`
	Status       string `gorm:"column:status;type:varchar(20);not null"                 json:"status"`
	// CreatedAt is when the store was created in platform_api, mirrored so
	// this service can date things from it — a backfilled trial, above all
	// (#827). NULL for rows mirrored before migration 136, which is the
	// honest answer: those rows never carried it.
	//
	// NOT SyncedAt. That is when this projection last copied the row and
	// moves on every upsert, so using it would date a trial from the last
	// time a merchant edited their store settings.
	//
	// autoCreateTime:false is LOAD-BEARING. GORM auto-populates any field
	// named CreatedAt with the current time on Create, and Upsert uses
	// Create with an ON CONFLICT clause — so without this tag every upsert
	// sends now(), the COALESCE below prefers it, and the mirrored creation
	// date is overwritten with the moment of the last sync.
	//
	// That is not hypothetical: it happened in production. Four stores had
	// their real dates replaced within minutes of the column shipping, and
	// the trial backfill would then have granted a fresh 90 days to a store
	// created in April. Renaming the field would also fix it; the tag is
	// kept because `created_at` is the column's name in platform_api and
	// matching it is worth one annotation.
	CreatedAt *time.Time `gorm:"column:created_at;autoCreateTime:false"                  json:"created_at,omitempty"`
	SyncedAt  time.Time  `gorm:"column:synced_at;not null;default:now()"                 json:"synced_at"`
	// StorefrontCustomerPortalSecret is the per-store HMAC key for customer
	// portal tokens (§15.4, migration 058). Generated at migration time;
	// lazily regenerated if empty via customerportal.GenerateSecret().
	StorefrontCustomerPortalSecret string `gorm:"column:storefront_customer_portal_secret;type:char(64)" json:"-"`
}

func (Store) TableName() string { return "stores" }

// StoreWatermark is bumped asynchronously by the outbox publisher after
// any product/variant/media/category mutation. Storefront ETag reads
// from this table, not from stores itself — the separation eliminates
// the hot-row lock on the authoritative store row (spec §14.1).
type StoreWatermark struct {
	StoreID           string    `gorm:"primaryKey;column:store_id;type:uuid"             json:"store_id"`
	ProductsUpdatedAt time.Time `gorm:"column:products_updated_at;not null;default:now()" json:"products_updated_at"`
}

func (StoreWatermark) TableName() string { return "store_watermarks" }

// Status constants match the CHECK constraint in migration 000001.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
