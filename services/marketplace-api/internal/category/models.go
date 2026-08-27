// Package category owns the Category tree model. Categories are per-store
// (the real scope) with tenant_id denormalised for fast tenant-wide admin
// queries. Soft-delete via deleted_at; partial unique index on (store_id,
// slug) WHERE deleted_at IS NULL so deleted slugs are reusable.
package category

import "time"

// Category is one node in the per-store category tree.
type Category struct {
	ID          string  `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID    string  `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID     string  `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	ParentID    *string `gorm:"column:parent_id;type:uuid"                               json:"parent_id,omitempty"`
	Name        string  `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	Slug        string  `gorm:"column:slug;type:varchar(200);not null"                   json:"slug"`
	Description *string `gorm:"column:description;type:text"                             json:"description,omitempty"`
	ImageURL    *string `gorm:"column:image_url;type:text"                               json:"image_url,omitempty"`
	Position    int     `gorm:"column:position;not null;default:0"                       json:"position"`
	// No `default:` tag -- see page.Published (#394). A category created
	// with IsActive=false was silently stored active, so nothing could be
	// deactivated. toServiceCreateCategory supplies the true default in Go.
	IsActive  bool       `gorm:"column:is_active;not null"                                json:"is_active"`
	Featured  bool       `gorm:"column:featured;not null;default:false"                   json:"featured"`
	CreatedAt time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`
}

func (Category) TableName() string { return "categories" }
