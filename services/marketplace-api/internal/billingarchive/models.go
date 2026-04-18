package billingarchive

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// BillingArchive is the GORM model for the billing_archive table. It is a
// long-retention record of a deleted merchant's billing history, kept for
// tax-authority compliance. Rows in this table outlive the store that
// generated them and must be retained until archive_expires_at.
type BillingArchive struct {
	ID               uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OriginalStoreID  uuid.UUID      `gorm:"column:original_store_id;type:uuid;not null"`
	OriginalTenantID uuid.UUID      `gorm:"column:original_tenant_id;type:uuid;not null"`
	BusinessName     string         `gorm:"column:business_name;type:varchar(500);not null"`
	TaxID            *string        `gorm:"column:tax_id;type:varchar(50)"`
	TaxIDCountry     *string        `gorm:"column:tax_id_country;type:char(2)"`
	BillingCountry   *string        `gorm:"column:billing_country;type:char(2)"`
	BillingCurrency  *string        `gorm:"column:billing_currency;type:char(3)"`
	StripeCustomerID string         `gorm:"column:stripe_customer_id;type:varchar(100);not null"`
	AllInvoices      datatypes.JSON `gorm:"column:all_invoices;type:jsonb;not null"`
	TotalRevenueUSD  *float64       `gorm:"column:total_revenue_usd;type:numeric(12,2)"`
	HardDeletedAt    time.Time      `gorm:"column:hard_deleted_at;not null"`
	ArchiveExpiresAt time.Time      `gorm:"column:archive_expires_at;not null"`
}

// TableName returns the database table name for GORM.
func (BillingArchive) TableName() string { return "billing_archive" }
