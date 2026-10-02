// Package storefront — personalisation_dto.go: what a BUYER is told about
// the fields a product asks them to fill in (#965).
//
// This is a narrower view than the admin's. It carries what the form
// needs to render and what the shopper needs to decide, and nothing else:
//
//   - no tenant_id/store_id — the buyer is already on the store
//   - no created_at/updated_at — merchant bookkeeping
//   - no mockup_storage_key or print_area — those drive the 2D mockup
//     preview, which is #966. Exposing a storage key here would also
//     leak a bucket path into an anonymous response for no reason.
//
// price_delta IS exposed, on both a checkbox and a select's options,
// because a shopper deciding between "Matte" and "Gloss (+2.50)" cannot
// decide without it. It is advisory on the wire: checkout re-reads every
// delta from the catalog by id and never trusts the request (#967).
package storefront

import (
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

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

	// checkbox
	PriceDelta *decimal.Decimal `json:"price_delta,omitempty"`

	// select
	Options []StorefrontPersonalisationOption `json:"options,omitempty"`
}

// ToStorefrontPersonalisationFields maps the catalog rows to the buyer's
// view, dropping everything the buyer has no use for.
func ToStorefrontPersonalisationFields(
	fields []product.PersonalisationField,
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
