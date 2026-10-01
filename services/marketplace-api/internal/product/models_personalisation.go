// Package product — models_personalisation.go: what a buyer may supply on
// a product. An image for a figurine, a name to engrave, a message on a
// mug, a choice that changes the price.
//
// Design notes:
//   - Fields hang off the PRODUCT, not the variant (design decision 1).
//     Variants own money and stock; personalisation is orthogonal to both.
//   - These models are deliberately NOT part of Aggregate. Adding them
//     there would change the shape of every existing product response,
//     admin and storefront, for a feature nothing reads yet. The
//     storefront picks them up explicitly in its own phase.
//   - PersonalisationField.Key is the stable machine key snapshotted onto
//     the order line at checkout. Labels are merchant copy and change;
//     keys are history.
package product

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// Personalisation field kinds. These match the CHECK constraint in
// migration 000139 and are a closed set: a sixth kind is a migration and
// a checkout-validation change, not a config value.
const (
	PersonalisationKindImage    = "image"
	PersonalisationKindText     = "text"
	PersonalisationKindTextarea = "textarea"
	PersonalisationKindSelect   = "select"
	PersonalisationKindCheckbox = "checkbox"
)

// MaxPersonalisationFieldsPerProduct is the absolute ceiling, applied even
// on plans whose limit is Unlimited.
//
// It exists because every field on a product is validated on every
// checkout of that product: an unbounded field count is unbounded
// per-order work on the hot path. The per-plan limit (see
// plangate.FeaturePersonalisationFields) is the one a merchant actually
// meets; this is the one that stops Unlimited meaning unbounded.
const MaxPersonalisationFieldsPerProduct = 20

// MaxPersonalisationOptionsPerField caps the values on a select, for the
// same reason.
const MaxPersonalisationOptionsPerField = 50

// PersonalisationField is one thing the buyer fills in. The kind decides
// which of the constraint columns are meaningful; the rest are NULL and
// the database enforces that (migration 000139), so a max_length on an
// image field fails at insert rather than being silently ignored.
type PersonalisationField struct {
	ID        string  `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID  string  `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID   string  `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	ProductID string  `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	Key       string  `gorm:"column:key;type:varchar(60);not null"                     json:"key"`
	Label     string  `gorm:"column:label;type:varchar(120);not null"                  json:"label"`
	Kind      string  `gorm:"column:kind;type:varchar(20);not null"                    json:"kind"`
	Required  bool    `gorm:"column:required;not null;default:false"                   json:"required"`
	Position  int     `gorm:"column:position;not null;default:0"                       json:"position"`
	HelpText  *string `gorm:"column:help_text;type:varchar(300)"                       json:"help_text,omitempty"`

	// text, textarea
	MaxLength *int `gorm:"column:max_length"  json:"max_length,omitempty"`

	// image
	MaxImages        *int            `gorm:"column:max_images"                json:"max_images,omitempty"`
	MinPx            *int            `gorm:"column:min_px"                    json:"min_px,omitempty"`
	MockupStorageKey *string         `gorm:"column:mockup_storage_key;type:text" json:"mockup_storage_key,omitempty"`
	PrintArea        *datatypes.JSON `gorm:"column:print_area;type:jsonb"     json:"print_area,omitempty"`

	// checkbox
	PriceDelta *decimal.Decimal `gorm:"column:price_delta;type:numeric(12,2)" json:"price_delta,omitempty"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`

	Options []PersonalisationOption `gorm:"foreignKey:FieldID" json:"options,omitempty"`
}

func (PersonalisationField) TableName() string { return "product_personalisation_fields" }

// PersonalisationOption is one value of a select-kind field, and the money
// choosing it adds to the line.
//
// PriceDelta here is the only source of a select's money. Checkout reads
// it by id, scoped to the store, and never takes a delta from the request
// — the reasoning is the comment block in checkout_reprice.go.
type PersonalisationOption struct {
	ID         string          `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	FieldID    string          `gorm:"column:field_id;type:uuid;not null"                       json:"field_id"`
	Value      string          `gorm:"column:value;type:varchar(200);not null"                  json:"value"`
	Label      string          `gorm:"column:label;type:varchar(200);not null"                  json:"label"`
	PriceDelta decimal.Decimal `gorm:"column:price_delta;type:numeric(12,2);not null;default:0" json:"price_delta"`
	Position   int             `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt  time.Time       `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (PersonalisationOption) TableName() string { return "product_personalisation_options" }

// IsValidPersonalisationKind reports whether k is one of the five kinds.
func IsValidPersonalisationKind(k string) bool {
	switch k {
	case PersonalisationKindImage, PersonalisationKindText, PersonalisationKindTextarea,
		PersonalisationKindSelect, PersonalisationKindCheckbox:
		return true
	default:
		return false
	}
}

// PrintAreaRect is the rectangle a buyer's artwork occupies inside the
// merchant's mockup image, as percentages of the mockup's own dimensions.
// Percentages rather than pixels so the mockup can be re-exported at a
// different size without invalidating every field that referenced it.
type PrintAreaRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}
