package vendor

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Handler is the thin HTTP layer that exposes vendor endpoints for
// internal callers (platform-api, the backfill CLI). Mounted by the
// caller on any router group — typically the "/internal" group on
// the admin engine.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the vendor routes on the given router group.
func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/tenants/:tenantID/ensure-self-vendor", h.ensureSelfVendor)
	g.GET("/tenants/:tenantID/self-vendor", h.getSelfVendor)
	g.GET("/vendors/:id", h.getByID)
}

type ensureSelfVendorRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handler) ensureSelfVendor(c *gin.Context) {
	var req ensureSelfVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	v, err := h.svc.EnsureSelfVendor(c.Request.Context(), c.Param("tenantID"), req.Name, req.Slug)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *Handler) getSelfVendor(c *gin.Context) {
	v, err := h.svc.repo.GetSelfByTenantID(c.Request.Context(), c.Param("tenantID"))
	if err != nil {
		respondError(c, err)
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "no self-vendor for this tenant",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *Handler) getByID(c *gin.Context) {
	v, err := h.svc.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "vendor not found",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

// respondError maps a typed *apperrors.Error to the right HTTP status.
// Unknown or untyped errors become 500 — the admin package's central
// RespondErr exists for the rest of the codebase; we keep a minimal
// local mapper here to avoid a reverse dependency.
func respondError(c *gin.Context, err error) {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		status := statusForCode(ae.Code)
		body := gin.H{"error": string(ae.Code), "message": ae.Message}
		if len(ae.Details) > 0 {
			body["details"] = ae.Details
		}
		c.JSON(status, body)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}

func statusForCode(code apperrors.Code) int {
	switch code {
	case apperrors.CodeValidationFailed:
		return http.StatusBadRequest
	case apperrors.CodeNotFound:
		return http.StatusNotFound
	case apperrors.CodeForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
