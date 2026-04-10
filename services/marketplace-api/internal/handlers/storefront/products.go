package storefront

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// StorefrontHandler serves the public read routes rooted under
// /storefront/stores/:storeSlug. It owns no admin or tenant-scoped
// dependencies — every lookup is store-scoped by ID resolved from the
// path slug via the SlugCache in the middleware chain.
type StorefrontHandler struct {
	productRepo  product.Repository
	categoryRepo category.Repository
	watermarks   stores.WatermarkReader
	logger       *slog.Logger
}

// NewStorefrontHandler constructs a handler. All deps are required.
func NewStorefrontHandler(
	productRepo product.Repository,
	categoryRepo category.Repository,
	watermarks stores.WatermarkReader,
	logger *slog.Logger,
) *StorefrontHandler {
	return &StorefrontHandler{
		productRepo:  productRepo,
		categoryRepo: categoryRepo,
		watermarks:   watermarks,
		logger:       logger,
	}
}

// listPublishedQuery is the storefront pagination input. Validation
// failures translate to 404 — the anonymous path never leaks field names.
type listPublishedQuery struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Search   string `form:"search" binding:"omitempty,max=200"`
}

func (q *listPublishedQuery) defaults() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
}

// List handles GET /storefront/stores/:storeSlug/products.
func (h *StorefrontHandler) List(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}

	var q listPublishedQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		respondNotFound(c)
		return
	}
	q.defaults()

	aggs, err := h.productRepo.ListPublished(c.Request.Context(), product.ListPublishedQuery{
		StoreID:  store.ID,
		Page:     q.Page,
		PageSize: q.PageSize,
		Search:   q.Search,
	})
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	catByID, err := h.storeCategoryMap(c, store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]StorefrontProductResponse, 0, len(aggs))
	for i := range aggs {
		refs := resolveStorefrontCategoryRefs(&aggs[i], catByID)
		out = append(out, ToStorefrontProductResponse(&aggs[i], refs))
	}

	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"page": q.Page, "page_size": q.PageSize},
	})
}

// GetByHandle handles GET /storefront/stores/:storeSlug/products/:handle.
func (h *StorefrontHandler) GetByHandle(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")
	if handle == "" {
		respondNotFound(c)
		return
	}
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}
	agg, err := h.productRepo.GetPublishedByHandle(c.Request.Context(), store.ID, handle)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			respondNotFound(c)
			return
		}
		respondInternal(c, h.logger, err)
		return
	}
	catByID, err := h.storeCategoryMap(c, store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	refs := resolveStorefrontCategoryRefs(agg, catByID)
	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, ToStorefrontProductResponse(agg, refs))
}

// storeCategoryMap loads all active categories for the store into an
// id-keyed map so the mapper can hydrate product category refs without
// an extra DB hop per product.
func (h *StorefrontHandler) storeCategoryMap(c *gin.Context, storeID string) (map[string]category.Category, error) {
	cats, err := h.categoryRepo.ListActiveByStoreID(c.Request.Context(), storeID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]category.Category, len(cats))
	for _, cat := range cats {
		m[cat.ID] = cat
	}
	return m, nil
}

// resolveStorefrontCategoryRefs filters a product aggregate's category
// links through the pre-built id-keyed store category map, dropping any
// link whose category is not active / not in the store's visible tree.
func resolveStorefrontCategoryRefs(a *product.Aggregate, catByID map[string]category.Category) []StorefrontCategoryRef {
	refs := make([]StorefrontCategoryRef, 0, len(a.CategoryLinks))
	for _, link := range a.CategoryLinks {
		cat, ok := catByID[link.CategoryID]
		if !ok {
			continue
		}
		refs = append(refs, StorefrontCategoryRef{Name: cat.Name, Slug: cat.Slug})
	}
	return refs
}
