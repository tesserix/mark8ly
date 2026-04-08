// Package store owns the Store entity — one row per storefront
// under a tenant. Introduced in Phase Q to separate "the company"
// from "the branded shops the company runs".
//
// Design notes:
//   - Slug is globally unique (not just per-tenant) because the
//     storefront URL must be globally unique.
//   - Country / currency / timezone are store-level because two
//     stores under the same tenant can serve different regions
//     with different currencies.
//   - `name` is the store's public display name. A solo-founder
//     tenant has a single store named "Main Store" by default
//     after onboarding; the merchant can rename it from the
//     store settings page.
//   - `logo_url` is reserved for a future upload slice; Phase Q
//     ships it as a nullable text column so we don't need another
//     migration later.
package store

import (
	"encoding/json"
	"time"
)

// Store is one storefront under a tenant.
type Store struct {
	ID              string          `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID        string          `gorm:"column:tenant_id;type:uuid;not null;index"                json:"tenant_id"`
	Slug            string          `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"        json:"slug"`
	Name            string          `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	CountryCode     string          `gorm:"column:country_code;type:char(2);not null"                json:"country_code"`
	CurrencyCode    string          `gorm:"column:currency_code;type:char(3);not null"               json:"currency_code"`
	Timezone        string          `gorm:"column:timezone;type:varchar(64);not null"                json:"timezone"`
	LogoURL         *string         `gorm:"column:logo_url;type:text"                                json:"logo_url,omitempty"`
	StorefrontTheme json.RawMessage `gorm:"column:storefront_theme;type:jsonb;not null;default:'{}'::jsonb" json:"storefront_theme"`
	Status          string          `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName overrides GORM's default pluralization.
func (Store) TableName() string { return "stores" }

// Status constants match the CHECK constraint in migration 0008.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
