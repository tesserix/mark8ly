package admin

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
)

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// CreateCategoryRequest is the wire body for POST admin create category.
type CreateCategoryRequest struct {
	ParentID    *string `json:"parent_id,omitempty" binding:"omitempty,uuid"`
	Name        string  `json:"name" binding:"required,max=200"`
	Slug        string  `json:"slug" binding:"omitempty,max=200"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Position    int     `json:"position"`
	IsActive    *bool   `json:"is_active,omitempty"`
	Featured    *bool   `json:"featured,omitempty"`
}

// UpdateCategoryRequest is the wire body for PATCH admin update category.
type UpdateCategoryRequest struct {
	ParentID    *string `json:"parent_id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Position    *int    `json:"position,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	Featured    *bool   `json:"featured,omitempty"`
}

// UpdateVariantRequest is the wire body for the variant quick-PATCH.
type UpdateVariantRequest struct {
	SKU               *string          `json:"sku,omitempty" binding:"omitempty,max=100"`
	Barcode           *string          `json:"barcode,omitempty"`
	Price             *decimal.Decimal `json:"price,omitempty"`
	CompareAtPrice    *decimal.Decimal `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal `json:"cost_price,omitempty"`
	CurrencyCode      *string          `json:"currency_code,omitempty"`
	WeightGrams       *int             `json:"weight_grams,omitempty"`
	LengthCM          *decimal.Decimal `json:"length_cm,omitempty"`
	WidthCM           *decimal.Decimal `json:"width_cm,omitempty"`
	HeightCM          *decimal.Decimal `json:"height_cm,omitempty"`
	InventoryQuantity *int             `json:"inventory_quantity,omitempty"`
	// InventoryByLocation is the per-warehouse breakdown (#177 PR 5e),
	// keyed by warehouse id. Mutually exclusive with InventoryQuantity —
	// the service rejects both together rather than guessing.
	InventoryByLocation map[string]int `json:"inventory_by_location,omitempty"`
	InventoryPolicy     *string        `json:"inventory_policy,omitempty" binding:"omitempty,oneof=deny continue"`
	LowStockThreshold   *int           `json:"low_stock_threshold,omitempty"`
	Position            *int           `json:"position,omitempty"`
}

// UploadURLRequest is the wire body for POST /media/upload-url.
type UploadURLRequest struct {
	ContentHash string `json:"content_hash" binding:"required,min=16,max=128"`
	Filename    string `json:"filename" binding:"required,max=200"`
	ContentType string `json:"content_type" binding:"required,oneof=image/png image/jpeg image/webp"`
}

// UploadURLResponse is the wire response for POST /media/upload-url.
type UploadURLResponse struct {
	URL        string    `json:"url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// CreateMediaRequest is the wire body for POST /products/:id/media.
type CreateMediaRequest struct {
	StorageKey string  `json:"storage_key" binding:"required"`
	URL        string  `json:"url" binding:"required"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type" binding:"omitempty,oneof=image video"`
	VariantID  *string `json:"variant_id,omitempty" binding:"omitempty,uuid"`
}

// UpdateMediaWireRequest is the wire body for PATCH /products/:id/media/:mediaId.
type UpdateMediaWireRequest struct {
	Alt        *string `json:"alt,omitempty"`
	Position   *int    `json:"position,omitempty"`
	URL        *string `json:"url,omitempty"`
	VariantID  *string `json:"variant_id,omitempty" binding:"omitempty,uuid"`
	StorageKey *string `json:"storage_key,omitempty"`
}

// CropBox is the pixel-space crop rectangle the frontend applied to the
// pristine original image before uploading the cropped result.
//
// Width/Height use gte=0 (not gt=0) because this box is currently only sent to
// the recrop-PREPARE endpoint, which ignores it — the real crop happens in the
// browser and is committed via storage_key. The prepare call therefore sends a
// zero placeholder; requiring gt=0 here rejected that placeholder with 400 and
// made "Crop" silently do nothing.
type CropBox struct {
	X      int `json:"x" binding:"gte=0"`
	Y      int `json:"y" binding:"gte=0"`
	Width  int `json:"width" binding:"gte=0"`
	Height int `json:"height" binding:"gte=0"`
}

// RecropMediaRequest is the wire body for
// POST /products/:id/media/:mediaId/recrop. The crop_box + rotation are
// metadata the client sends so the backend can log / audit the transform;
// the actual pixel work happens in the browser. crop_box is optional — the
// prepare endpoint only needs filename + content_type.
type RecropMediaRequest struct {
	CropBox     CropBox `json:"crop_box"`
	Rotation    int     `json:"rotation"`
	Filename    string  `json:"filename" binding:"omitempty,max=200"`
	ContentType string  `json:"content_type" binding:"omitempty,oneof=image/png image/jpeg image/webp"`
}

// RecropMediaResponse is the wire response for the recrop endpoint.
// The client downloads source_original_url, applies the crop in a canvas,
// PUTs the cropped blob to upload_url, then PATCHes the media row with
// new_storage_key to commit. storage_key_original never moves.
type RecropMediaResponse struct {
	SourceOriginalURL string    `json:"source_original_url"`
	UploadURL         string    `json:"upload_url"`
	NewStorageKey     string    `json:"new_storage_key"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// toServiceCreateCategory maps the wire create body to the category
// service CreateRequest. IsActive defaults to true when not supplied;
// Featured defaults to false (merchants opt categories in explicitly).
func toServiceCreateCategory(req CreateCategoryRequest, tenantID, storeID string) category.CreateRequest {
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	featured := false
	if req.Featured != nil {
		featured = *req.Featured
	}
	return category.CreateRequest{
		StoreID:     storeID,
		TenantID:    tenantID,
		ParentID:    req.ParentID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		Position:    req.Position,
		IsActive:    active,
		Featured:    featured,
	}
}

// toServiceUpdateCategory maps the wire patch body to the category
// service UpdateRequest.
func toServiceUpdateCategory(req UpdateCategoryRequest, id, tenantID, storeID string) category.UpdateRequest {
	return category.UpdateRequest{
		ID:          id,
		StoreID:     storeID,
		TenantID:    tenantID,
		ParentID:    req.ParentID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		Position:    req.Position,
		IsActive:    req.IsActive,
		Featured:    req.Featured,
	}
}

// toServiceUpdateVariantBasics maps the wire variant patch body to the
// product service request type. The CurrencyCode field is carried
// through so the service can reject cross-currency mutations.
func toServiceUpdateVariantBasics(req UpdateVariantRequest, productID, variantID, storeID, tenantID string) product.UpdateVariantBasicsRequest {
	return product.UpdateVariantBasicsRequest{
		ProductID:           productID,
		VariantID:           variantID,
		StoreID:             storeID,
		TenantID:            tenantID,
		SKU:                 req.SKU,
		Barcode:             req.Barcode,
		Price:               req.Price,
		CompareAtPrice:      req.CompareAtPrice,
		CostPrice:           req.CostPrice,
		WeightGrams:         req.WeightGrams,
		LengthCM:            req.LengthCM,
		WidthCM:             req.WidthCM,
		HeightCM:            req.HeightCM,
		InventoryQuantity:   req.InventoryQuantity,
		InventoryByLocation: req.InventoryByLocation,
		InventoryPolicy:     req.InventoryPolicy,
		LowStockThreshold:   req.LowStockThreshold,
		Position:            req.Position,
		CurrencyCode:        req.CurrencyCode,
	}
}

// toServiceAddMedia maps the wire create body to the product service
// AddMediaRequest. MediaType defaults to "image" when not supplied.
func toServiceAddMedia(req CreateMediaRequest, productID, storeID, tenantID string) product.AddMediaRequest {
	mt := req.MediaType
	if mt == "" {
		mt = "image"
	}
	return product.AddMediaRequest{
		ProductID:  productID,
		StoreID:    storeID,
		TenantID:   tenantID,
		StorageKey: req.StorageKey,
		URL:        req.URL,
		Alt:        req.Alt,
		Position:   req.Position,
		MediaType:  mt,
		VariantID:  req.VariantID,
	}
}

// toServiceUpdateMedia maps the wire patch body to the product service
// UpdateMediaRequest.
func toServiceUpdateMedia(req UpdateMediaWireRequest, productID, mediaID, storeID, tenantID string) product.UpdateMediaRequest {
	return product.UpdateMediaRequest{
		ProductID:  productID,
		MediaID:    mediaID,
		StoreID:    storeID,
		TenantID:   tenantID,
		Alt:        req.Alt,
		Position:   req.Position,
		URL:        req.URL,
		VariantID:  req.VariantID,
		StorageKey: req.StorageKey,
	}
}

// CreateProductRequest is the wire body for POST admin create.
type CreateProductRequest struct {
	Handle            string   `json:"handle"`
	Title             string   `json:"title" binding:"required,max=300"`
	Description       *string  `json:"description,omitempty"`
	Status            string   `json:"status" binding:"omitempty,oneof=draft active archived"`
	Tags              []string `json:"tags"`
	SEOTitle          *string  `json:"seo_title,omitempty"`
	SEODescription    *string  `json:"seo_description,omitempty"`
	PrimaryCategoryID *string  `json:"primary_category_id,omitempty"`
	// Tax classification. Strategy-specific interpretation — see
	// product.Product field comments.
	TaxCode         *string                     `json:"tax_code,omitempty" binding:"omitempty,max=32"`
	TaxRateOverride *decimal.Decimal            `json:"tax_rate_override,omitempty"`
	TaxCategory     *string                     `json:"tax_category,omitempty" binding:"omitempty,oneof=standard reduced zero_rated exempt"`
	Options         []CreateProductOptionInput  `json:"options"`
	Variants        []CreateProductVariantInput `json:"variants" binding:"required,min=1"`
	Media           []CreateProductMediaInput   `json:"media"`
	CategoryIDs     []string                    `json:"category_ids"`
}

type CreateProductOptionInput struct {
	Name   string   `json:"name" binding:"required,max=100"`
	Values []string `json:"values" binding:"required,min=1"`
}

type CreateProductVariantInput struct {
	// ID preserves identity on updates so the aggregate service can match
	// an incoming variant to an existing row without relying on option
	// tuples or SKU heuristics. Empty on create.
	ID                string                        `json:"id,omitempty"`
	SKU               string                        `json:"sku" binding:"required,max=100"`
	Barcode           *string                       `json:"barcode,omitempty"`
	Price             decimal.Decimal               `json:"price" binding:"required"`
	CompareAtPrice    *decimal.Decimal              `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal              `json:"cost_price,omitempty"`
	CurrencyCode      string                        `json:"currency_code"`
	WeightGrams       *int                          `json:"weight_grams,omitempty"`
	LengthCM          *decimal.Decimal              `json:"length_cm,omitempty"`
	WidthCM           *decimal.Decimal              `json:"width_cm,omitempty"`
	HeightCM          *decimal.Decimal              `json:"height_cm,omitempty"`
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
	TaxCode           *string                      `json:"tax_code,omitempty" binding:"omitempty,max=32"`
	TaxRateOverride   *decimal.Decimal             `json:"tax_rate_override,omitempty"`
	TaxCategory       *string                      `json:"tax_category,omitempty" binding:"omitempty,oneof=standard reduced zero_rated exempt"`
	Options           *[]CreateProductOptionInput  `json:"options,omitempty"`
	Variants          *[]CreateProductVariantInput `json:"variants,omitempty"`
	Media             *[]CreateProductMediaInput   `json:"media,omitempty"`
	CategoryIDs       *[]string                    `json:"category_ids,omitempty"`
	RemovedVariantIDs *[]string                    `json:"removed_variant_ids,omitempty"`
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
		TaxCode:           req.TaxCode,
		TaxRateOverride:   req.TaxRateOverride,
		TaxCategory:       req.TaxCategory,
		CategoryIDs:       req.CategoryIDs,
	}
	// Only set CreatedBy when the value is a valid UUID. Post-GIP migration,
	// user_id may be a Firebase UID (non-UUID), which would violate the
	// `created_by uuid` column constraint.
	if createdBy != "" && isValidUUID(createdBy) {
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
			LengthCM: v.LengthCM, WidthCM: v.WidthCM, HeightCM: v.HeightCM,
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
		TaxCode:           req.TaxCode,
		TaxRateOverride:   req.TaxRateOverride,
		TaxCategory:       req.TaxCategory,
	}
	if updatedBy != "" && isValidUUID(updatedBy) {
		out.UpdatedBy = &updatedBy
	}
	return out
}

// toServiceUpdateAggregateRequest maps the wire patch body to the M7c
// aggregate service request. Handles options, variants, removed variant
// IDs, and category links in one call. Scalar fields are carried
// through so the service can apply them in the same transaction.
func toServiceUpdateAggregateRequest(req UpdateProductRequest, id, tenantID, storeID, updatedBy string) product.UpdateAggregateRequest {
	out := product.UpdateAggregateRequest{
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
		TaxCode:           req.TaxCode,
		TaxRateOverride:   req.TaxRateOverride,
		TaxCategory:       req.TaxCategory,
		CategoryIDs:       req.CategoryIDs,
		RemovedVariantIDs: req.RemovedVariantIDs,
	}
	if updatedBy != "" && isValidUUID(updatedBy) {
		out.UpdatedBy = &updatedBy
	}
	if req.Options != nil {
		opts := make([]product.OptionSpec, 0, len(*req.Options))
		for _, o := range *req.Options {
			vals := make([]product.OptionValueSpec, 0, len(o.Values))
			for _, v := range o.Values {
				vals = append(vals, product.OptionValueSpec{Value: v})
			}
			opts = append(opts, product.OptionSpec{Name: o.Name, Values: vals})
		}
		out.Options = &opts
	}
	if req.Variants != nil {
		vars := make([]product.VariantInput, 0, len(*req.Variants))
		for _, v := range *req.Variants {
			ovs := make([]product.OptionValueRef, 0, len(v.OptionValues))
			for _, ref := range v.OptionValues {
				ovs = append(ovs, product.OptionValueRef{OptionName: ref.OptionName, Value: ref.Value})
			}
			vars = append(vars, product.VariantInput{
				ID:  v.ID,
				SKU: v.SKU, Barcode: v.Barcode,
				Price: v.Price, CompareAtPrice: v.CompareAtPrice, CostPrice: v.CostPrice,
				CurrencyCode: v.CurrencyCode, WeightGrams: v.WeightGrams,
				LengthCM: v.LengthCM, WidthCM: v.WidthCM, HeightCM: v.HeightCM,
				InitialStock:      v.InventoryQuantity,
				InventoryPolicy:   v.InventoryPolicy,
				LowStockThreshold: v.LowStockThreshold,
				OptionValues:      ovs,
				Position:          v.Position,
			})
		}
		out.Variants = &vars
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
