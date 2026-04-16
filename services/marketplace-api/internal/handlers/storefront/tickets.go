// Package storefront — tickets.go: public POST endpoint for storefront
// visitors to open a support ticket. The handler sits behind the same
// StoreContext middleware used for every other storefront route, which
// resolves the target store from the :storeSlug path param. No customer
// auth is required: shoppers can reach out before signing in.
//
// Created tickets land in the tickets table scoped to the store, which
// is exactly what the admin Dashboard D2 list already reads — no admin-
// side changes needed for the new rows to surface.
package storefront

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// htmlTagRe strips HTML from user input. Matches the admin-side regex
// so both surfaces sanitize identically.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// TicketsHandler serves the storefront support-ticket endpoints.
type TicketsHandler struct {
	svc    *ticket.Service
	notify *notification.Service // optional — nil-safe; fires admin bell
	logger *slog.Logger
}

// NewTicketsHandler constructs a TicketsHandler.
func NewTicketsHandler(svc *ticket.Service, logger *slog.Logger) *TicketsHandler {
	return &TicketsHandler{svc: svc, logger: logger}
}

// WithNotifier attaches the notification service so a merchant-visible
// notification fires when a customer opens a ticket. Nil-safe.
func (h *TicketsHandler) WithNotifier(n *notification.Service) *TicketsHandler {
	h.notify = n
	return h
}

// createTicketRequest is the public wire body.
type createTicketRequest struct {
	Name        string `json:"name"        binding:"required,max=200"`
	Email       string `json:"email"       binding:"required,email,max=300"`
	Subject     string `json:"subject"     binding:"required,max=300"`
	Description string `json:"description" binding:"required,max=5000"`
	Priority    string `json:"priority"    binding:"omitempty,oneof=low medium high"`
}

// createTicketResponse is the storefront-safe projection: we return the
// ticket number so the visitor has a reference, but never the internal
// UUID or merchant-side fields.
type createTicketResponse struct {
	TicketNumber string `json:"ticket_number"`
	Status       string `json:"status"`
	Subject      string `json:"subject"`
}

// Create handles POST /storefront/stores/:storeSlug/support/tickets.
//
// Public endpoint — no customer auth required. The submitter supplies
// their name + email in the payload; those fields are what the admin
// inbox displays.
func (h *TicketsHandler) Create(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_store"})
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_tenant"})
		return
	}

	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	subject := strings.TrimSpace(htmlTagRe.ReplaceAllString(req.Subject, ""))
	description := strings.TrimSpace(htmlTagRe.ReplaceAllString(req.Description, ""))
	name := strings.TrimSpace(htmlTagRe.ReplaceAllString(req.Name, ""))
	email := strings.TrimSpace(strings.ToLower(req.Email))

	if subject == "" || description == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "subject and description are required",
		})
		return
	}

	t, err := h.svc.Create(c.Request.Context(), ticket.CreateInput{
		TenantID:         tenantID,
		StoreID:          storeID,
		Subject:          subject,
		Description:      description,
		Priority:         req.Priority,
		SubmittedByName:  name,
		SubmittedByEmail: email,
	})
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		}
		respondInternal(c, h.logger, err)
		return
	}

	// Fire a merchant bell notification. system_alert is non-toggleable
	// so merchants can't accidentally silence support tickets. Best-effort
	// — a notification failure must not fail the public submission.
	msg := "New support ticket " + t.TicketNumber + " from " + name + "."
	resourceType := "ticket"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     tenantID,
		StoreID:      storeID,
		Type:         notification.TypeSystemAlert,
		Title:        "New support ticket",
		Message:      &msg,
		ResourceType: &resourceType,
		ResourceID:   &t.ID,
	})

	c.JSON(http.StatusCreated, createTicketResponse{
		TicketNumber: t.TicketNumber,
		Status:       string(t.Status),
		Subject:      t.Subject,
	})
}
