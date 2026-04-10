package admin

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CampaignHandler bundles dependencies for campaign admin endpoints.
type CampaignHandler struct {
	svc    *campaign.Service
	repo   campaign.Repository
	logger *slog.Logger
}

// NewCampaignHandler constructs a CampaignHandler.
func NewCampaignHandler(svc *campaign.Service, repo campaign.Repository, logger *slog.Logger) *CampaignHandler {
	return &CampaignHandler{svc: svc, repo: repo, logger: logger}
}

// List handles GET /admin/stores/:storeId/campaigns.
func (h *CampaignHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	campaigns, total, err := h.repo.ListCampaignsByStore(c.Request.Context(), h.svc.DB(), storeID, status, page, pageSize)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": ToCampaignListResponse(campaigns),
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": ceilDiv(total, int64(pageSize)),
		},
	})
}

// Create handles POST /admin/stores/:storeId/campaigns.
func (h *CampaignHandler) Create(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid UUID"), h.logger)
		return
	}

	var req CreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	model := &campaign.Campaign{
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     req.Name,
		Type:     req.Type,
	}
	if req.Subject != "" {
		model.Subject = &req.Subject
	}
	if req.Content != "" {
		model.Content = &req.Content
	}
	if req.SegmentID != nil {
		id, err := uuid.Parse(*req.SegmentID)
		if err != nil {
			RespondErr(c, apperrors.ValidationFailed("segment_id", "invalid UUID"), h.logger)
			return
		}
		model.SegmentID = &id
	}
	if req.CouponID != nil {
		id, err := uuid.Parse(*req.CouponID)
		if err != nil {
			RespondErr(c, apperrors.ValidationFailed("coupon_id", "invalid UUID"), h.logger)
			return
		}
		model.CouponID = &id
	}

	if err := h.svc.CreateCampaign(c.Request.Context(), model); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": ToCampaignResponse(model)})
}

// Get handles GET /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	result, err := h.repo.GetCampaignByID(c.Request.Context(), h.svc.DB(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(result)})
}

// Patch handles PATCH /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Patch(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	existing, err := h.repo.GetCampaignByID(c.Request.Context(), h.svc.DB(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	var req UpdateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Apply partial updates.
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Subject != nil {
		existing.Subject = req.Subject
	}
	if req.Content != nil {
		existing.Content = req.Content
	}
	if req.SegmentID != nil {
		sid, err := uuid.Parse(*req.SegmentID)
		if err != nil {
			RespondErr(c, apperrors.ValidationFailed("segment_id", "invalid UUID"), h.logger)
			return
		}
		existing.SegmentID = &sid
	}
	if req.CouponID != nil {
		cid, err := uuid.Parse(*req.CouponID)
		if err != nil {
			RespondErr(c, apperrors.ValidationFailed("coupon_id", "invalid UUID"), h.logger)
			return
		}
		existing.CouponID = &cid
	}

	if err := h.svc.UpdateCampaign(c.Request.Context(), existing); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(existing)})
}

// Delete handles DELETE /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	if err := h.svc.DeleteCampaign(c.Request.Context(), id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.Status(http.StatusNoContent)
}

// Send handles POST /admin/stores/:storeId/campaigns/:id/send.
func (h *CampaignHandler) Send(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	result, err := h.svc.SendCampaign(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(result)})
}

// Schedule handles POST /admin/stores/:storeId/campaigns/:id/schedule.
func (h *CampaignHandler) Schedule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	var req ScheduleCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("scheduled_at", "must be RFC3339 format"), h.logger)
		return
	}

	result, err := h.svc.ScheduleCampaign(c.Request.Context(), id, scheduledAt)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(result)})
}

// Pause handles POST /admin/stores/:storeId/campaigns/:id/pause.
func (h *CampaignHandler) Pause(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	result, err := h.svc.PauseCampaign(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(result)})
}

// Resume handles POST /admin/stores/:storeId/campaigns/:id/resume.
func (h *CampaignHandler) Resume(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	result, err := h.svc.ResumeCampaign(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToCampaignResponse(result)})
}
