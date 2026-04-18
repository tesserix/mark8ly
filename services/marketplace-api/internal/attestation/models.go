package attestation

import (
	"time"

	"github.com/google/uuid"
)

// BusinessEntityAttestation is the GORM model for the
// business_entity_attestations table. It records the moment a merchant
// checks a jurisdiction-specific checkbox affirming they are a registered
// business entity — required for B2B reverse-charge VAT compliance.
// The raw IP is never stored; only a hashed value (ip_hash) is kept.
// Task 15 adds the full security test suite for this model.
type BusinessEntityAttestation struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	StoreID         uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	TenantID        uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	Country         string    `gorm:"column:country;type:char(2);not null"`
	CheckboxText    string    `gorm:"column:checkbox_text;not null"`
	CheckboxVersion string    `gorm:"column:checkbox_version;type:varchar(20);not null"`
	UserAgent       *string   `gorm:"column:user_agent"`
	IPHash          *string   `gorm:"column:ip_hash;type:varchar(64)"`
	SignedAt        time.Time `gorm:"column:signed_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (BusinessEntityAttestation) TableName() string { return "business_entity_attestations" }
