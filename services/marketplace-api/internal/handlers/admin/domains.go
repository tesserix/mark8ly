// Package admin — domains.go: HTTP handler for custom domain management
// endpoints (Settings S2).
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// DomainsHandler handles /admin/stores/:storeId/domains endpoints.
type DomainsHandler struct {
	svc    *domain.Service
	logger *slog.Logger
}

// NewDomainsHandler constructs a DomainsHandler.
func NewDomainsHandler(svc *domain.Service, logger *slog.Logger) *DomainsHandler {
	return &DomainsHandler{svc: svc, logger: logger}
}

// DomainResponse is the wire DTO for a custom domain.
type DomainResponse struct {
	ID         string  `json:"id"`
	Domain     string  `json:"domain"`
	Status     string  `json:"status"`
	SSLStatus  string  `json:"ssl_status"`
	VerifiedAt *string `json:"verified_at,omitempty"`
	Error      *string `json:"error,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func toDomainResponse(d domain.CustomDomain) DomainResponse {
	resp := DomainResponse{
		ID:        d.ID.String(),
		Domain:    d.Domain,
		Status:    string(d.Status),
		SSLStatus: string(d.SSLStatus),
		Error:     d.ErrorMessage,
		CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if d.VerifiedAt != nil {
		t := d.VerifiedAt.Format("2006-01-02T15:04:05Z")
		resp.VerifiedAt = &t
	}
	return resp
}

// List handles GET /admin/stores/:storeId/domains.
func (h *DomainsHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	domains, err := h.svc.List(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]DomainResponse, 0, len(domains))
	for _, d := range domains {
		out = append(out, toDomainResponse(d))
	}

	c.JSON(http.StatusOK, gin.H{"domains": out})
}

// AddDomainRequest is the request body for POST /admin/stores/:storeId/domains.
type AddDomainRequest struct {
	Domain     string `json:"domain" binding:"required"`
	CFAPIToken string `json:"cf_api_token" binding:"required"`
}

// Add handles POST /admin/stores/:storeId/domains.
func (h *DomainsHandler) Add(c *gin.Context) {
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

	var req AddDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	d, err := h.svc.Add(c.Request.Context(), domain.AddInput{
		TenantID:            tenantID,
		StoreID:             storeID,
		Domain:              req.Domain,
		CFAPITokenEncrypted: req.CFAPIToken, // encryption happens at a higher layer
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toDomainResponse(*d))
}

// Remove handles DELETE /admin/stores/:storeId/domains/:id.
func (h *DomainsHandler) Remove(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.Remove(c.Request.Context(), storeID, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// Verify handles POST /admin/stores/:storeId/domains/:id/verify.
func (h *DomainsHandler) Verify(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	d, err := h.svc.Verify(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toDomainResponse(*d))
}
