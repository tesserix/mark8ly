package vendor

import "time"

// Vendor is a seller under a tenant. Every tenant has exactly one
// self-vendor (is_self=true) representing the tenant itself. Real
// marketplace vendors (Phase 8+) will be additional rows with
// is_self=false.
type Vendor struct {
	ID        string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TenantID  string    `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	Name      string    `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	Slug      string    `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"        json:"slug"`
	Status    string    `gorm:"column:status;type:varchar(32);not null;default:active"   json:"status"`
	IsSelf    bool      `gorm:"column:is_self;not null;default:false"                    json:"is_self"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName pins the Vendor struct to the `vendors` table.
func (Vendor) TableName() string { return "vendors" }

// Status values.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)
