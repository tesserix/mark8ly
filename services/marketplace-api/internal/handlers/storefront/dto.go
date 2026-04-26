// Package storefront owns the public, anonymous-read HTTP surface for a
// store. Its DTO family is deliberately narrower than admin's so that no
// audit, cost, or inventory-count field can leak onto the wire. The
// leak-regression test in dto_test.go is the authoritative guarantee.
package storefront

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

// StorefrontProductResponse is the public wire DTO for a product. It
// carries NO cost, NO raw inventory counts, NO audit fields, NO tenant
// identifiers.
type StorefrontProductResponse struct {
	ID             string                      `json:"id"`
	Handle         string                      `json:"handle"`
	Title          string                      `json:"title"`
	Description    *string                     `json:"description,omitempty"`
	Tags           []string                    `json:"tags"`
	SEOTitle       *string                     `json:"seo_title,omitempty"`
	SEODescription *string                     `json:"seo_description,omitempty"`
	Categories     []StorefrontCategoryRef     `json:"categories"`
	Options        []StorefrontProductOption   `json:"options"`
	Variants       []StorefrontVariantResponse `json:"variants"`
	Media          []StorefrontMediaResponse   `json:"media"`
	PriceRange     StorefrontPriceRange        `json:"price_range"`
	// Tax classification surfaced for the cart — the storefront
	// echoes these values back on checkout line items so the server
	// can apply the right per-product rate/category.
	TaxCode         *string          `json:"tax_code,omitempty"`
	TaxRateOverride *decimal.Decimal `json:"tax_rate_override,omitempty"`
	TaxCategory     *string          `json:"tax_category,omitempty"`
	PublishedAt     time.Time        `json:"published_at"`
}

// StorefrontProductOption is one option axis on a product.
type StorefrontProductOption struct {
	Name   string                         `json:"name"`
	Values []StorefrontProductOptionValue `json:"values"`
}

// StorefrontProductOptionValue is one value on an option axis.
type StorefrontProductOptionValue struct {
	Value    string `json:"value"`
	Position int    `json:"position"`
}

// StorefrontVariantResponse carries only the derived in-stock / low-stock
// booleans, never the raw counts or cost.
type StorefrontVariantResponse struct {
	ID             string                       `json:"id"`
	Price          decimal.Decimal              `json:"price"`
	CompareAtPrice *decimal.Decimal             `json:"compare_at_price,omitempty"`
	CurrencyCode   string                       `json:"currency_code"`
	InStock        bool                         `json:"in_stock"`
	LowStock       bool                         `json:"low_stock"`
	WeightGrams    *int                         `json:"weight_grams,omitempty"`
	LengthCM       *decimal.Decimal             `json:"length_cm,omitempty"`
	WidthCM        *decimal.Decimal             `json:"width_cm,omitempty"`
	HeightCM       *decimal.Decimal             `json:"height_cm,omitempty"`
	OptionValues   []StorefrontVariantOptionRef `json:"option_values"`
}

// StorefrontVariantOptionRef links a variant to an option name/value.
type StorefrontVariantOptionRef struct {
	OptionName string `json:"option_name"`
	Value      string `json:"value"`
}

// StorefrontMediaResponse carries only the public-safe fields of a
// product_media row.
type StorefrontMediaResponse struct {
	URL       string  `json:"url"`
	Alt       *string `json:"alt,omitempty"`
	Position  int     `json:"position"`
	MediaType string  `json:"media_type"`
	Width     *int    `json:"width,omitempty"`
	Height    *int    `json:"height,omitempty"`
}

// StorefrontCategoryRef is a minimal category pointer from a product.
type StorefrontCategoryRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// StorefrontCategoryResponse is the wire DTO for a category listing.
type StorefrontCategoryResponse struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
	Featured bool   `json:"featured"`
}

// StorefrontPriceRange is the min/max price across a product's variants.
type StorefrontPriceRange struct {
	Min          decimal.Decimal `json:"min"`
	Max          decimal.Decimal `json:"max"`
	CurrencyCode string          `json:"currency_code"`
}

// ToStorefrontProductResponse converts a product aggregate (+ pre-resolved
// category refs) into the public DTO. Pure function — no DB access.
//
// Derives per-variant InStock/LowStock from the raw InventoryQuantity and
// LowStockThreshold; the raw counts are NEVER placed on the DTO.
func ToStorefrontProductResponse(a *product.Aggregate, categories []StorefrontCategoryRef) StorefrontProductResponse {
	// Option value lookup for variant option refs.
	type ovInfo struct{ optionName, value string }
	lookup := make(map[string]ovInfo)
	options := make([]StorefrontProductOption, 0, len(a.Options))
	for _, opt := range a.Options {
		values := make([]StorefrontProductOptionValue, 0, len(opt.Values))
		for _, v := range opt.Values {
			values = append(values, StorefrontProductOptionValue{Value: v.Value, Position: v.Position})
			lookup[v.ID] = ovInfo{optionName: opt.Name, value: v.Value}
		}
		options = append(options, StorefrontProductOption{Name: opt.Name, Values: values})
	}

	variants := make([]StorefrontVariantResponse, 0, len(a.Variants))
	var (
		minPrice, maxPrice decimal.Decimal
		currency           string
		havePrice          bool
	)
	for _, v := range a.Variants {
		ovs := make([]StorefrontVariantOptionRef, 0, len(v.OptionValueLinks))
		for _, link := range v.OptionValueLinks {
			info := lookup[link.OptionValueID]
			ovs = append(ovs, StorefrontVariantOptionRef{
				OptionName: info.optionName,
				Value:      info.value,
			})
		}
		inStock := v.InventoryQuantity > 0
		lowStock := v.LowStockThreshold != nil && v.InventoryQuantity <= *v.LowStockThreshold
		variants = append(variants, StorefrontVariantResponse{
			ID:             v.ID,
			Price:          v.Price,
			CompareAtPrice: v.CompareAtPrice,
			CurrencyCode:   v.CurrencyCode,
			InStock:        inStock,
			LowStock:       lowStock,
			WeightGrams:    v.WeightGrams,
			LengthCM:       v.LengthCM,
			WidthCM:        v.WidthCM,
			HeightCM:       v.HeightCM,
			OptionValues:   ovs,
		})
		if !havePrice {
			minPrice = v.Price
			maxPrice = v.Price
			currency = v.CurrencyCode
			havePrice = true
		} else {
			if v.Price.LessThan(minPrice) {
				minPrice = v.Price
			}
			if v.Price.GreaterThan(maxPrice) {
				maxPrice = v.Price
			}
		}
	}

	media := make([]StorefrontMediaResponse, 0, len(a.Media))
	for _, m := range a.Media {
		media = append(media, StorefrontMediaResponse{
			URL:       m.URL,
			Alt:       m.Alt,
			Position:  m.Position,
			MediaType: m.MediaType,
			Width:     m.Width,
			Height:    m.Height,
		})
	}

	tags := []string(a.Product.Tags)
	if tags == nil {
		tags = []string{}
	}
	if categories == nil {
		categories = []StorefrontCategoryRef{}
	}

	var publishedAt time.Time
	if a.Product.PublishedAt != nil {
		publishedAt = *a.Product.PublishedAt
	}

	return StorefrontProductResponse{
		ID:             a.Product.ID,
		Handle:         a.Product.Handle,
		Title:          a.Product.Title,
		Description:    a.Product.Description,
		Tags:           tags,
		SEOTitle:       a.Product.SEOTitle,
		SEODescription: a.Product.SEODescription,
		Categories:     categories,
		Options:        options,
		Variants:       variants,
		Media:          media,
		PriceRange: StorefrontPriceRange{
			Min:          minPrice,
			Max:          maxPrice,
			CurrencyCode: currency,
		},
		TaxCode:         a.Product.TaxCode,
		TaxRateOverride: a.Product.TaxRateOverride,
		TaxCategory:     a.Product.TaxCategory,
		PublishedAt:     publishedAt,
	}
}
