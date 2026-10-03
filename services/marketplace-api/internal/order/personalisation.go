// Package order — personalisation.go: what the buyer supplied, as the
// merchant needs to read it back (#968).
//
// With image-to-3D deferred, downloading the buyer's photograph and
// reading their text IS the figurine feature. An order whose artwork the
// merchant cannot retrieve is worse than no feature at all, because the
// money has already been taken.
package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ItemPersonalisation is one answer, as the order recorded it.
//
// Every field is a snapshot taken at checkout. There is no FK back to the
// catalog, so this survives the merchant renaming or deleting the field
// it came from — see migration 000141.
type ItemPersonalisation struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderItemID uuid.UUID `gorm:"column:order_item_id;type:uuid;not null"`
	FieldKey    string    `gorm:"column:field_key;type:varchar(60);not null"`
	FieldLabel  string    `gorm:"column:field_label;type:varchar(120);not null"`
	Kind        string    `gorm:"column:kind;type:varchar(20);not null"`
	TextValue   *string   `gorm:"column:text_value;type:text"`

	PriceDelta decimal.Decimal `gorm:"column:price_delta;type:numeric(12,2)"`

	// StorageKeyOriginal is the PRISTINE upload — the only thing a print
	// pipeline may read. StorageKey is a browser-cropped preview and is
	// lossy by construction; conflating them is how a figurine ships with
	// a blurry face.
	StorageKeyOriginal *string         `gorm:"column:storage_key_original;type:text"`
	StorageKey         *string         `gorm:"column:storage_key;type:text"`
	Crop               *datatypes.JSON `gorm:"column:crop;type:jsonb"`
	ContentType        *string         `gorm:"column:content_type;type:varchar(100)"`
	SizeBytes          *int64          `gorm:"column:size_bytes"`
	OriginalFilename   *string         `gorm:"column:original_filename;type:varchar(300)"`

	Position int `gorm:"column:position;not null;default:0"`
}

func (ItemPersonalisation) TableName() string { return "order_item_personalisations" }

// HasArtwork reports whether this answer carries a downloadable file.
func (p ItemPersonalisation) HasArtwork() bool {
	return p.StorageKeyOriginal != nil && *p.StorageKeyOriginal != ""
}

// ListPersonalisationsForOrder returns every answer on an order, scoped
// through the order's own items so an order id from another store cannot
// read a merchant's artwork.
func ListPersonalisationsForOrder(
	ctx context.Context, db *gorm.DB, orderID uuid.UUID,
) ([]ItemPersonalisation, error) {
	var out []ItemPersonalisation
	err := db.WithContext(ctx).
		Table("order_item_personalisations AS p").
		Select("p.*").
		Joins("JOIN order_items AS i ON i.id = p.order_item_id").
		Where("i.order_id = ?", orderID).
		Order("p.order_item_id, p.position, p.field_key").
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("order: list personalisations: %w", err)
	}
	return out, nil
}

// GetPersonalisationForOrder loads one answer and proves it belongs to
// the named order AND store.
//
// Both scopes matter. The order id alone would let one merchant download
// another's artwork by guessing a personalisation id; the store scope is
// what makes the id meaningless outside its own tenant.
func GetPersonalisationForOrder(
	ctx context.Context, db *gorm.DB, storeID, orderID, personalisationID uuid.UUID,
) (*ItemPersonalisation, error) {
	var row ItemPersonalisation
	err := db.WithContext(ctx).
		Table("order_item_personalisations AS p").
		Select("p.*").
		Joins("JOIN order_items AS i ON i.id = p.order_item_id").
		Joins("JOIN orders AS o ON o.id = i.order_id").
		Where("p.id = ? AND o.id = ? AND o.store_id = ?", personalisationID, orderID, storeID).
		Take(&row).Error
	if err != nil {
		return nil, fmt.Errorf("order: personalisation not found")
	}
	return &row, nil
}
