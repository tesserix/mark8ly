package storefront

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ListCategories handles GET /storefront/stores/:storeSlug/categories.
func (h *StorefrontHandler) ListCategories(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}
	cats, err := h.categoryRepo.ListActiveByStoreID(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	out := make([]StorefrontCategoryResponse, 0, len(cats))
	for _, cat := range cats {
		out = append(out, StorefrontCategoryResponse{
			Name:     cat.Name,
			Slug:     cat.Slug,
			Position: cat.Position,
		})
	}
	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// ListByCategorySlug handles
// GET /storefront/stores/:storeSlug/categories/:slug/products.
func (h *StorefrontHandler) ListByCategorySlug(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	slug := c.Param("slug")
	if slug == "" {
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

	var q listPublishedQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		respondNotFound(c)
		return
	}
	q.defaults()

	aggs, err := h.productRepo.ListPublished(c.Request.Context(), product.ListPublishedQuery{
		StoreID:      store.ID,
		CategorySlug: slug,
		Page:         q.Page,
		PageSize:     q.PageSize,
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
