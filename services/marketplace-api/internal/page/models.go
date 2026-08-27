package page

import (
	"time"

	"github.com/google/uuid"
)

// Page is a merchant-authored content page scoped to a store. Used for
// About / Terms / Privacy / etc. Body is markdown; rendering happens in
// the storefront with react-markdown + skipHtml.
type Page struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID        uuid.UUID `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Slug           string    `gorm:"column:slug;type:varchar(63);not null"                    json:"slug"`
	Title          string    `gorm:"column:title;type:varchar(200);not null"                  json:"title"`
	Body           string    `gorm:"column:body;type:text;not null;default:''"                json:"body"`
	SEOTitle       *string   `gorm:"column:seo_title;type:varchar(200)"                       json:"seo_title,omitempty"`
	SEODescription *string   `gorm:"column:seo_description;type:varchar(300)"                 json:"seo_description,omitempty"`
	// No `default:` tag, deliberately. GORM omits a zero-valued field from
	// the INSERT when it carries one, and `false` IS the zero value for a
	// plain bool -- so `default:true` made `Published: false` unwritable and
	// served unpublished pages to customers (#394). Service.Create supplies
	// the true default in Go, so nothing here depended on the column default;
	// the column keeps its DEFAULT true in SQL for direct inserts.
	Published bool      `gorm:"column:published;not null"                                json:"published"`
	SortOrder int       `gorm:"column:sort_order;not null;default:0"                     json:"sort_order"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName pins the Page struct to the `pages` table.
func (Page) TableName() string { return "pages" }
