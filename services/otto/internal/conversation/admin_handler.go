package conversation

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mark8ly/otto/internal/auth"
	"github.com/mark8ly/otto/internal/event"
	"github.com/mark8ly/otto/internal/hub"
	"github.com/mark8ly/otto/internal/message"
)

// AdminDeps is the dependency bag for the staff-side handler.
type AdminDeps struct {
	Conversations *Repository
	Messages      *message.Repository
	Hub           *hub.Hub
	Logger        *slog.Logger
}

// AdminHandler exposes the /api/v1/admin/otto/* endpoints.
type AdminHandler struct{ d AdminDeps }

func NewAdminHandler(d AdminDeps) *AdminHandler { return &AdminHandler{d: d} }

// Register mounts routes. The caller must apply auth.StaffAuth and
// auth.StoreResolver so every route has tenant_id + store_id set.
func (h *AdminHandler) Register(r *gin.RouterGroup) {
	r.GET("/conversations", h.list)
	r.GET("/conversations/:id", h.get)
	r.GET("/conversations/:id/messages", h.listMessages)
	r.POST("/conversations/:id/accept", h.accept)
	r.POST("/conversations/:id/messages", h.postMessage)
	r.POST("/conversations/:id/close", h.close)
	r.GET("/ws", h.inboxWebsocket)
	r.GET("/conversations/:id/ws", h.conversationWebsocket)
}

func (h *AdminHandler) list(c *gin.Context) {
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)
	userID := c.GetString(auth.CtxUserID)

	p := ListInboxParams{TenantID: tenantID, StoreID: storeID}
	switch strings.ToLower(c.Query("status")) {
	case "pending":
		p.Status = StatusPending
	case "active":
		p.Status = StatusActive
	case "closed":
		p.Status = StatusClosed
	}
	switch strings.ToLower(c.Query("assignee")) {
	case "mine":
		p.AssigneeUserID = userID
	case "unassigned":
		p.OnlyUnassigned = true
	}

	items, err := h.d.Conversations.ListInbox(c.Request.Context(), p)
	if err != nil {
		h.d.Logger.Error("otto: list inbox", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	if items == nil {
		items = []Conversation{}
	}
	c.JSON(http.StatusOK, gin.H{"conversations": items})
}

func (h *AdminHandler) get(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	// Staff opening the thread clears their side of the unread counter.
	_ = h.d.Conversations.ClearUnread(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, AudienceStaff)
	c.JSON(http.StatusOK, gin.H{"conversation": conv})
}

func (h *AdminHandler) listMessages(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	msgs, err := h.d.Messages.ListByConversation(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, 500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	if msgs == nil {
		msgs = []message.Message{}
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (h *AdminHandler) accept(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	userID := c.GetString(auth.CtxUserID)
	name := c.GetString(auth.CtxUserName)
	email := c.GetString(auth.CtxUserEmail)

	updated, err := h.d.Conversations.Accept(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, Assignee{
		UserID: userID,
		Name:   name,
		Email:  email,
	})
	if err != nil {
		h.d.Logger.Error("otto: accept", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "accept_failed"})
		return
	}

	// Post a system message so both sides get a visible "Sara joined the
	// chat" in the thread — better UX than a silent state flip.
	if updated.Status == StatusActive && updated.Assignee != nil && updated.Assignee.UserID == userID {
		now := time.Now().UTC()
		sys := &message.Message{
			ID:             uuid.NewString(),
			ConversationID: updated.ID,
			TenantID:       updated.TenantID,
			StoreID:        updated.StoreID,
			SenderType:     message.SenderSystem,
			SenderName:     "Otto",
			Body:           systemJoinBody(updated.Assignee.Name, updated.Assignee.Email),
			CreatedAt:      now,
		}
		_ = h.d.Messages.Insert(c.Request.Context(), sys)
		_ = h.d.Conversations.BumpOnMessage(c.Request.Context(), updated.TenantID, updated.StoreID, updated.ID, AudienceCustomer, now)
		h.d.Hub.Broadcast(hub.RoomConversation(updated.ID), hub.Envelope{
			Type:    event.TypeMessageCreated,
			Payload: map[string]any{"message": sys},
		})
	}

	h.d.Hub.Broadcast(hub.RoomConversation(updated.ID), hub.Envelope{
		Type:    event.TypeConversationUpdated,
		Payload: map[string]any{"conversation": updated},
	})
	h.d.Hub.Broadcast(hub.RoomInbox(updated.TenantID, updated.StoreID), hub.Envelope{
		Type:    event.TypeConversationUpdated,
		Payload: map[string]any{"conversation": updated},
	})
	c.JSON(http.StatusOK, gin.H{"conversation": updated})
}

type adminPostMessageRequest struct {
	Body string `json:"body"`
}

func (h *AdminHandler) postMessage(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	if conv.Status == StatusClosed {
		c.JSON(http.StatusConflict, gin.H{"error": "thread_closed"})
		return
	}
	// Only the assignee (or an unassigned thread, for the "pick up and
	// reply" flow where accept is implicit) can post. Anything stricter
	// can be added later behind an FGA check.
	userID := c.GetString(auth.CtxUserID)
	if conv.Assignee != nil && conv.Assignee.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not_assignee"})
		return
	}

	var body adminPostMessageRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	body.Body = strings.TrimSpace(body.Body)
	if body.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty_message"})
		return
	}

	now := time.Now().UTC()
	msg := &message.Message{
		ID:             uuid.NewString(),
		ConversationID: conv.ID,
		TenantID:       conv.TenantID,
		StoreID:        conv.StoreID,
		SenderType:     message.SenderStaff,
		SenderID:       userID,
		SenderName:     displayName(c.GetString(auth.CtxUserName), c.GetString(auth.CtxUserEmail), "Support"),
		Body:           body.Body,
		CreatedAt:      now,
	}
	if err := h.d.Messages.Insert(c.Request.Context(), msg); err != nil {
		h.d.Logger.Error("otto: insert staff msg", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
		return
	}
	if err := h.d.Conversations.BumpOnMessage(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, AudienceCustomer, now); err != nil {
		h.d.Logger.Warn("otto: bump conversation", "err", err)
	}

	h.d.Hub.Broadcast(hub.RoomConversation(conv.ID), hub.Envelope{
		Type:    event.TypeMessageCreated,
		Payload: map[string]any{"message": msg},
	})
	h.d.Hub.Broadcast(hub.RoomInbox(conv.TenantID, conv.StoreID), hub.Envelope{
		Type:    event.TypeConversationUpdated,
		Payload: map[string]any{"conversation_id": conv.ID, "last_message": msg},
	})
	c.JSON(http.StatusCreated, gin.H{"message": msg})
}

func (h *AdminHandler) close(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	updated, err := h.d.Conversations.Close(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "close_failed"})
		return
	}
	h.d.Hub.Broadcast(hub.RoomConversation(conv.ID), hub.Envelope{
		Type:    event.TypeConversationClosed,
		Payload: map[string]any{"conversation": updated},
	})
	h.d.Hub.Broadcast(hub.RoomInbox(conv.TenantID, conv.StoreID), hub.Envelope{
		Type:    event.TypeConversationClosed,
		Payload: map[string]any{"conversation": updated},
	})
	c.JSON(http.StatusOK, gin.H{"conversation": updated})
}

func (h *AdminHandler) inboxWebsocket(c *gin.Context) {
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)
	conn, err := websocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := h.d.Hub.NewClient(conn, map[string]string{
		"role":      "staff",
		"user_id":   c.GetString(auth.CtxUserID),
		"tenant_id": tenantID,
		"store_id":  storeID,
	})
	h.d.Hub.Subscribe(client, hub.RoomInbox(tenantID, storeID))
	client.Run(h.d.Hub)
}

func (h *AdminHandler) conversationWebsocket(c *gin.Context) {
	conv, ok := h.loadForStaff(c)
	if !ok {
		return
	}
	conn, err := websocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := h.d.Hub.NewClient(conn, map[string]string{
		"role":            "staff",
		"user_id":         c.GetString(auth.CtxUserID),
		"conversation_id": conv.ID,
		"tenant_id":       conv.TenantID,
		"store_id":        conv.StoreID,
	})
	h.d.Hub.Subscribe(client, hub.RoomConversation(conv.ID))
	h.d.Hub.Subscribe(client, hub.RoomInbox(conv.TenantID, conv.StoreID))
	client.Run(h.d.Hub)
}

// loadForStaff loads a conversation and confirms it belongs to the staff
// caller's tenant+store. Returns false after writing an error response.
func (h *AdminHandler) loadForStaff(c *gin.Context) (*Conversation, bool) {
	id := c.Param("id")
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)
	conv, err := h.d.Conversations.GetByID(c.Request.Context(), tenantID, storeID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return nil, false
		}
		h.d.Logger.Error("otto: load conversation (admin)", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return nil, false
	}
	return conv, true
}

func systemJoinBody(name, email string) string {
	who := name
	if who == "" {
		who = email
	}
	if who == "" {
		who = "A support agent"
	}
	return who + " joined the conversation."
}

// Re-export the upgrader so we share one instance across handlers without
// circular imports.
var _ = websocket.Upgrader{}
