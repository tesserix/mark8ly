// Package tenant owns the canonical Tenant entity for Mark8ly.
//
// A tenant is a merchant on the platform. Created at the end of the
// onboarding flow, persists for the lifetime of the merchant's account.
//
// Design notes:
//   - The slug is the public identifier (used in subdomains). It cannot
//     be changed once assigned without breaking inbound URLs.
//   - The owner_user_id is the GIP UID of the user who completed onboarding.
//     There is exactly one owner per tenant for v1; staff/RBAC come later
//     and live in OpenFGA tuples, not on this row.
//   - Country/currency/timezone are FK references to the location domain
//     so the seeded reference data is the only source of truth.
package tenant

import "time"

// Tenant is a merchant on Mark8ly.
type Tenant struct {
	ID            string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	Slug          string    `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"        json:"slug"`
	Name          string    `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	OwnerUserID   string    `gorm:"column:owner_user_id;type:varchar(128);not null;index"    json:"owner_user_id"`
	OwnerEmail    string    `gorm:"column:owner_email;type:varchar(320);not null;index"      json:"owner_email"`
	CountryCode   string    `gorm:"column:country_code;type:char(2);not null"                json:"country_code"`
	CurrencyCode  string    `gorm:"column:currency_code;type:char(3);not null"               json:"currency_code"`
	Timezone      string    `gorm:"column:timezone;type:varchar(64);not null"                json:"timezone"`
	Status        string    `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName overrides GORM's default pluralization.
func (Tenant) TableName() string { return "tenants" }

// Status constants. Match the CHECK constraint in the migration.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
