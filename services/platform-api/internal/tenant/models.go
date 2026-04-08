// Package tenant owns the canonical Tenant entity for Mark8ly.
//
// Phase Q: a tenant is a COMPANY — the billable, FGA-scoped unit
// that owns one or more stores. The user-facing storefront identity
// (slug, currency, timezone, country, logo) lives on `store`, not
// here. What's left on `tenant` is the owner identity (the founder
// GIP UID and email) and the company name.
//
// Design notes:
//   - owner_user_id is the GIP UID of the user who completed
//     onboarding. There is exactly one owner per tenant; additional
//     humans join via the Phase P invite flow and land as admin/
//     staff/viewer relations in OpenFGA.
//   - `name` is the company name. Onboarding copies the merchant's
//     "business name" into BOTH tenant.name (company) and
//     store.name (the default store's public name). A future rename
//     flow can separate them.
//   - No slug column — the URL identity moved to `store.slug` and a
//     tenant with multiple stores has multiple slugs.
package tenant

import "time"

// Tenant is a merchant company on Mark8ly.
type Tenant struct {
	ID          string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	OwnerUserID string    `gorm:"column:owner_user_id;type:varchar(128);not null;index"    json:"owner_user_id"`
	OwnerEmail  string    `gorm:"column:owner_email;type:varchar(320);not null;index"      json:"owner_email"`
	Status      string    `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName overrides GORM's default pluralization.
func (Tenant) TableName() string { return "tenants" }

// Status constants. Match the CHECK constraint in the migration.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
