// Package product owns the Product aggregate and its child models:
// Option, OptionValue, Variant, VariantOptionValue, Media. Categories
// and Stores are separate packages; Product references their IDs only.
//
// Design notes:
//   - Product has no money or stock — those live on Variant.
//   - Variant has a composite FK (product_id, store_id) → products
//     (id, store_id) so store_id can never drift (spec §14.4).
//   - Variant.InventoryQuantity is maintained by a DB trigger on
//     variant_stock (spec §14.5). Do NOT write to it directly; write
//     to variant_stock and let the trigger propagate.
//   - Media.StorageKey is content-addressed (sha256-prefixed path);
//     refcount is a `count(*) on storage_key` query — the same object
//     may be referenced by multiple product_media rows after copy-to-store.
package product

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/lib/pq"
)

// Status constants match the CHECK constraint in migration 000001.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// InventoryPolicy constants.
const (
	InventoryPolicyDeny     = "deny"     // reject orders when stock hits 0
	InventoryPolicyContinue = "continue" // accept backorders, allow negative stock
)

// MediaType constants.
const (
	MediaTypeImage = "image"
	MediaTypeVideo = "video"
)

// Product is the catalog record. No prices, no stock — see Variant.
type Product struct {
	ID                  string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID            string     `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID             string     `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Handle              string     `gorm:"column:handle;type:varchar(200);not null"                 json:"handle"`
	Title               string     `gorm:"column:title;type:varchar(300);not null"                  json:"title"`
	Description         *string    `gorm:"column:description;type:text"                             json:"description,omitempty"`
	Status              string     `gorm:"column:status;type:varchar(20);not null;default:draft"    json:"status"`
	VendorID            *string    `gorm:"column:vendor_id;type:uuid"                               json:"vendor_id,omitempty"`
	Tags                pq.StringArray `gorm:"column:tags;type:text[];not null;default:'{}'"        json:"tags"`
	SEOTitle            *string    `gorm:"column:seo_title;type:varchar(300)"                       json:"seo_title,omitempty"`
	SEODescription      *string    `gorm:"column:seo_description;type:varchar(500)"                 json:"seo_description,omitempty"`
	PrimaryCategoryID   *string    `gorm:"column:primary_category_id;type:uuid"                     json:"primary_category_id,omitempty"`
	CopySourceProductID *string    `gorm:"column:copy_source_product_id;type:uuid"                  json:"copy_source_product_id,omitempty"`
	PublishedAt         *time.Time `gorm:"column:published_at"                                      json:"published_at,omitempty"`
	CreatedBy           *string    `gorm:"column:created_by;type:uuid"                              json:"created_by,omitempty"`
	UpdatedBy           *string    `gorm:"column:updated_by;type:uuid"                              json:"updated_by,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt           *time.Time `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`

	// Eager-loaded relations (optional; populated via Preload)
	Options  []Option   `gorm:"foreignKey:ProductID" json:"options,omitempty"`
	Variants []Variant  `gorm:"foreignKey:ProductID" json:"variants,omitempty"`
	Media    []Media    `gorm:"foreignKey:ProductID" json:"media,omitempty"`
}

func (Product) TableName() string { return "products" }

// Option is an option axis for a product (Size, Color, Material).
// Max 3 per product, enforced in the service layer.
type Option struct {
	ID        string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID string    `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	Name      string    `gorm:"column:name;type:varchar(100);not null"                   json:"name"`
	Position  int       `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`

	Values []OptionValue `gorm:"foreignKey:OptionID" json:"values,omitempty"`
}

func (Option) TableName() string { return "product_options" }

// OptionValue is one value on an option axis.
type OptionValue struct {
	ID        string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	OptionID  string    `gorm:"column:option_id;type:uuid;not null"                      json:"option_id"`
	Value     string    `gorm:"column:value;type:varchar(200);not null"                  json:"value"`
	Position  int       `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (OptionValue) TableName() string { return "product_option_values" }

// Variant is the sellable unit — where money and stock live.
// InventoryQuantity is trigger-maintained from variant_stock; do not write
// to it directly.
type Variant struct {
	ID                string          `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID         string          `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	StoreID           string          `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	SKU               string          `gorm:"column:sku;type:varchar(100);not null"                    json:"sku"`
	Barcode           *string         `gorm:"column:barcode;type:varchar(100)"                         json:"barcode,omitempty"`
	Price             decimal.Decimal `gorm:"column:price;type:numeric(12,2);not null"                 json:"price"`
	CompareAtPrice    *decimal.Decimal `gorm:"column:compare_at_price;type:numeric(12,2)"              json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal `gorm:"column:cost_price;type:numeric(12,2)"                    json:"cost_price,omitempty"`
	CurrencyCode      string          `gorm:"column:currency_code;type:char(3);not null"               json:"currency_code"`
	WeightGrams       *int            `gorm:"column:weight_grams"                                      json:"weight_grams,omitempty"`
	InventoryQuantity int             `gorm:"column:inventory_quantity;not null;default:0"             json:"inventory_quantity"`
	InventoryPolicy   string          `gorm:"column:inventory_policy;type:varchar(20);not null;default:deny" json:"inventory_policy"`
	LowStockThreshold *int            `gorm:"column:low_stock_threshold"                               json:"low_stock_threshold,omitempty"`
	Position          int             `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt         time.Time       `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt         *time.Time      `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`

	OptionValueLinks []VariantOptionValue `gorm:"foreignKey:VariantID" json:"option_value_links,omitempty"`
}

func (Variant) TableName() string { return "product_variants" }

// VariantOptionValue joins a variant to one option value. The pair
// (variant_id, option_value_id) is the primary key.
type VariantOptionValue struct {
	VariantID     string `gorm:"primaryKey;column:variant_id;type:uuid"      json:"variant_id"`
	OptionValueID string `gorm:"primaryKey;column:option_value_id;type:uuid" json:"option_value_id"`
}

func (VariantOptionValue) TableName() string { return "variant_option_values" }

// Media is a product-level or variant-level media asset.
// StorageKey is content-addressed; refcount via count(*) on storage_key.
type Media struct {
	ID         string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID  string    `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	VariantID  *string   `gorm:"column:variant_id;type:uuid"                              json:"variant_id,omitempty"`
	URL        string    `gorm:"column:url;type:text;not null"                            json:"url"`
	StorageKey string    `gorm:"column:storage_key;type:text;not null"                    json:"storage_key"`
	Alt        *string   `gorm:"column:alt;type:varchar(300)"                             json:"alt,omitempty"`
	Position   int       `gorm:"column:position;not null;default:0"                       json:"position"`
	MediaType  string    `gorm:"column:media_type;type:varchar(20);not null;default:image" json:"media_type"`
	Width      *int      `gorm:"column:width"                                             json:"width,omitempty"`
	Height     *int      `gorm:"column:height"                                            json:"height,omitempty"`
	Bytes      *int64    `gorm:"column:bytes"                                             json:"bytes,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (Media) TableName() string { return "product_media" }

// ProductCategory joins a product to a category (M:N).
type ProductCategory struct {
	ProductID  string `gorm:"primaryKey;column:product_id;type:uuid"  json:"product_id"`
	CategoryID string `gorm:"primaryKey;column:category_id;type:uuid" json:"category_id"`
}

func (ProductCategory) TableName() string { return "product_categories" }

// VariantStock is the per-location stock row. Writing to this table
// triggers sync_variant_inventory() which updates the denormalised
// Variant.InventoryQuantity column. Slice 1 uses exactly one row per
// variant at DEFAULT_LOCATION_ID; slice 2+ extends to multi-warehouse.
//
// The composite primary key (variant_id, location_id) is declared
// explicitly so GORM does NOT try to add a RETURNING id clause on
// INSERT. Without the primaryKey tags on both fields, GORM would look
// for a single-column PK and fail.
type VariantStock struct {
	VariantID  string    `gorm:"primaryKey;column:variant_id;type:uuid"     json:"variant_id"`
	LocationID string    `gorm:"primaryKey;column:location_id;type:uuid"    json:"location_id"`
	Quantity   int       `gorm:"column:quantity;not null;default:0"          json:"quantity"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;default:now()"    json:"updated_at"`
}

func (VariantStock) TableName() string { return "variant_stock" }
