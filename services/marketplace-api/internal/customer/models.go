// Package customer provides the domain model, repository, and service
// for customer profiles and addresses. A profile IS the customer's
// membership of one store: UNIQUE (store_id, email). It is created only
// by an explicit join (Service.JoinStore) — never as a side effect of an
// authenticated request, which is what session paths use
// Service.LookupProfile for.
package customer

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ProfileStatus constants match the CHECK on customer_profiles.status.
const (
	StatusActive  = "active"
	StatusBlocked = "blocked"
)

// CustomerProfile maps to the customer_profiles table.
type CustomerProfile struct {
	ID             uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	GipUID         *string        `gorm:"column:gip_uid;type:varchar(200)"`
	Email          string         `gorm:"column:email;type:varchar(300);not null"`
	FirstName      *string        `gorm:"column:first_name;type:varchar(200)"`
	LastName       *string        `gorm:"column:last_name;type:varchar(200)"`
	Phone          *string        `gorm:"column:phone;type:varchar(40)"`
	AvatarURL      *string        `gorm:"column:avatar_url;type:text"`
	Tags           pq.StringArray `gorm:"column:tags;type:text[];not null;default:'{}'"`
	Status         string         `gorm:"column:status;type:varchar(20);not null;default:active"`
	BlockReason    *string        `gorm:"column:block_reason;type:varchar(300)"`
	Notes          *string        `gorm:"column:notes;type:text"`
	MarketingOptIn bool           `gorm:"column:marketing_opt_in;type:boolean;not null;default:false"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerProfile) TableName() string { return "customer_profiles" }

// CustomerAddress maps to the customer_addresses table.
type CustomerAddress struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	CustomerID  uuid.UUID `gorm:"column:customer_id;type:uuid;not null"`
	Label       *string   `gorm:"column:label;type:varchar(100)"`
	IsDefault   bool      `gorm:"column:is_default;type:boolean;not null;default:false"`
	Name        string    `gorm:"column:name;type:varchar(200);not null"`
	Line1       string    `gorm:"column:line1;type:varchar(300);not null"`
	Line2       *string   `gorm:"column:line2;type:varchar(300)"`
	City        string    `gorm:"column:city;type:varchar(200);not null"`
	Region      *string   `gorm:"column:region;type:varchar(200)"`
	PostalCode  *string   `gorm:"column:postal_code;type:varchar(40)"`
	CountryCode string    `gorm:"column:country_code;type:char(2);not null"`
	Phone       *string   `gorm:"column:phone;type:varchar(40)"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerAddress) TableName() string { return "customer_addresses" }

// CustomerWithStats extends CustomerProfile with computed aggregation
// fields populated via correlated subqueries against the orders table.
type CustomerWithStats struct {
	CustomerProfile
	OrderCount  int64      `gorm:"column:order_count"  json:"order_count"`
	TotalSpent  float64    `gorm:"column:total_spent"  json:"total_spent"`
	LastOrderAt *time.Time `gorm:"column:last_order_at" json:"last_order_at"`
}

// ListCustomersQuery holds the parsed query params for the admin customer list.
type ListCustomersQuery struct {
	Search   string
	Status   string
	Tag      string
	SortBy   string
	SortDir  string
	Page     int
	PageSize int
}

func (q *ListCustomersQuery) Defaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}
	if q.SortBy == "" {
		q.SortBy = "created_at"
	}
	if q.SortDir == "" {
		q.SortDir = "desc"
	}
}

// sanitizeSortColumn maps user-supplied sort columns to safe SQL identifiers.
func sanitizeSortColumn(col string) string {
	switch col {
	case "order_count", "total_spent", "last_order_at":
		return col
	case "email":
		return "cp.email"
	case "name":
		return "cp.first_name"
	default:
		return "cp.created_at"
	}
}

// sanitizeSortDir normalises the sort direction to ASC or DESC.
func sanitizeSortDir(dir string) string {
	if strings.EqualFold(dir, "asc") {
		return "ASC"
	}
	return "DESC"
}

// StorefrontProfileResponse is the customer-facing DTO. Excludes admin-only
// fields: notes, block_reason, status, tags.
type StorefrontProfileResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	FirstName      string `json:"first_name,omitempty"`
	LastName       string `json:"last_name,omitempty"`
	Phone          string `json:"phone,omitempty"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	MarketingOptIn bool   `json:"marketing_opt_in"`
	CreatedAt      string `json:"created_at"`
}

// AddressResponse is the DTO returned for customer addresses.
type AddressResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	IsDefault   bool   `json:"is_default"`
	Name        string `json:"name"`
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone,omitempty"`
}

// UpdateProfileRequest is the storefront PATCH /account payload.
type UpdateProfileRequest struct {
	FirstName      *string `json:"first_name" binding:"omitempty,max=200"`
	LastName       *string `json:"last_name" binding:"omitempty,max=200"`
	Phone          *string `json:"phone" binding:"omitempty,max=40"`
	AvatarURL      *string `json:"avatar_url" binding:"omitempty,url,max=2048"`
	MarketingOptIn *bool   `json:"marketing_opt_in"`
}

// CreateAddressRequest is the storefront POST /account/addresses payload.
type CreateAddressRequest struct {
	Label       *string `json:"label" binding:"omitempty,max=100"`
	IsDefault   bool    `json:"is_default"`
	Name        string  `json:"name" binding:"required,max=200"`
	Line1       string  `json:"line1" binding:"required,max=300"`
	Line2       *string `json:"line2" binding:"omitempty,max=300"`
	City        string  `json:"city" binding:"required,max=200"`
	Region      *string `json:"region" binding:"omitempty,max=200"`
	PostalCode  *string `json:"postal_code" binding:"omitempty,max=40"`
	CountryCode string  `json:"country_code" binding:"required,len=2"`
	Phone       *string `json:"phone" binding:"omitempty,max=40"`
}

// UpdateAddressRequest is the storefront PATCH /account/addresses/:id payload.
type UpdateAddressRequest struct {
	Label       *string `json:"label" binding:"omitempty,max=100"`
	IsDefault   *bool   `json:"is_default"`
	Name        *string `json:"name" binding:"omitempty,max=200"`
	Line1       *string `json:"line1" binding:"omitempty,max=300"`
	Line2       *string `json:"line2" binding:"omitempty,max=300"`
	City        *string `json:"city" binding:"omitempty,max=200"`
	Region      *string `json:"region" binding:"omitempty,max=200"`
	PostalCode  *string `json:"postal_code" binding:"omitempty,max=40"`
	CountryCode *string `json:"country_code" binding:"omitempty,len=2"`
	Phone       *string `json:"phone" binding:"omitempty,max=40"`
}
