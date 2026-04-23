// Package admin — products.go: HTTP handler for the admin products
// subset mounted under /api/v1/admin/stores/:storeId/products. Each
// method extracts path params + auth context, binds the wire DTO,
// invokes the product service, and renders via ToAdminProductResponse
// or RespondErr.
//
// M5a scope note: the M3 product service splits mutations across
// Create / UpdateBasics / UpdateMedia / UpdateCategoryLinks / Delete /
// Copy. There is NO top-level Update method and NO UpdateVariants /
// UpdateOptions yet. The Patch handler therefore fans the wire
// UpdateProductRequest out to whichever sub-methods are relevant for
// the populated sections. Variants/Options updates are deferred to
// M5b — see the TODO in Patch.
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ProductHandler bundles dependencies for the admin product endpoints.
type ProductHandler struct {
	svc     *product.Service
	catRepo category.Repository
	audit   *audit.Emitter // optional — nil-safe
	logger  *slog.Logger
}

// NewProductHandler constructs a ProductHandler.
func NewProductHandler(svc *product.Service, catRepo category.Repository, logger *slog.Logger) *ProductHandler {
	return &ProductHandler{svc: svc, catRepo: catRepo, logger: logger}
}

// WithAudit attaches an audit emitter so product lifecycle events are
// recorded. Nil-safe.
func (h *ProductHandler) WithAudit(e *audit.Emitter) *ProductHandler {
	h.audit = e
	return h
}

// List handles GET /admin/stores/:storeId/products.
func (h *ProductHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var q ListProductsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}
	q.Defaults()

	page, total, err := h.svc.List(c.Request.Context(), product.ListAdminQuery{
		StoreID:  storeID,
		TenantID: tenantID,
		Status:   q.Status,
		Search:   q.Search,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	resp := make([]AdminProductResponse, 0, len(page))
	for i := range page {
		agg := &page[i]
		categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
		resp = append(resp, ToAdminProductResponse(agg, categories))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": resp,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": ceilDiv(total, int64(q.PageSize)),
		},
	})
}

// Get handles GET /admin/stores/:storeId/products/:id.
func (h *ProductHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	agg, err := h.svc.Get(c.Request.Context(), id, storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusOK, ToAdminProductResponse(agg, categories))
}

// Create handles POST /admin/stores/:storeId/products.
func (h *ProductHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	svcReq := toServiceCreateRequest(req, tenantID, storeID, userID)
	agg, err := h.svc.Create(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "product.created",
		ResourceType: "product",
		ResourceID:   agg.Product.ID,
		Metadata: map[string]any{
			"handle": agg.Product.Handle,
			"title":  agg.Product.Title,
			"status": agg.Product.Status,
		},
	})
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusCreated, ToAdminProductResponse(agg, categories))
}

// Patch handles PATCH /admin/stores/:storeId/products/:id.
//
// M3 splits updates across multiple service methods. We fan the wire
// DTO out to whichever sub-methods are relevant:
//
//  1. Basics (title/description/status/tags/seo/handle/primary category)
//     -> svc.UpdateBasics
//  2. Media -> svc.UpdateMedia
//  3. CategoryIDs -> svc.UpdateCategoryLinks
//  4. Variants/Options -> TODO M5b (UpdateVariants does not yet exist on
//     the M3 service; the Patch handler silently ignores these sections
//     for now to keep M5a focused on scalar + media + category updates).
//
// After all applicable sub-calls succeed, we re-load the fresh aggregate
// and render it.
func (h *ProductHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	ctx := c.Request.Context()

	// If the request touches options, variants, or removed variant IDs,
	// route through the aggregate path which handles scalar + options +
	// variants + categories in a single transaction and enqueues one
	// product.updated event.
	if req.Options != nil || req.Variants != nil || req.RemovedVariantIDs != nil {
		aggReq := toServiceUpdateAggregateRequest(req, id, tenantID, storeID, userID)
		if _, err := h.svc.UpdateAggregate(ctx, aggReq); err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	} else if req.Title != nil || req.Handle != nil || req.Description != nil ||
		req.Status != nil || req.Tags != nil || req.SEOTitle != nil ||
		req.SEODescription != nil || req.PrimaryCategoryID != nil ||
		req.TaxCode != nil || req.TaxRateOverride != nil || req.TaxCategory != nil {
		// 1. Basics-only path — any scalar/SEO/handle/tax field present triggers this.
		basicsReq := toServiceUpdateBasicsRequest(req, id, tenantID, storeID, userID)
		if _, err := h.svc.UpdateBasics(ctx, basicsReq); err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	// 2. Media replacement.
	if req.Media != nil {
		media := make([]product.MediaInput, 0, len(*req.Media))
		for _, m := range *req.Media {
			mt := m.MediaType
			if mt == "" {
				mt = "image"
			}
			media = append(media, product.MediaInput{
				StorageKey: m.StorageKey,
				URL:        m.URL,
				Alt:        m.Alt,
				Position:   m.Position,
				MediaType:  mt,
			})
		}
		if err := h.svc.ReplaceMedia(ctx, id, storeID, tenantID, media); err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	// 3. Category link replacement. If the aggregate path already ran
	//    and the request also carried CategoryIDs, UpdateAggregate already
	//    handled them in the same tx, so skip the secondary call.
	if req.CategoryIDs != nil && req.Options == nil && req.Variants == nil && req.RemovedVariantIDs == nil {
		if err := h.svc.UpdateCategoryLinks(ctx, id, storeID, tenantID, *req.CategoryIDs); err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	agg, err := h.svc.Get(ctx, id, storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "product.updated",
		ResourceType: "product",
		ResourceID:   agg.Product.ID,
		Metadata: map[string]any{
			"handle": agg.Product.Handle,
			"title":  agg.Product.Title,
			"status": agg.Product.Status,
		},
	})
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusOK, ToAdminProductResponse(agg, categories))
}

// Delete handles DELETE /admin/stores/:storeId/products/:id.
func (h *ProductHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	if err := h.svc.Delete(c.Request.Context(), id, storeID, tenantID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "product.deleted",
		ResourceType: "product",
		ResourceID:   id,
		Severity:     audit.SeverityWarning,
	})
	c.Status(http.StatusNoContent)
}

// Copy handles POST /admin/stores/:storeId/products/:id/copy.
func (h *ProductHandler) Copy(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req CopyProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	svcReq := toServiceCopyRequest(req, id, tenantID, storeID, userID)
	agg, err := h.svc.Copy(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, agg.Product.StoreID, tenantID)
	c.JSON(http.StatusCreated, ToAdminProductResponse(agg, categories))
}

// resolveCategoryRefs hydrates the wire DTO category list with names
// and slugs via the category repo. Missing-category lookups are
// downgraded to empty slice + a warn log so a category lookup glitch
// never blocks a product response.
func (h *ProductHandler) resolveCategoryRefs(c *gin.Context, links []product.ProductCategory, storeID, tenantID string) []AdminCategoryRef {
	if len(links) == 0 {
		return []AdminCategoryRef{}
	}
	out := make([]AdminCategoryRef, 0, len(links))
	for _, link := range links {
		cat, err := h.catRepo.GetByIDForStore(c.Request.Context(), link.CategoryID, storeID, tenantID)
		if err != nil {
			if !errors.Is(err, apperrors.ErrNotFound) && h.logger != nil {
				h.logger.Warn("resolve category ref", "category_id", link.CategoryID, "err", err)
			}
			continue
		}
		out = append(out, AdminCategoryRef{ID: cat.ID, Name: cat.Name, Slug: cat.Slug})
	}
	return out
}

func ceilDiv(a int64, b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
