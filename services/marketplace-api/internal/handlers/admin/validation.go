package admin

import (
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

// CreateProductRequest is the wire body for POST admin create.
type CreateProductRequest struct {
	Handle            string                      `json:"handle"`
	Title             string                      `json:"title" binding:"required,max=300"`
	Description       *string                     `json:"description,omitempty"`
	Status            string                      `json:"status" binding:"omitempty,oneof=draft active archived"`
	Tags              []string                    `json:"tags"`
	SEOTitle          *string                     `json:"seo_title,omitempty"`
	SEODescription    *string                     `json:"seo_description,omitempty"`
	PrimaryCategoryID *string                     `json:"primary_category_id,omitempty"`
	Options           []CreateProductOptionInput  `json:"options"`
	Variants          []CreateProductVariantInput `json:"variants" binding:"required,min=1"`
	Media             []CreateProductMediaInput   `json:"media"`
	CategoryIDs       []string                    `json:"category_ids"`
}

type CreateProductOptionInput struct {
	Name   string   `json:"name" binding:"required,max=100"`
	Values []string `json:"values" binding:"required,min=1"`
}

type CreateProductVariantInput struct {
	SKU               string                        `json:"sku" binding:"required,max=100"`
	Barcode           *string                       `json:"barcode,omitempty"`
	Price             decimal.Decimal               `json:"price" binding:"required"`
	CompareAtPrice    *decimal.Decimal              `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal              `json:"cost_price,omitempty"`
	CurrencyCode      string                        `json:"currency_code"`
	WeightGrams       *int                          `json:"weight_grams,omitempty"`
	InventoryQuantity int                           `json:"inventory_quantity"`
	InventoryPolicy   string                        `json:"inventory_policy" binding:"omitempty,oneof=deny continue"`
	LowStockThreshold *int                          `json:"low_stock_threshold,omitempty"`
	OptionValues      []CreateVariantOptionRefInput `json:"option_values"`
	Position          int                           `json:"position"`
}

type CreateVariantOptionRefInput struct {
	OptionName string `json:"option_name" binding:"required"`
	Value      string `json:"value" binding:"required"`
}

type CreateProductMediaInput struct {
	StorageKey string  `json:"storage_key" binding:"required"`
	URL        string  `json:"url" binding:"required"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type" binding:"omitempty,oneof=image video"`
}

// UpdateProductRequest is the wire body for PATCH admin update. Fields
// are pointer-typed so the handler can distinguish "unset" from "zero".
// Note (M5a adaptation): M3 splits update into UpdateBasicsRequest and
// UpdateVariantsRequest. Task 5 will fan this wire struct out into the
// right service calls; the helper here only maps the basics fields.
type UpdateProductRequest struct {
	Handle            *string                      `json:"handle,omitempty"`
	Title             *string                      `json:"title,omitempty"`
	Description       *string                      `json:"description,omitempty"`
	Status            *string                      `json:"status,omitempty" binding:"omitempty,oneof=draft active archived"`
	Tags              *[]string                    `json:"tags,omitempty"`
	SEOTitle          *string                      `json:"seo_title,omitempty"`
	SEODescription    *string                      `json:"seo_description,omitempty"`
	PrimaryCategoryID *string                      `json:"primary_category_id,omitempty"`
	Options           *[]CreateProductOptionInput  `json:"options,omitempty"`
	Variants          *[]CreateProductVariantInput `json:"variants,omitempty"`
	Media             *[]CreateProductMediaInput   `json:"media,omitempty"`
	CategoryIDs       *[]string                    `json:"category_ids,omitempty"`
}

// CopyProductRequest is the wire body for POST admin copy.
type CopyProductRequest struct {
	TargetStoreID string `json:"target_store_id" binding:"required,uuid"`
}

// ListProductsQuery is the query-string binding for admin list.
type ListProductsQuery struct {
	Status   string `form:"status" binding:"omitempty,oneof=draft active archived"`
	Search   string `form:"search"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// Defaults sets Page=1, PageSize=20 when unset.
func (q *ListProductsQuery) Defaults() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
}

// toServiceCreateRequest maps the wire CreateProductRequest to the M3
// service-layer product.CreateRequest. Adaptations from plan:
//   - service Options use []OptionSpec / []OptionValueSpec (not OptionInput)
//   - service VariantInput uses InitialStock (not InventoryQuantity) and
//     []OptionValueRef (not VariantOptionRef)
//   - service Description is a plain string (not *string)
func toServiceCreateRequest(req CreateProductRequest, tenantID, storeID, createdBy string) product.CreateRequest {
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	out := product.CreateRequest{
		TenantID:          tenantID,
		StoreID:           storeID,
		Handle:            req.Handle,
		Title:             req.Title,
		Description:       desc,
		Status:            req.Status,
		Tags:              req.Tags,
		SEOTitle:          req.SEOTitle,
		SEODescription:    req.SEODescription,
		PrimaryCategoryID: req.PrimaryCategoryID,
		CategoryIDs:       req.CategoryIDs,
	}
	if createdBy != "" {
		out.CreatedBy = &createdBy
	}
	for _, o := range req.Options {
		values := make([]product.OptionValueSpec, 0, len(o.Values))
		for _, v := range o.Values {
			values = append(values, product.OptionValueSpec{Value: v})
		}
		out.Options = append(out.Options, product.OptionSpec{Name: o.Name, Values: values})
	}
	for _, v := range req.Variants {
		ovs := make([]product.OptionValueRef, 0, len(v.OptionValues))
		for _, ref := range v.OptionValues {
			ovs = append(ovs, product.OptionValueRef{OptionName: ref.OptionName, Value: ref.Value})
		}
		out.Variants = append(out.Variants, product.VariantInput{
			SKU: v.SKU, Barcode: v.Barcode,
			Price: v.Price, CompareAtPrice: v.CompareAtPrice, CostPrice: v.CostPrice,
			CurrencyCode: v.CurrencyCode, WeightGrams: v.WeightGrams,
			InitialStock:      v.InventoryQuantity,
			InventoryPolicy:   v.InventoryPolicy,
			LowStockThreshold: v.LowStockThreshold,
			OptionValues:      ovs,
			Position:          v.Position,
		})
	}
	for _, m := range req.Media {
		mt := m.MediaType
		if mt == "" {
			mt = "image"
		}
		out.Media = append(out.Media, product.MediaInput{
			StorageKey: m.StorageKey, URL: m.URL, Alt: m.Alt,
			Position: m.Position, MediaType: mt,
		})
	}
	return out
}

// toServiceUpdateBasicsRequest maps the patch wire DTO to the M3
// UpdateBasicsRequest (scalar fields only). Variants/media/options are
// routed through dedicated service methods in Task 5.
func toServiceUpdateBasicsRequest(req UpdateProductRequest, id, tenantID, storeID, updatedBy string) product.UpdateBasicsRequest {
	out := product.UpdateBasicsRequest{
		ID:                id,
		StoreID:           storeID,
		TenantID:          tenantID,
		Title:             req.Title,
		Handle:            req.Handle,
		Description:       req.Description,
		Status:            req.Status,
		Tags:              req.Tags,
		SEOTitle:          req.SEOTitle,
		SEODescription:    req.SEODescription,
		PrimaryCategoryID: req.PrimaryCategoryID,
	}
	if updatedBy != "" {
		out.UpdatedBy = &updatedBy
	}
	return out
}

// toServiceCopyRequest maps the wire copy body + caller context to the
// M3 product.CopyRequest.
func toServiceCopyRequest(req CopyProductRequest, sourceProductID, sourceTenantID, sourceStoreID, copiedBy string) product.CopyRequest {
	out := product.CopyRequest{
		SourceProductID: sourceProductID,
		SourceTenantID:  sourceTenantID,
		SourceStoreID:   sourceStoreID,
		TargetStoreID:   req.TargetStoreID,
	}
	if copiedBy != "" {
		out.CopiedBy = &copiedBy
	}
	return out
}
