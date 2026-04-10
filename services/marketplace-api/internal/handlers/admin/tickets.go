// Package admin — tickets.go: HTTP handler for the per-store
// support ticket endpoints (Dashboard D2).
package admin

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// htmlTagRe strips HTML tags from user-submitted content.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// TicketsHandler handles /admin/stores/:storeId/tickets endpoints.
type TicketsHandler struct {
	svc    *ticket.Service
	logger *slog.Logger
}

// NewTicketsHandler constructs a TicketsHandler.
func NewTicketsHandler(svc *ticket.Service, logger *slog.Logger) *TicketsHandler {
	return &TicketsHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/tickets.
func (h *TicketsHandler) List(c *gin.Context) {
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
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	f := ticket.ListFilter{
		StoreID:  storeID,
		TenantID: tenantID,
		Status:   c.Query("status"),
		Search:   c.Query("search"),
		Page:     page,
		PerPage:  pageSize,
	}

	result, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	counts, err := h.svc.CountByStatus(c.Request.Context(), storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]TicketResponse, 0, len(result.Tickets))
	for _, t := range result.Tickets {
		out = append(out, toTicketResponse(t))
	}

	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (result.Total + int64(pageSize) - 1) / int64(pageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       result.Total,
			"total_pages": totalPages,
		},
		"counts": counts,
	})
}

// Create handles POST /admin/stores/:storeId/tickets.
func (h *TicketsHandler) Create(c *gin.Context) {
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

	var req CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	userName := c.GetString("user_name")
	if userName == "" {
		userName = c.GetString("user_email")
	}
	userEmail := c.GetString("user_email")

	t, err := h.svc.Create(c.Request.Context(), ticket.CreateInput{
		TenantID:         tenantID,
		StoreID:          storeID,
		Subject:          strings.TrimSpace(req.Subject),
		Description:      strings.TrimSpace(req.Description),
		Priority:         req.Priority,
		SubmittedByName:  userName,
		SubmittedByEmail: userEmail,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toTicketResponse(*t))
}

// Get handles GET /admin/stores/:storeId/tickets/:id.
func (h *TicketsHandler) Get(c *gin.Context) {
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	t, err := h.svc.Get(c.Request.Context(), storeID, tenantID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toTicketResponse(*t))
}

// Reply handles POST /admin/stores/:storeId/tickets/:id/reply.
func (h *TicketsHandler) Reply(c *gin.Context) {
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
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	var req TicketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	// Sanitize content — strip HTML tags.
	sanitized := htmlTagRe.ReplaceAllString(req.Content, "")
	sanitized = strings.TrimSpace(sanitized)

	userName := c.GetString("user_name")
	if userName == "" {
		userName = c.GetString("user_email")
	}

	r, err := h.svc.AddReply(c.Request.Context(), ticket.AddReplyInput{
		StoreID:     storeID,
		TenantID:    tenantID,
		TicketID:    ticketID,
		AuthorType:  string(ticket.AuthorTypeMerchant),
		AuthorName:  userName,
		AuthorEmail: c.GetString("user_email"),
		Content:     sanitized,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toTicketReplyResponse(*r))
}

// UpdateStatus handles PATCH /admin/stores/:storeId/tickets/:id.
func (h *TicketsHandler) UpdateStatus(c *gin.Context) {
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	var req UpdateTicketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	t, err := h.svc.UpdateStatus(c.Request.Context(), storeID, tenantID, id, req.Status)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toTicketResponse(*t))
}
