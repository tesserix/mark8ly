package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SegmentHandler bundles dependencies for segment admin endpoints.
type SegmentHandler struct {
	svc    *campaign.Service
	logger *slog.Logger
}

// NewSegmentHandler constructs a SegmentHandler.
func NewSegmentHandler(svc *campaign.Service, logger *slog.Logger) *SegmentHandler {
	return &SegmentHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/segments.
func (h *SegmentHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	segments, err := h.svc.ListSegments(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToSegmentListResponse(segments)})
}

// Create handles POST /admin/stores/:storeId/segments.
func (h *SegmentHandler) Create(c *gin.Context) {
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

	var req CreateSegmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	seg := &campaign.CustomerSegment{
		TenantID:    tenantID,
		StoreID:     storeID,
		Name:        req.Name,
		Description: desc,
		Rules:       []byte(req.Rules),
	}

	if err := h.svc.CreateSegment(c.Request.Context(), seg); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": ToSegmentResponse(seg)})
}

// Delete handles DELETE /admin/stores/:storeId/segments/:id.
func (h *SegmentHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	if err := h.svc.DeleteSegment(c.Request.Context(), id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.Status(http.StatusNoContent)
}
