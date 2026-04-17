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

	"github.com/mark8ly/marketplace-api/internal/customer"
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

// customerTicketSummary is the trimmed projection for list views.
type customerTicketSummary struct {
	ID           string `json:"id"`
	TicketNumber string `json:"ticket_number"`
	Subject      string `json:"subject"`
	Status       string `json:"status"`
	Priority     string `json:"priority"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// customerTicketReply is the reply projection for the detail view.
// Merchant and platform replies are flattened to a single author_type
// from the customer's perspective — the storefront UI shouldn't leak
// internal merchant/platform distinctions to end users.
type customerTicketReply struct {
	ID         string `json:"id"`
	AuthorType string `json:"author_type"`
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// customerTicketDetail is the full detail projection with replies.
type customerTicketDetail struct {
	customerTicketSummary
	Description string                `json:"description"`
	Replies     []customerTicketReply `json:"replies"`
}

// resolveCustomer pulls the signed-in customer profile from the gin
// context (set by OptionalCustomerAuth + gated by RequireCustomerAuth).
// RequireCustomerAuth has already rejected the request if no profile is
// attached, so nil here is an unexpected middleware regression.
func resolveCustomer(c *gin.Context) *customer.CustomerProfile {
	val, ok := c.Get(CustomerProfileKey)
	if !ok {
		return nil
	}
	p, ok := val.(*customer.CustomerProfile)
	if !ok {
		return nil
	}
	return p
}

func ticketReplyView(r ticket.TicketReply) customerTicketReply {
	// Merchant and platform are both "support" from the shopper's view.
	authorType := "support"
	if r.AuthorType == ticket.AuthorTypeCustomer {
		authorType = "customer"
	}
	return customerTicketReply{
		ID:         r.ID.String(),
		AuthorType: authorType,
		AuthorName: r.AuthorName,
		Content:    r.Content,
		CreatedAt:  r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func ticketSummaryView(t ticket.Ticket) customerTicketSummary {
	return customerTicketSummary{
		ID:           t.ID.String(),
		TicketNumber: t.TicketNumber,
		Subject:      t.Subject,
		Status:       string(t.Status),
		Priority:     string(t.Priority),
		CreatedAt:    t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ListMine handles GET /storefront/stores/:storeSlug/account/tickets.
// Returns every ticket the signed-in customer has submitted for this
// store, newest first. Gated by RequireCustomerAuth.
func (h *TicketsHandler) ListMine(c *gin.Context) {
	profile := resolveCustomer(c)
	if profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
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

	tickets, err := h.svc.ListForCustomer(c.Request.Context(), storeID, profile.Email)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]customerTicketSummary, 0, len(tickets))
	for _, t := range tickets {
		out = append(out, ticketSummaryView(t))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// GetMine handles GET /storefront/stores/:storeSlug/account/tickets/:id.
func (h *TicketsHandler) GetMine(c *gin.Context) {
	profile := resolveCustomer(c)
	if profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
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
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ticket_id"})
		return
	}

	t, err := h.svc.GetForCustomer(c.Request.Context(), storeID, ticketID, profile.Email)
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) && ae.Code == apperrors.CodeNotFound {
			respondNotFound(c)
			return
		}
		respondInternal(c, h.logger, err)
		return
	}

	replies := make([]customerTicketReply, 0, len(t.Replies))
	for _, r := range t.Replies {
		replies = append(replies, ticketReplyView(r))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": customerTicketDetail{
			customerTicketSummary: ticketSummaryView(*t),
			Description:           t.Description,
			Replies:               replies,
		},
	})
}

// addMyReplyRequest is the customer-reply wire body.
type addMyReplyRequest struct {
	Content string `json:"content" binding:"required,max=5000"`
}

// AddMyReply handles POST /storefront/stores/:storeSlug/account/tickets/:id/reply.
// The ticket must belong to the signed-in customer (by submitter email)
// and must not be closed.
func (h *TicketsHandler) AddMyReply(c *gin.Context) {
	profile := resolveCustomer(c)
	if profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
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
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ticket_id"})
		return
	}

	// Ownership check first — we rely on GetForCustomer to 404 when the
	// ticket belongs to a different shopper. Without this, a malicious
	// customer could reply on another customer's ticket just because the
	// admin-side AddReply does no ownership check.
	t, err := h.svc.GetForCustomer(c.Request.Context(), storeID, ticketID, profile.Email)
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) && ae.Code == apperrors.CodeNotFound {
			respondNotFound(c)
			return
		}
		respondInternal(c, h.logger, err)
		return
	}

	var req addMyReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}
	content := strings.TrimSpace(htmlTagRe.ReplaceAllString(req.Content, ""))
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "message cannot be empty",
		})
		return
	}

	authorName := strings.TrimSpace(t.SubmittedByName)
	if authorName == "" {
		authorName = profile.Email
	}

	_, err = h.svc.AddReply(c.Request.Context(), ticket.AddReplyInput{
		StoreID:     storeID,
		TenantID:    tenantID,
		TicketID:    ticketID,
		AuthorType:  string(ticket.AuthorTypeCustomer),
		AuthorName:  authorName,
		AuthorEmail: profile.Email,
		Content:     content,
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

	// Nudge the merchant bell so customer follow-ups surface alongside
	// new tickets. Non-toggleable so the signal isn't silenced.
	replyMsg := "Customer replied on ticket " + t.TicketNumber + "."
	resourceType := "ticket"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     tenantID,
		StoreID:      storeID,
		Type:         notification.TypeSystemAlert,
		Title:        "Ticket reply",
		Message:      &replyMsg,
		ResourceType: &resourceType,
		ResourceID:   &t.ID,
	})

	// Re-fetch the full thread so the client renders the new reply
	// alongside any state transitions AddReply may have applied (e.g.
	// a resolved ticket flipping back to open).
	fresh, err := h.svc.GetForCustomer(c.Request.Context(), storeID, ticketID, profile.Email)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	replies := make([]customerTicketReply, 0, len(fresh.Replies))
	for _, r := range fresh.Replies {
		replies = append(replies, ticketReplyView(r))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": customerTicketDetail{
			customerTicketSummary: ticketSummaryView(*fresh),
			Description:           fresh.Description,
			Replies:               replies,
		},
	})
}
