// Package admin — personalisation_dto.go: wire types for the
// personalisation-fields surface (#962).
//
// The request types are deliberately permissive about which columns they
// accept for which kind: they take everything, and the service rejects
// what does not belong to the kind with an error that names the field.
// Validating kind-compatibility in two places would mean maintaining the
// rules in two places, and the service's copy is the one the storefront
// and CSV import will also go through.
package admin

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

// PrintAreaDTO is the rectangle a buyer's artwork occupies in the
// merchant's mockup, as percentages of the mockup.
type PrintAreaDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// PersonalisationOptionDTO is one value of a select field.
type PersonalisationOptionDTO struct {
	Value      string          `json:"value"       binding:"required"`
	Label      string          `json:"label"       binding:"required"`
	PriceDelta decimal.Decimal `json:"price_delta"`
	Position   int             `json:"position"`
}

// CreatePersonalisationFieldBody is POST .../personalisation-fields.
type CreatePersonalisationFieldBody struct {
	Key      string  `json:"key"      binding:"required"`
	Label    string  `json:"label"    binding:"required"`
	Kind     string  `json:"kind"     binding:"required"`
	Required bool    `json:"required"`
	Position int     `json:"position"`
	HelpText *string `json:"help_text"`

	MaxLength        *int             `json:"max_length"`
	MaxImages        *int             `json:"max_images"`
	MinPx            *int             `json:"min_px"`
	MockupStorageKey *string          `json:"mockup_storage_key"`
	PrintArea        *PrintAreaDTO    `json:"print_area"`
	PriceDelta       *decimal.Decimal `json:"price_delta"`

	Options []PersonalisationOptionDTO `json:"options"`
}

// PatchPersonalisationFieldBody is PATCH .../personalisation-fields/:fieldId.
// Kind is absent on purpose — see UpdatePersonalisationFieldRequest.
type PatchPersonalisationFieldBody struct {
	Label    *string `json:"label"`
	Required *bool   `json:"required"`
	Position *int    `json:"position"`
	HelpText *string `json:"help_text"`

	MaxLength        *int             `json:"max_length"`
	MaxImages        *int             `json:"max_images"`
	MinPx            *int             `json:"min_px"`
	MockupStorageKey *string          `json:"mockup_storage_key"`
	PrintArea        *PrintAreaDTO    `json:"print_area"`
	PriceDelta       *decimal.Decimal `json:"price_delta"`
}

// PatchPersonalisationOptionBody is PATCH .../options/:optionId. An
// absent price_delta leaves the stored value alone; sending 0 sets it to
// zero, which is why the handler tracks presence rather than
// zero-ness.
type PatchPersonalisationOptionBody struct {
	Value      *string          `json:"value"`
	Label      *string          `json:"label"`
	PriceDelta *decimal.Decimal `json:"price_delta"`
	Position   *int             `json:"position"`
}

// PersonalisationOptionResponse is one select value on the wire.
type PersonalisationOptionResponse struct {
	ID         string          `json:"id"`
	Value      string          `json:"value"`
	Label      string          `json:"label"`
	PriceDelta decimal.Decimal `json:"price_delta"`
	Position   int             `json:"position"`
}

// PersonalisationFieldResponse is one field on the wire.
//
// Kind-irrelevant columns are omitted rather than sent as null, so the
// admin UI can drive its form off presence instead of a kind switch of
// its own.
type PersonalisationFieldResponse struct {
	ID        string  `json:"id"`
	ProductID string  `json:"product_id"`
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Kind      string  `json:"kind"`
	Required  bool    `json:"required"`
	Position  int     `json:"position"`
	HelpText  *string `json:"help_text,omitempty"`

	MaxLength        *int             `json:"max_length,omitempty"`
	MaxImages        *int             `json:"max_images,omitempty"`
	MinPx            *int             `json:"min_px,omitempty"`
	MockupStorageKey *string          `json:"mockup_storage_key,omitempty"`
	PrintArea        *PrintAreaDTO    `json:"print_area,omitempty"`
	PriceDelta       *decimal.Decimal `json:"price_delta,omitempty"`

	Options []PersonalisationOptionResponse `json:"options,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// toServicePrintArea converts the wire rect to the domain's.
func toServicePrintArea(in *PrintAreaDTO) *product.PrintAreaRect {
	if in == nil {
		return nil
	}
	return &product.PrintAreaRect{X: in.X, Y: in.Y, W: in.W, H: in.H}
}

// toServiceOptionSpecs converts the wire option list to the domain's.
func toServiceOptionSpecs(in []PersonalisationOptionDTO) []product.PersonalisationOptionSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]product.PersonalisationOptionSpec, 0, len(in))
	for _, o := range in {
		out = append(out, product.PersonalisationOptionSpec{
			Value:      o.Value,
			Label:      o.Label,
			PriceDelta: o.PriceDelta,
			Position:   o.Position,
		})
	}
	return out
}

// ToPersonalisationFieldResponse maps a domain field to the wire shape.
//
// A print_area that fails to decode is dropped rather than failing the
// response: the column is jsonb written only by encodePrintArea, so a bad
// value means someone wrote the row by hand, and a merchant should still
// be able to see and fix the rest of the field.
func ToPersonalisationFieldResponse(f *product.PersonalisationField) PersonalisationFieldResponse {
	out := PersonalisationFieldResponse{
		ID:               f.ID,
		ProductID:        f.ProductID,
		Key:              f.Key,
		Label:            f.Label,
		Kind:             f.Kind,
		Required:         f.Required,
		Position:         f.Position,
		HelpText:         f.HelpText,
		MaxLength:        f.MaxLength,
		MaxImages:        f.MaxImages,
		MinPx:            f.MinPx,
		MockupStorageKey: f.MockupStorageKey,
		PriceDelta:       f.PriceDelta,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
	}
	if f.PrintArea != nil {
		var r PrintAreaDTO
		if err := json.Unmarshal(*f.PrintArea, &r); err == nil {
			out.PrintArea = &r
		}
	}
	for _, o := range f.Options {
		out.Options = append(out.Options, PersonalisationOptionResponse{
			ID:         o.ID,
			Value:      o.Value,
			Label:      o.Label,
			PriceDelta: o.PriceDelta,
			Position:   o.Position,
		})
	}
	return out
}

// ToPersonalisationFieldResponses maps a list.
func ToPersonalisationFieldResponses(fs []product.PersonalisationField) []PersonalisationFieldResponse {
	out := make([]PersonalisationFieldResponse, 0, len(fs))
	for i := range fs {
		out = append(out, ToPersonalisationFieldResponse(&fs[i]))
	}
	return out
}
