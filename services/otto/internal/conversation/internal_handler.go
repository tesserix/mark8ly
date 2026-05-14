// internal_handler.go — server-to-server endpoints for sister services
// inside the marketplace stack. The shared X-Internal-Auth secret gates
// every route (applied at the group level in cmd/server/main.go); on top
// of that, every read still requires X-Tenant-Id + X-Store-Id so a
// caller can never pull a transcript from a tenant they don't own.
//
// Today's only consumer is marketplace-api: when the storefront renders
// a customer ticket detail page, marketplace-api lazy-loads the linked
// chat transcript through GET /api/v1/internal/otto/conversations/:id/
// transcript so the customer can read what they discussed before the
// case was escalated into a ticket.

package conversation

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/otto/internal/auth"
	"github.com/mark8ly/otto/internal/message"
)

// InternalDeps is the dependency bag for the S2S handler. We keep this
// surface deliberately narrow — internal callers should only read, not
// mutate; mutations belong on the admin/storefront groups where actor
// identity is established.
type InternalDeps struct {
	Conversations *Repository
	Messages      *message.Repository
	Logger        *slog.Logger
}

// InternalHandler exposes /api/v1/internal/otto/* endpoints.
type InternalHandler struct{ d InternalDeps }

// NewInternalHandler wires the handler with its dependencies.
func NewInternalHandler(d InternalDeps) *InternalHandler {
	return &InternalHandler{d: d}
}

// Register mounts the routes onto the caller-supplied group. The caller
// MUST have already attached auth.CustomerContext (or equivalent) so
// tenant_id + store_id are available on every request and X-Internal-Auth
// has been verified.
func (h *InternalHandler) Register(r *gin.RouterGroup) {
	r.GET("/conversations/:id/transcript", h.getTranscript)
}

// transcriptConversation is the minimal conversation summary in the S2S
// response. Callers don't need the full document — case_id + status +
// timestamps are enough to render the storefront's transcript banner.
type transcriptConversation struct {
	ID        string `json:"id"`
	CaseID    string `json:"case_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

// transcriptMessage is the on-the-wire shape for one chat line. Mirrors
// the fields marketplace-api re-projects to the customer; keeping the
// shape stable here means schema churn inside otto's storage layer
// doesn't ripple downstream.
type transcriptMessage struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
}

// transcriptResponse is the wire body for GET .../transcript.
type transcriptResponse struct {
	Conversation transcriptConversation `json:"conversation"`
	Messages     []transcriptMessage    `json:"messages"`
}

// getTranscript returns the conversation + messages for a single chat,
// scoped to the tenant+store on the request. Missing conversation OR
// cross-tenant access both return 404 (no enumeration).
func (h *InternalHandler) getTranscript(c *gin.Context) {
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	conv, err := h.d.Conversations.GetByID(c.Request.Context(), tenantID, storeID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if h.d.Logger != nil {
			h.d.Logger.Warn("otto: internal transcript: get conversation", "err", err, "conv_id", id)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return
	}
	if conv == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	// 500 is the right code on a Mongo error reading messages; an empty
	// slice is a legitimate result for a brand-new escalation with no
	// customer messages yet.
	raw, err := h.d.Messages.ListByConversation(c.Request.Context(), tenantID, storeID, conv.ID, 500)
	if err != nil {
		if h.d.Logger != nil {
			h.d.Logger.Warn("otto: internal transcript: list messages", "err", err, "conv_id", id)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}

	out := transcriptResponse{
		Conversation: transcriptConversation{
			ID:        conv.ID,
			CaseID:    conv.CaseID,
			Status:    string(conv.Status),
			CreatedAt: conv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		},
		Messages: make([]transcriptMessage, 0, len(raw)),
	}
	if conv.ClosedAt != nil {
		out.Conversation.ClosedAt = conv.ClosedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	for _, m := range raw {
		out.Messages = append(out.Messages, transcriptMessage{
			ID:         m.ID,
			SenderType: string(m.SenderType),
			SenderName: m.SenderName,
			Body:       m.Body,
			CreatedAt:  m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	c.JSON(http.StatusOK, out)
}
