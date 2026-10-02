// Package personalisationupload owns buyer-supplied artwork between the
// product page and the order (#963).
//
// # Owned by a cart, not a customer
//
// Most buyers are guests, so there is no customer row to hang an upload
// off. The owner is cart_token — the same identity stock_holds uses,
// minted by the storefront BFF. Every read is scoped by it, which is what
// stops one shopper attaching another shopper's photograph by id.
//
// # The private bucket is mandatory
//
// These objects never go in the product-media bucket: it is public-read,
// and a photograph a buyer uploaded is not a product shot. With no
// private bucket configured the service refuses to issue upload URLs
// rather than falling back — see apperrors.NotImplemented at the call
// sites and config.PrivateGCSBucket for why.
package personalisationupload

import (
	"time"

	"gorm.io/datatypes"
)

// Upload states.
const (
	// StatePending — a signed PUT was issued; the object may not exist yet.
	StatePending = "pending"
	// StateVerified — the object exists and passed the size/type checks.
	StateVerified = "verified"
	// StateClaimed — an order was placed against it. Immune to the sweeper.
	StateClaimed = "claimed"
)

// TTL is how long an upload survives without being claimed.
//
// 72 hours, not the stock holds' 15 minutes. Holds protect inventory from
// other shoppers; this protects a shopper's own work from us. Someone who
// uploads a photo of their child, sleeps on the decision and comes back
// the next evening must not find an empty slot where their photo was.
const TTL = 72 * time.Hour

// Upload is one buyer-supplied object held against a cart.
type Upload struct {
	ID        string `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID  string `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID   string `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	ProductID string `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	FieldID   string `gorm:"column:field_id;type:uuid;not null"                       json:"field_id"`
	CartToken string `gorm:"column:cart_token;type:uuid;not null"                     json:"-"`

	// StorageKeyOriginal is the pristine upload — what the merchant prints
	// from, and the only thing a print pipeline may read.
	StorageKeyOriginal string `gorm:"column:storage_key_original;type:text;not null" json:"-"`
	// StorageKey is a browser-cropped, downscaled PREVIEW. Lossy by
	// construction: canvas.toBlob() re-encodes, so treating it as the
	// print source is how a figurine ships with a blurry face.
	StorageKey *string `gorm:"column:storage_key;type:text" json:"-"`
	// Crop is the rectangle in ORIGINAL pixel coordinates, so full
	// resolution can be re-derived later.
	Crop *datatypes.JSON `gorm:"column:crop;type:jsonb" json:"crop,omitempty"`

	ContentHash      string `gorm:"column:content_hash;type:varchar(64);not null"       json:"-"`
	ContentType      string `gorm:"column:content_type;type:varchar(100);not null"      json:"content_type"`
	SizeBytes        int64  `gorm:"column:size_bytes;not null"                          json:"size_bytes"`
	OriginalFilename string `gorm:"column:original_filename;type:varchar(300);not null" json:"original_filename"`

	// WidthPx/HeightPx are CLIENT-SUPPLIED and advisory. They drive only
	// the "this may print soft" warning. Nothing about money, storage or
	// access depends on them, so there is nothing to gain by lying.
	WidthPx  *int `gorm:"column:width_px"  json:"width_px,omitempty"`
	HeightPx *int `gorm:"column:height_px" json:"height_px,omitempty"`

	State     string    `gorm:"column:state;type:varchar(20);not null;default:pending" json:"state"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"                             json:"expires_at"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"               json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"               json:"updated_at"`
}

func (Upload) TableName() string { return "personalisation_uploads" }

// CropRect is the rectangle the buyer chose, in ORIGINAL pixels — not
// percentages, unlike a field's print_area. Pixels because the merchant
// re-applies it to the pristine original at full resolution, and a
// percentage would lose precision on the way.
type CropRect struct {
	X        int `json:"x"`
	Y        int `json:"y"`
	W        int `json:"w"`
	H        int `json:"h"`
	Rotation int `json:"rotation,omitempty"`
}

// AllowedContentTypes is what a buyer may upload.
//
// HEIC is DELIBERATELY absent. Every photo an iPhone picks is HEIC unless
// something converts it, and most print pipelines cannot read it — so
// accepting it would hand merchants files they cannot use. The mobile app
// converts on pick; the web storefront rejects with a message saying what
// to do, which is a worse experience than converting in the browser and
// the right trade for v1.
var AllowedContentTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// IsAllowedContentType reports whether a buyer may upload this type.
func IsAllowedContentType(ct string) bool {
	_, ok := AllowedContentTypes[ct]
	return ok
}
