// Package storefront — personalisation_dto.go: what a BUYER is told about
// the fields a product asks them to fill in (#965).
//
// This is a narrower view than the admin's. It carries what the form
// needs to render and what the shopper needs to decide, and nothing else:
//
//   - no tenant_id/store_id — the buyer is already on the store
//   - no created_at/updated_at — merchant bookkeeping
//   - no mockup_storage_key — the KEY stays off the wire. The mockup
//     itself is public (it is the merchant's own product artwork, in the
//     public media bucket), so #966 exposes a fully-qualified mockup_url
//     and print_area to drive the 2D preview. A bucket path is still
//     withheld: a URL is a thing to fetch, a key is a thing to reason
//     about our storage layout with.
//
// price_delta IS exposed, on both a checkbox and a select's options,
// because a shopper deciding between "Matte" and "Gloss (+2.50)" cannot
// decide without it. It is advisory on the wire: checkout re-reads every
// delta from the catalog by id and never trusts the request (#967).
package storefront

import (
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/product"
)

// decodePrintArea reads the stored rectangle.
//
// A malformed or zero-sized rectangle yields nil, which drops the mockup
// entirely: compositing the buyer's artwork into a rectangle of no width
// would render an invisible image over the merchant's mockup and look
// like the upload had failed.
func decodePrintArea(raw *datatypes.JSON) *product.PrintAreaRect {
	if raw == nil {
		return nil
	}
	var rect product.PrintAreaRect
	if err := json.Unmarshal([]byte(*raw), &rect); err != nil {
		return nil
	}
	if rect.W <= 0 || rect.H <= 0 {
		return nil
	}
	return &rect
}

// StorefrontPersonalisationOption is one value of a choice field.
type StorefrontPersonalisationOption struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	PriceDelta decimal.Decimal `json:"price_delta"`
}

// StorefrontPersonalisationField is one thing the buyer fills in.
type StorefrontPersonalisationField struct {
	ID       string  `json:"id"`
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	Required bool    `json:"required"`
	Position int     `json:"position"`
	HelpText *string `json:"help_text,omitempty"`

	// text / textarea
	MaxLength *int `json:"max_length,omitempty"`

	// image
	MaxImages *int `json:"max_images,omitempty"`
	// MinPx drives the "this may print soft" warning the buyer sees
	// BEFORE adding to cart — the cheapest refund prevention in the
	// feature, and the reason this field is on the wire at all.
	MinPx *int `json:"min_px,omitempty"`
	// MockupURL and PrintArea drive the 2D composite (#966): the
	// merchant's mockup photograph with the buyer's artwork rendered
	// inside the rectangle they marked out.
	//
	// Both or neither. A mockup with no rectangle has nowhere to put the
	// artwork, and a rectangle with no mockup has nothing to sit on, so
	// the mapper emits them as a pair or not at all — that way the
	// storefront's check is "do I have a mockup" rather than two.
	//
	// PrintArea is in PERCENTAGES of the mockup's own dimensions, which
	// is what lets the composite be plain CSS: the buyer's image is
	// absolutely positioned at x%/y%/w%/h% over the mockup. No canvas,
	// no server-side image work, and it re-exports at any size.
	MockupURL *string                `json:"mockup_url,omitempty"`
	PrintArea *product.PrintAreaRect `json:"print_area,omitempty"`

	// checkbox
	PriceDelta *decimal.Decimal `json:"price_delta,omitempty"`

	// select
	Options []StorefrontPersonalisationOption `json:"options,omitempty"`
}

// ToStorefrontPersonalisationFields maps the catalog rows to the buyer's
// view, dropping everything the buyer has no use for.
//
// mediaBaseURL turns a mockup storage key into something the browser can
// fetch. Empty disables the mockup composite rather than emitting a
// relative URL that would 404 against the storefront's own origin.
func ToStorefrontPersonalisationFields(
	fields []product.PersonalisationField,
	mediaBaseURL string,
) []StorefrontPersonalisationField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]StorefrontPersonalisationField, 0, len(fields))
	for i := range fields {
		f := &fields[i]
		item := StorefrontPersonalisationField{
			ID:         f.ID,
			Key:        f.Key,
			Label:      f.Label,
			Kind:       f.Kind,
			Required:   f.Required,
			Position:   f.Position,
			HelpText:   f.HelpText,
			MaxLength:  f.MaxLength,
			MaxImages:  f.MaxImages,
			MinPx:      f.MinPx,
			PriceDelta: f.PriceDelta,
		}
		// The mockup pair. Emitted together or not at all — see the
		// field comments. A key with no base URL is dropped rather than
		// turned into a relative path the browser would resolve against
		// the storefront's own origin and 404 on.
		if f.MockupStorageKey != nil && *f.MockupStorageKey != "" &&
			f.PrintArea != nil && mediaBaseURL != "" {
			if rect := decodePrintArea(f.PrintArea); rect != nil {
				url := strings.TrimRight(mediaBaseURL, "/") + "/" + *f.MockupStorageKey
				item.MockupURL = &url
				item.PrintArea = rect
			}
		}
		for _, o := range f.Options {
			item.Options = append(item.Options, StorefrontPersonalisationOption{
				ID:         o.ID,
				Label:      o.Label,
				PriceDelta: o.PriceDelta,
			})
		}
		out = append(out, item)
	}
	return out
}
