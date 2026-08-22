package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CampaignHandler bundles dependencies for campaign admin endpoints.
type CampaignHandler struct {
	svc    *campaign.Service
	repo   campaign.Repository
	logger *slog.Logger

	// P9 — budget gate + concurrency slot; both optional for backward compat.
	budget *campaignbudget.Service
	slots  concurrency.SlotAcquirer
}

// NewCampaignHandler constructs a CampaignHandler.
func NewCampaignHandler(svc *campaign.Service, repo campaign.Repository, logger *slog.Logger) *CampaignHandler {
	return &CampaignHandler{svc: svc, repo: repo, logger: logger}
}

// WithBudgetGate attaches the P9 budget service and concurrency slot acquirer
// to the handler. Must be called before the handler is registered on the router.
func (h *CampaignHandler) WithBudgetGate(budget *campaignbudget.Service, slots concurrency.SlotAcquirer) *CampaignHandler {
	return &CampaignHandler{
		svc:    h.svc,
		repo:   h.repo,
		logger: h.logger,
		budget: budget,
		slots:  slots,
	}
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
		TenantID:           tenantID,
		StoreID:            storeID,
		Name:               req.Name,
		Type:               req.Type,
		ShowOnStorefront:   req.ShowOnStorefront,
		StorefrontLabel:    req.StorefrontLabel,
		StorefrontPriority: req.StorefrontPriority,
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
	if req.ShowOnStorefront != nil {
		existing.ShowOnStorefront = *req.ShowOnStorefront
	}
	if req.StorefrontLabel != nil {
		existing.StorefrontLabel = req.StorefrontLabel
	}
	if req.StorefrontPriority != nil {
		existing.StorefrontPriority = *req.StorefrontPriority
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
// When P9 budget gate is wired (h.budget != nil), the handler enforces:
//  1. Max 3 concurrent sends per store via h.slots.AcquireSlot → 429
//  2. Monthly campaign email budget via h.budget.Reserve → 403
func (h *CampaignHandler) Send(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	// P9: acquire concurrency slot before doing any work.
	if h.slots != nil {
		release, slotErr := h.slots.AcquireSlot(c.Request.Context(), storeID)
		if slotErr != nil {
			if errors.Is(slotErr, concurrency.ErrTooManyConcurrentSends) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "too_many_concurrent_sends",
					"message": "You already have 3 campaign sends in flight. Try again shortly.",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		defer release()
	}

	// P9: check budget. Fetch recipient count from existing campaign data first.
	if h.budget != nil {
		cmp, fetchErr := h.repo.GetCampaignByID(c.Request.Context(), h.svc.DB(), id)
		if fetchErr != nil {
			RespondErr(c, fetchErr, h.logger)
			return
		}
		recipientCount := cmp.TotalRecipients
		if recipientCount <= 0 {
			// Segment not yet resolved — resolve now so we can check budget.
			recipientCount64, countErr := h.repo.CountRecipientsByCampaign(c.Request.Context(), h.svc.DB(), id)
			if countErr != nil {
				RespondErr(c, countErr, h.logger)
				return
			}
			recipientCount = int(recipientCount64)
		}
		if recipientCount > 0 {
			_, budgetErr := h.budget.Reserve(c.Request.Context(), storeID, recipientCount)
			switch {
			case errors.Is(budgetErr, campaignbudget.ErrBudgetExhausted):
				campaignbudget.BudgetExhaustedTotal.WithLabelValues("unknown").Inc()
				c.JSON(http.StatusForbidden, gin.H{
					"error":   "campaign_email_budget_exhausted",
					"message": "You've used your monthly campaign email allowance. Upgrade your plan for more.",
				})
				return
			case errors.Is(budgetErr, campaignbudget.ErrNoBudgetRow):
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "budget_row_missing",
					"message": "Monthly budget not initialised. Support has been notified.",
				})
				return
			case budgetErr != nil:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
				return
			}
		}
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
