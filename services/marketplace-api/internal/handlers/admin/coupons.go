// Package admin — coupons.go: HTTP handler for the per-store
// coupon CRUD surface mounted at /admin/stores/:storeId/coupons.
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CouponHandler bundles dependencies for the admin coupon endpoints.
type CouponHandler struct {
	svc    *coupon.Service
	logger *slog.Logger
}

// NewCouponHandler constructs a CouponHandler.
func NewCouponHandler(svc *coupon.Service, logger *slog.Logger) *CouponHandler {
	return &CouponHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/coupons.
func (h *CouponHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	f := coupon.ListFilter{
		StoreID:  storeID,
		TenantID: tenantID,
		Status:   c.Query("status"),
		Search:   c.Query("search"),
		Page:     page,
		PerPage:  perPage,
	}

	result, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminCouponResponse, 0, len(result.Coupons))
	for i := range result.Coupons {
		out = append(out, toAdminCouponResponse(&result.Coupons[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  out,
		"total": result.Total,
		"page":  page,
	})
}

// Create handles POST /admin/stores/:storeId/coupons.
func (h *CouponHandler) Create(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	in := coupon.CreateInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		Code:         req.Code,
		Title:        req.Title,
		Description:  req.Description,
		Type:         req.Type,
		Value:        req.Value,
		CurrencyCode: req.CurrencyCode,
		MinPurchase:  req.MinPurchase,
		MaxDiscount:  req.MaxDiscount,
		UsageLimit:   req.UsageLimit,
		PerCustomer:  req.PerCustomer,
		TargetType:   req.TargetType,
		TargetIDs:    req.TargetIDs,
		Stackable:    req.Stackable,
		StartsAt:     req.StartsAt,
		EndsAt:       req.EndsAt,
	}

	created, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toAdminCouponResponse(created)})
}

// Get handles GET /admin/stores/:storeId/coupons/:id.
func (h *CouponHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	cpn, err := h.svc.Get(c.Request.Context(), storeID, couponID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Fetch usage stats (amendment FIX 3: include tenant_id).
	page, _ := strconv.Atoi(c.DefaultQuery("usage_page", "1"))
	usages, usageTotal, err := h.svc.ListUsage(c.Request.Context(), tenantID, couponID, page, 20)
	if err != nil {
		// Non-fatal: return the coupon without usage data.
		h.logger.Warn("coupon get: usage list failed", "err", err)
		c.JSON(http.StatusOK, gin.H{"data": toAdminCouponResponse(cpn)})
		return
	}

	usageOut := make([]AdminCouponUsageResponse, 0, len(usages))
	for i := range usages {
		usageOut = append(usageOut, toAdminCouponUsageResponse(&usages[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        toAdminCouponResponse(cpn),
		"usage":       usageOut,
		"usage_total": usageTotal,
	})
}

// Patch handles PATCH /admin/stores/:storeId/coupons/:id.
func (h *CouponHandler) Patch(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	var req PatchCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	in := coupon.PatchInput{
		Title:       req.Title,
		Description: req.Description,
		MinPurchase: req.MinPurchase,
		MaxDiscount: req.MaxDiscount,
		UsageLimit:  req.UsageLimit,
		PerCustomer: req.PerCustomer,
		Stackable:   req.Stackable,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		Status:      req.Status,
	}

	updated, err := h.svc.Patch(c.Request.Context(), storeID, couponID, in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toAdminCouponResponse(updated)})
}

// Delete handles DELETE /admin/stores/:storeId/coupons/:id.
// This is a soft-disable, not a hard delete.
func (h *CouponHandler) Delete(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), storeID, couponID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "coupon disabled"})
}
