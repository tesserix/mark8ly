// Package ticket — internal_handler.go
//
// POST /internal/v1/tickets/from-conversation is the slm-router escalation
// hook. When the Otto AI decides to hand a chat off to a human (low
// confidence, customer explicitly asked, tool failure, etc.) slm-router
// flips conversation.needs_human=true in Mongo and then POSTs here with
// the conversation context so a durable Ticket lands in the merchant's
// dashboard immediately.
//
// The endpoint is idempotent on conversation_id: a second call with the
// same conversation_id returns the existing ticket rather than creating
// a duplicate. slm-router has no transactional state, so retries on
// transient failure are expected.
//
// Auth: the route lives on the /internal group which is protected by
// the platform-api shared-secret middleware (same pattern as
// /internal/stores/upsert). The slm-router pod carries the secret via
// the SLM_ROUTER_INTERNAL_AUTH env var which mirrors PLATFORM_INTERNAL_AUTH.
package ticket

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InternalHandler exposes platform-to-platform routes. Distinct from
// the merchant-facing TicketsHandler so the auth/security boundary
// stays explicit.
type InternalHandler struct {
	svc    *Service
	notify TicketNotifier
}

// TicketNotifier is implemented by the marketplace-api notification
// layer. The handler calls it after a ticket lands so the customer
// gets a "we received your support request" email out-of-band — slow
// SMTP must never block the slm-router → ticket POST.
type TicketNotifier interface {
	NotifyTicketCreated(ctx context.Context, t *Ticket)
	NotifyTicketResolved(ctx context.Context, t *Ticket)
}

// NewInternalHandler wires the handler with its dependencies. notifier
// may be nil in test/local builds; the handler logs and skips emails.
func NewInternalHandler(svc *Service, notifier TicketNotifier) *InternalHandler {
	return &InternalHandler{svc: svc, notify: notifier}
}

// RegisterRoutes mounts the internal endpoints. Caller is expected to
// have already attached the platform-internal-auth middleware.
func (h *InternalHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/v1/tickets/from-conversation", h.fromConversation)
}

type fromConversationRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
	TenantID       string `json:"tenant_id"       binding:"required"`
	StoreID        string `json:"store_id"        binding:"required"`
	CustomerName   string `json:"customer_name"`
	CustomerEmail  string `json:"customer_email"  binding:"required"`
	// Subject is a one-line summary slm-router builds from the first
	// customer message (or an LLM call). Description is the full
	// conversation transcript verbatim — the merchant reads it inline.
	Subject     string `json:"subject"     binding:"required"`
	Description string `json:"description" binding:"required"`
	// EscalationReason is "low_confidence", "customer_asked",
	// "tool_failed", "keyword_match", etc. Surfaced verbatim in the
	// ticket's metadata for analytics.
	EscalationReason string `json:"escalation_reason"`
}

type fromConversationResponse struct {
	Ticket     *Ticket `json:"ticket"`
	Duplicated bool    `json:"duplicated"`
}

// fromConversation creates (or returns the existing) ticket for a
// given otto conversation. Idempotent on conversation_id.
func (h *InternalHandler) fromConversation(c *gin.Context) {
	var req fromConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": err.Error()})
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id"})
		return
	}
	storeID, err := uuid.Parse(req.StoreID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return
	}
	subject := strings.TrimSpace(req.Subject)
	if len(subject) > 300 {
		subject = subject[:300]
	}

	// Idempotency: if a ticket already exists for this conversation
	// just return it. slm-router retries on transient failure.
	existing, lookupErr := h.svc.GetByConversation(c.Request.Context(), storeID, req.ConversationID)
	if lookupErr == nil && existing != nil {
		c.JSON(http.StatusOK, fromConversationResponse{Ticket: existing, Duplicated: true})
		return
	}

	t, err := h.svc.Create(c.Request.Context(), CreateInput{
		TenantID:         tenantID,
		StoreID:          storeID,
		Subject:          subject,
		Description:      req.Description,
		Priority:         string(TicketPriorityMedium),
		SubmittedByName:  strings.TrimSpace(req.CustomerName),
		SubmittedByEmail: strings.TrimSpace(req.CustomerEmail),
		ConversationID:   req.ConversationID,
	})
	if err != nil {
		// Race: another concurrent POST may have created it.
		// Look up by conversation_id once more before failing.
		if e, lerr := h.svc.GetByConversation(c.Request.Context(), storeID, req.ConversationID); lerr == nil && e != nil {
			c.JSON(http.StatusOK, fromConversationResponse{Ticket: e, Duplicated: true})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed", "message": errOrEmpty(err)})
		return
	}

	// Fire-and-forget customer email. Notification failure must not
	// fail the escalation — the ticket already exists either way.
	if h.notify != nil {
		go h.notify.NotifyTicketCreated(context.Background(), t)
	}

	c.JSON(http.StatusCreated, fromConversationResponse{Ticket: t, Duplicated: false})
}

func errOrEmpty(err error) string {
	var msg string
	if err != nil {
		msg = err.Error()
	}
	if errors.Is(err, context.Canceled) {
		return "request cancelled"
	}
	return msg
}
