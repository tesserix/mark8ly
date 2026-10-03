// Package storefront — checkout_personalisation.go: validating what the
// buyer filled in, and pricing it from the catalog (#967).
//
// # Every byte here arrives from a browser
//
// checkout_reprice.go already states the rule this file obeys: the client
// supplies the shape, the catalog supplies the money. A shopper could
// otherwise POST a 50.00 discount as a negative delta, or attach another
// shopper's photograph by id, or answer a field belonging to a different
// merchant's product.
//
// So nothing on CheckoutPersonalisationRequest is trusted except as a
// LOOKUP KEY. Deltas are read from product_personalisation_options and
// product_personalisation_fields by id, scoped to the store. Labels are
// read from the same rows. Uploads must be `verified` AND carry the same
// cart token as the request.
//
// Anything that cannot be resolved fails the checkout. There is
// deliberately no fallback to the request: a line we cannot validate is a
// line we must not sell.
package storefront

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/personalisationupload"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// CheckoutPersonalisationRequest is one answer, as the browser sends it.
type CheckoutPersonalisationRequest struct {
	FieldID  string  `json:"field_id" binding:"required"`
	UploadID *string `json:"upload_id"`
	Text     *string `json:"text"`
	OptionID *string `json:"option_id"`
	Checked  *bool   `json:"checked"`
}

// resolvedPersonalisation is one validated answer, with every value taken
// from the catalog rather than the request.
type resolvedPersonalisation struct {
	FieldKey   string
	FieldLabel string
	Kind       string
	TextValue  *string
	PriceDelta decimal.Decimal
	Position   int

	// image only
	StorageKeyOriginal *string
	StorageKey         *string
	Crop               []byte
	ContentType        *string
	SizeBytes          *int64
	OriginalFilename   *string
	UploadID           string
}

// personalisationResolver validates a line's answers and prices them.
type personalisationResolver struct{ db *gorm.DB }

// resolveLine validates every answer against the product's own fields and
// returns what the order should record, plus the surcharge for ONE unit.
//
// productID comes from the catalog lookup the caller already did, not
// from the request: otherwise a shopper could name product A's variant
// and product B's fields.
func (r personalisationResolver) resolveLine(
	ctx context.Context,
	storeID, productID, cartToken string,
	answers []CheckoutPersonalisationRequest,
) ([]resolvedPersonalisation, decimal.Decimal, error) {
	surcharge := decimal.Zero

	var fields []product.PersonalisationField
	err := r.db.WithContext(ctx).
		Preload("Options").
		Where("product_id = ? AND store_id = ?", productID, storeID).
		Order("position ASC, key ASC").
		Find(&fields).Error
	if err != nil {
		return nil, surcharge, fmt.Errorf("storefront: load personalisation fields: %w", err)
	}

	byID := make(map[string]*product.PersonalisationField, len(fields))
	for i := range fields {
		byID[fields[i].ID] = &fields[i]
	}

	// Answers per field, so required-ness and max_images can be judged on
	// the whole set rather than one at a time.
	given := make(map[string][]CheckoutPersonalisationRequest, len(answers))
	for _, a := range answers {
		f, ok := byID[a.FieldID]
		if !ok {
			// A field that is not on this product — a stale tab, another
			// merchant's id, or a forged one. All three are the same
			// answer: we cannot sell this line.
			return nil, surcharge, fmt.Errorf(
				"storefront: personalisation field %q does not belong to this product", a.FieldID)
		}
		given[f.ID] = append(given[f.ID], a)
	}

	out := make([]resolvedPersonalisation, 0, len(answers))
	for i := range fields {
		f := &fields[i]
		supplied := given[f.ID]

		if len(supplied) == 0 {
			if f.Required {
				return nil, surcharge, fmt.Errorf(
					"storefront: %q is required on this product", f.Label)
			}
			continue
		}

		switch f.Kind {
		case product.PersonalisationKindImage:
			max := 1
			if f.MaxImages != nil {
				max = *f.MaxImages
			}
			if len(supplied) > max {
				return nil, surcharge, fmt.Errorf(
					"storefront: %q takes at most %d image(s)", f.Label, max)
			}
			for pos, a := range supplied {
				if a.UploadID == nil || *a.UploadID == "" {
					return nil, surcharge, fmt.Errorf("storefront: %q needs an image", f.Label)
				}
				up, uErr := r.loadUpload(ctx, *a.UploadID, cartToken, storeID, f.ID)
				if uErr != nil {
					return nil, surcharge, uErr
				}
				out = append(out, resolvedPersonalisation{
					FieldKey: f.Key, FieldLabel: f.Label, Kind: f.Kind,
					PriceDelta:         decimal.Zero,
					Position:           pos,
					UploadID:           up.ID,
					StorageKeyOriginal: &up.StorageKeyOriginal,
					StorageKey:         up.StorageKey,
					ContentType:        &up.ContentType,
					SizeBytes:          &up.SizeBytes,
					OriginalFilename:   &up.OriginalFilename,
					Crop:               cropBytes(up),
				})
			}

		case product.PersonalisationKindText, product.PersonalisationKindTextarea:
			a := supplied[0]
			if a.Text == nil {
				return nil, surcharge, fmt.Errorf("storefront: %q needs a value", f.Label)
			}
			value := strings.TrimSpace(*a.Text)
			if value == "" && f.Required {
				return nil, surcharge, fmt.Errorf("storefront: %q is required", f.Label)
			}
			if f.MaxLength != nil && len([]rune(value)) > *f.MaxLength {
				return nil, surcharge, fmt.Errorf(
					"storefront: %q must be %d characters or fewer", f.Label, *f.MaxLength)
			}
			if value == "" {
				continue
			}
			v := value
			out = append(out, resolvedPersonalisation{
				FieldKey: f.Key, FieldLabel: f.Label, Kind: f.Kind,
				TextValue: &v, PriceDelta: decimal.Zero,
			})

		case product.PersonalisationKindSelect:
			a := supplied[0]
			if a.OptionID == nil || *a.OptionID == "" {
				return nil, surcharge, fmt.Errorf("storefront: %q needs a choice", f.Label)
			}
			var picked *product.PersonalisationOption
			for j := range f.Options {
				if f.Options[j].ID == *a.OptionID {
					picked = &f.Options[j]
					break
				}
			}
			if picked == nil {
				// The option id is not one of THIS field's — a stale form
				// or a forged one. Never fall back to a default.
				return nil, surcharge, fmt.Errorf(
					"storefront: that choice is not available for %q", f.Label)
			}
			// The LABEL is recorded, not the value: it is what the buyer
			// read and what the merchant has to produce.
			label := picked.Label
			out = append(out, resolvedPersonalisation{
				FieldKey: f.Key, FieldLabel: f.Label, Kind: f.Kind,
				TextValue: &label,
				// From the catalog row, never from the request.
				PriceDelta: picked.PriceDelta,
			})
			surcharge = surcharge.Add(picked.PriceDelta)

		case product.PersonalisationKindCheckbox:
			a := supplied[0]
			if a.Checked == nil || !*a.Checked {
				if f.Required {
					return nil, surcharge, fmt.Errorf("storefront: %q must be accepted", f.Label)
				}
				continue
			}
			delta := decimal.Zero
			if f.PriceDelta != nil {
				delta = *f.PriceDelta
			}
			out = append(out, resolvedPersonalisation{
				FieldKey: f.Key, FieldLabel: f.Label, Kind: f.Kind,
				PriceDelta: delta,
			})
			surcharge = surcharge.Add(delta)
		}
	}

	return out, surcharge, nil
}

// loadUpload fetches an upload and proves the caller may use it.
//
// THE CART TOKEN CHECK IS THE POINT. An upload id is a bare uuid and the
// storefront has no authenticated customer, so without it one shopper
// could attach another's photograph to their own order by guessing or
// replaying an id.
//
// It must also be `verified` — a `pending` row means the object may not
// exist — and belong to the field being answered, so a photo uploaded for
// one field cannot be reattached to another.
func (r personalisationResolver) loadUpload(
	ctx context.Context, uploadID, cartToken, storeID, fieldID string,
) (*personalisationupload.Upload, error) {
	var up personalisationupload.Upload
	err := r.db.WithContext(ctx).
		Where("id = ? AND cart_token = ? AND store_id = ? AND field_id = ? AND state = ?",
			uploadID, cartToken, storeID, fieldID, personalisationupload.StateVerified).
		Take(&up).Error
	if err != nil {
		// One message for every failure mode — wrong cart, wrong field,
		// wrong store, not yet verified, does not exist. Distinguishing
		// them would tell a prober which uploads are real.
		return nil, fmt.Errorf("storefront: that image is no longer available")
	}
	return &up, nil
}

func cropBytes(up *personalisationupload.Upload) []byte {
	if up.Crop == nil {
		return nil
	}
	return []byte(*up.Crop)
}

// applyPersonalisation validates each line's answers and folds the
// surcharge into the money repriceItems already set.
//
// Runs AFTER repriceItems, never instead of it: the variant price is the
// base, and this only ever ADDS. Returns the resolved answers per line,
// positionally, so the order writer can snapshot them.
//
// # The product id comes from the database, not the request
//
// CheckoutItemRequest.ProductID is client-supplied. Trusting it would let
// a shopper name variant A (cheap) with product B's fields (free), or
// name a product whose fields are all optional to dodge a required one.
// The product is read from the variant, store-scoped, which is the same
// row repriceItems priced from.
func applyPersonalisation(
	ctx context.Context,
	r personalisationResolver,
	storeID, cartToken string,
	items []CheckoutItemRequest,
) ([][]resolvedPersonalisation, error) {
	resolved := make([][]resolvedPersonalisation, len(items))

	for i := range items {
		it := &items[i]
		if len(it.Personalisation) == 0 {
			// Still has to be checked: a product with a REQUIRED field
			// cannot be bought by simply omitting the array.
			if it.VariantID == nil {
				continue
			}
		}
		if it.VariantID == nil || *it.VariantID == "" {
			return nil, fmt.Errorf("storefront: cart line %d has no variant_id", i)
		}

		productID, err := r.productForVariant(ctx, storeID, *it.VariantID)
		if err != nil {
			return nil, fmt.Errorf("storefront: cart line %d: %w", i, err)
		}

		answers, surcharge, err := r.resolveLine(ctx, storeID, productID, cartToken, it.Personalisation)
		if err != nil {
			return nil, fmt.Errorf("cart line %d: %w", i, err)
		}
		resolved[i] = answers

		if surcharge.IsZero() {
			continue
		}
		// Folded into unit_price so every downstream consumer — tax,
		// refunds, invoices, the carrier's declared value — is already
		// correct with no change. The per-answer delta is recorded
		// separately for explanation only.
		it.UnitPrice = it.UnitPrice.Add(surcharge)
		it.LineTotal = it.UnitPrice.Mul(decimal.NewFromInt(int64(it.Quantity)))
	}
	return resolved, nil
}

// productForVariant resolves the variant's product, store-scoped and
// respecting soft-delete, so a removed product cannot be bought by id.
func (r personalisationResolver) productForVariant(
	ctx context.Context, storeID, variantID string,
) (string, error) {
	var productID string
	err := r.db.WithContext(ctx).
		Table("product_variants AS v").
		Select("v.product_id").
		Joins("JOIN products AS p ON p.id = v.product_id AND p.deleted_at IS NULL").
		Where("v.id = ? AND v.store_id = ? AND v.deleted_at IS NULL", variantID, storeID).
		Take(&productID).Error
	if err != nil {
		return "", errCatalogItemNotFound
	}
	return productID, nil
}
