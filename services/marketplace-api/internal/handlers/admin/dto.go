package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// AdminCategoryResponse is the wire DTO for a category rendered to admin
// callers. It mirrors the M3 Category model minus soft-delete internals.
type AdminCategoryResponse struct {
	ID          string    `json:"id"`
	StoreID     string    `json:"store_id"`
	ParentID    *string   `json:"parent_id,omitempty"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	ImageURL    *string   `json:"image_url,omitempty"`
	Position    int       `json:"position"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ToAdminCategoryResponse converts the domain Category to its wire DTO.
func ToAdminCategoryResponse(c *category.Category) AdminCategoryResponse {
	return AdminCategoryResponse{
		ID:          c.ID,
		StoreID:     c.StoreID,
		ParentID:    c.ParentID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		ImageURL:    c.ImageURL,
		Position:    c.Position,
		IsActive:    c.IsActive,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

// ToAdminVariantResponse converts a product.Variant to its wire DTO.
// Option value references are left empty; use ToAdminProductResponse for
// the full product aggregate view.
func ToAdminVariantResponse(v *product.Variant) AdminVariantResponse {
	return AdminVariantResponse{
		ID:                v.ID,
		SKU:               v.SKU,
		Barcode:           v.Barcode,
		Price:             v.Price,
		CompareAtPrice:    v.CompareAtPrice,
		CostPrice:         v.CostPrice,
		CurrencyCode:      v.CurrencyCode,
		WeightGrams:       v.WeightGrams,
		InventoryQuantity: v.InventoryQuantity,
		InventoryPolicy:   v.InventoryPolicy,
		LowStockThreshold: v.LowStockThreshold,
		OptionValues:      []AdminVariantOptionRef{},
		Position:          v.Position,
	}
}

// ToAdminMediaResponse converts a product.Media row to its wire DTO.
func ToAdminMediaResponse(m *product.Media) AdminMediaResponse {
	return AdminMediaResponse{
		ID:         m.ID,
		URL:        m.URL,
		StorageKey: m.StorageKey,
		Alt:        m.Alt,
		Position:   m.Position,
		MediaType:  m.MediaType,
		VariantID:  m.VariantID,
		Width:      m.Width,
		Height:     m.Height,
		Bytes:      m.Bytes,
	}
}

// AdminProductResponse is the wire DTO for a product as rendered to admin
// callers. Storefront has its own (narrower) DTO in M6.
type AdminProductResponse struct {
	ID                  string                 `json:"id"`
	StoreID             string                 `json:"store_id"`
	Handle              string                 `json:"handle"`
	Title               string                 `json:"title"`
	Description         *string                `json:"description,omitempty"`
	Status              string                 `json:"status"`
	Tags                []string               `json:"tags"`
	SEOTitle            *string                `json:"seo_title,omitempty"`
	SEODescription      *string                `json:"seo_description,omitempty"`
	PrimaryCategoryID   *string                `json:"primary_category_id,omitempty"`
	CopySourceProductID *string                `json:"copy_source_product_id,omitempty"`
	Categories          []AdminCategoryRef     `json:"categories"`
	Options             []AdminProductOption   `json:"options"`
	Variants            []AdminVariantResponse `json:"variants"`
	Media               []AdminMediaResponse   `json:"media"`
	PublishedAt         *time.Time             `json:"published_at,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

type AdminProductOption struct {
	ID       string                    `json:"id"`
	Name     string                    `json:"name"`
	Position int                       `json:"position"`
	Values   []AdminProductOptionValue `json:"values"`
}

type AdminProductOptionValue struct {
	ID       string `json:"id"`
	Value    string `json:"value"`
	Position int    `json:"position"`
}

type AdminVariantResponse struct {
	ID                string                  `json:"id"`
	SKU               string                  `json:"sku"`
	Barcode           *string                 `json:"barcode,omitempty"`
	Price             decimal.Decimal         `json:"price"`
	CompareAtPrice    *decimal.Decimal        `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal        `json:"cost_price,omitempty"`
	CurrencyCode      string                  `json:"currency_code"`
	WeightGrams       *int                    `json:"weight_grams,omitempty"`
	InventoryQuantity int                     `json:"inventory_quantity"`
	InventoryPolicy   string                  `json:"inventory_policy"`
	LowStockThreshold *int                    `json:"low_stock_threshold,omitempty"`
	OptionValues      []AdminVariantOptionRef `json:"option_values"`
	Position          int                     `json:"position"`
}

type AdminVariantOptionRef struct {
	OptionName    string `json:"option_name"`
	OptionValueID string `json:"option_value_id"`
	Value         string `json:"value"`
}

type AdminMediaResponse struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	StorageKey string  `json:"storage_key"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type"`
	VariantID  *string `json:"variant_id,omitempty"`
	Width      *int    `json:"width,omitempty"`
	Height     *int    `json:"height,omitempty"`
	Bytes      *int64  `json:"bytes,omitempty"`
}

type AdminCategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ToAdminProductResponse converts a domain Aggregate (product + nested
// rows loaded via Preload) into the wire DTO. Pure function — no DB
// access. The mapper resolves option-value names from the loaded options
// so each variant's OptionValues references include human-readable names
// without an extra DB hop.
//
// Adaptation from plan: the plan referenced an Aggregate.VariantOption
// slice that does not exist in M3; variant → option value links live on
// Variant.OptionValueLinks in the actual model. This mapper reads those
// links directly off each variant.
func ToAdminProductResponse(a *product.Aggregate, categories []AdminCategoryRef) AdminProductResponse {
	type ovInfo struct{ optionName, value string }
	lookup := make(map[string]ovInfo)
	options := make([]AdminProductOption, 0, len(a.Options))
	for _, opt := range a.Options {
		values := make([]AdminProductOptionValue, 0, len(opt.Values))
		for _, v := range opt.Values {
			values = append(values, AdminProductOptionValue{ID: v.ID, Value: v.Value, Position: v.Position})
			lookup[v.ID] = ovInfo{optionName: opt.Name, value: v.Value}
		}
		options = append(options, AdminProductOption{ID: opt.ID, Name: opt.Name, Position: opt.Position, Values: values})
	}

	variants := make([]AdminVariantResponse, 0, len(a.Variants))
	for _, v := range a.Variants {
		ovs := make([]AdminVariantOptionRef, 0, len(v.OptionValueLinks))
		for _, link := range v.OptionValueLinks {
			info := lookup[link.OptionValueID]
			ovs = append(ovs, AdminVariantOptionRef{
				OptionName:    info.optionName,
				OptionValueID: link.OptionValueID,
				Value:         info.value,
			})
		}
		variants = append(variants, AdminVariantResponse{
			ID: v.ID, SKU: v.SKU, Barcode: v.Barcode,
			Price: v.Price, CompareAtPrice: v.CompareAtPrice, CostPrice: v.CostPrice,
			CurrencyCode: v.CurrencyCode, WeightGrams: v.WeightGrams,
			InventoryQuantity: v.InventoryQuantity, InventoryPolicy: v.InventoryPolicy,
			LowStockThreshold: v.LowStockThreshold, OptionValues: ovs, Position: v.Position,
		})
	}

	media := make([]AdminMediaResponse, 0, len(a.Media))
	for _, m := range a.Media {
		media = append(media, AdminMediaResponse{
			ID: m.ID, URL: m.URL, StorageKey: m.StorageKey,
			Alt: m.Alt, Position: m.Position, MediaType: m.MediaType,
			VariantID: m.VariantID,
			Width:     m.Width, Height: m.Height, Bytes: m.Bytes,
		})
	}

	tags := []string(a.Product.Tags)
	if tags == nil {
		tags = []string{}
	}
	if categories == nil {
		categories = []AdminCategoryRef{}
	}

	return AdminProductResponse{
		ID:                  a.Product.ID,
		StoreID:             a.Product.StoreID,
		Handle:              a.Product.Handle,
		Title:               a.Product.Title,
		Description:         a.Product.Description,
		Status:              a.Product.Status,
		Tags:                tags,
		SEOTitle:            a.Product.SEOTitle,
		SEODescription:      a.Product.SEODescription,
		PrimaryCategoryID:   a.Product.PrimaryCategoryID,
		CopySourceProductID: a.Product.CopySourceProductID,
		Categories:          categories,
		Options:             options,
		Variants:            variants,
		Media:               media,
		PublishedAt:         a.Product.PublishedAt,
		CreatedAt:           a.Product.CreatedAt,
		UpdatedAt:           a.Product.UpdatedAt,
	}
}
