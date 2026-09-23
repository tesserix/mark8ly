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

// requireSegmentInStore parses :id, loads the segment, and proves it belongs
// to :storeId.
//
// StoreMiddleware proves only that :storeId belongs to the caller's tenant.
// campaign.Service.GetSegment takes a bare segment id, so without this a
// staff user could read, rewrite or delete another tenant's segment — and a
// segment is the recipient list a campaign sends to.
func (h *SegmentHandler) requireSegmentInStore(c *gin.Context) (*campaign.CustomerSegment, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return nil, false
	}

	seg, err := h.svc.GetSegment(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return nil, false
	}
	if seg.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("segment"), h.logger)
		return nil, false
	}
	return seg, true
}

// Get handles GET /admin/stores/:storeId/segments/:id.
func (h *SegmentHandler) Get(c *gin.Context) {
	seg, ok := h.requireSegmentInStore(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToSegmentResponse(seg)})
}

// Update handles PATCH /admin/stores/:storeId/segments/:id.
func (h *SegmentHandler) Update(c *gin.Context) {
	existing, ok := h.requireSegmentInStore(c)
	if !ok {
		return
	}

	var req UpdateSegmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	existing.Name = req.Name
	existing.Description = desc
	existing.Rules = []byte(req.Rules)

	if err := h.svc.UpdateSegment(c.Request.Context(), existing); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ToSegmentResponse(existing)})
}

// Delete handles DELETE /admin/stores/:storeId/segments/:id.
func (h *SegmentHandler) Delete(c *gin.Context) {
	existing, ok := h.requireSegmentInStore(c)
	if !ok {
		return
	}

	if err := h.svc.DeleteSegment(c.Request.Context(), existing.ID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.Status(http.StatusNoContent)
}
