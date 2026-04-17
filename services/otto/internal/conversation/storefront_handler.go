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
	"github.com/mark8ly/otto/internal/otp"
	"github.com/mark8ly/otto/internal/session"
)

// StorefrontDeps is the dependency bag for the customer-side handler.
type StorefrontDeps struct {
	Conversations *Repository
	Messages      *message.Repository
	Hub           *hub.Hub
	Signer        *session.Signer
	OTP           *otp.Service
	CookieName    string
	CookieDomain  string
	CookieSecure  bool
	Logger        *slog.Logger
}

// StorefrontHandler exposes the /api/v1/storefront/otto/* endpoints.
type StorefrontHandler struct{ d StorefrontDeps }

func NewStorefrontHandler(d StorefrontDeps) *StorefrontHandler {
	return &StorefrontHandler{d: d}
}

// Register mounts routes onto the given group. The group MUST already have
// auth.CustomerContext applied; any route requiring an existing session also
// requires auth.RequireCustomerSession (applied per-route here so the
// create endpoint can issue the cookie on the first call).
func (h *StorefrontHandler) Register(r *gin.RouterGroup) {
	r.POST("/conversations", h.create)

	// These require a valid otto_session cookie already.
	withSession := r.Group("")
	withSession.Use(auth.RequireCustomerSession(h.d.CookieName, h.d.Signer))
	withSession.GET("/conversations/:id", h.get)
	withSession.GET("/conversations/:id/messages", h.listMessages)
	withSession.POST("/conversations/:id/messages", h.postMessage)
	withSession.POST("/conversations/:id/close", h.close)
	withSession.GET("/conversations/:id/ws", h.websocket)
}

type createRequest struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
	// OTPCode is required for anonymous callers. When the storefront
	// proxy forwards a logged-in user's identity via X-User-Id /
	// X-User-Email headers the OTP step is skipped entirely.
	OTPCode string `json:"otp_code"`
}

func (h *StorefrontHandler) create(c *gin.Context) {
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)

	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": err.Error()})
		return
	}
	body.Subject = strings.TrimSpace(body.Subject)
	body.Message = strings.TrimSpace(body.Message)
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(body.Email)

	// A new thread must have something to say — an empty opener would leave
	// staff with nothing to accept.
	if body.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty_message"})
		return
	}

	// Authorisation: either the caller is a logged-in customer (identity
	// forwarded by the storefront proxy) or they completed the OTP flow.
	//
	// Logged-in users are already email-verified by the storefront's sign-in
	// path, so we accept them without a second factor. For everyone else a
	// valid OTP prevents trivial bot/spam creation of threads.
	verifiedUserID := c.GetString(auth.CtxUserID)
	verifiedEmail := c.GetString(auth.CtxUserEmail)
	if verifiedUserID == "" {
		// Anonymous — must bring an OTP.
		if body.Email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email_required"})
			return
		}
		if h.d.OTP == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "otp_not_configured"})
			return
		}
		if err := h.d.OTP.Verify(c.Request.Context(), otp.VerifyInput{
			TenantID: tenantID,
			StoreID:  storeID,
			Email:    body.Email,
			Code:     body.OTPCode,
		}); err != nil {
			mapOTPErrorToResponse(c, err)
			return
		}
		// Anonymous caller: prefer the email they typed.
		verifiedEmail = body.Email
	} else if body.Email == "" {
		// Logged-in user — fall back to the email the proxy forwarded when
		// the client didn't echo it (typical for the auto-start path).
		body.Email = verifiedEmail
	}

	raw, tok, err := h.d.Signer.Issue(tenantID, storeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue_session"})
		return
	}

	now := time.Now().UTC()
	conv := &Conversation{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		StoreID:       storeID,
		Status:        StatusPending,
		Subject:       body.Subject,
		Customer: Customer{
			SessionToken: tok.ID,
			UserID:       verifiedUserID,
			Name:         body.Name,
			Email:        body.Email,
		},
		CreatedAt:     now,
		UpdatedAt:     now,
		LastMessageAt: now,
		MessageCount:  1,
		// The new message counts as unread for staff since no one has seen it.
		UnreadCountStaff: 1,
	}
	if err := h.d.Conversations.Insert(c.Request.Context(), conv); err != nil {
		h.d.Logger.Error("otto: create conversation", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
		return
	}

	firstMsg := &message.Message{
		ID:             uuid.NewString(),
		ConversationID: conv.ID,
		TenantID:       tenantID,
		StoreID:        storeID,
		SenderType:     message.SenderCustomer,
		SenderID:       conv.Customer.UserID,
		SenderName:     displayName(body.Name, body.Email, "Customer"),
		Body:           body.Message,
		CreatedAt:      now,
	}
	if err := h.d.Messages.Insert(c.Request.Context(), firstMsg); err != nil {
		h.d.Logger.Error("otto: insert first message", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
		return
	}

	setSessionCookie(c, h.d.CookieName, raw, h.d.CookieDomain, h.d.CookieSecure, tok.Expiry)

	// Notify any staff already watching the inbox that a new pending thread
	// has arrived. Delivery is best-effort.
	h.d.Hub.Broadcast(hub.RoomInbox(tenantID, storeID), hub.Envelope{
		Type:    event.TypeConversationCreated,
		Payload: map[string]any{"conversation": conv, "first_message": firstMsg},
	})

	c.JSON(http.StatusCreated, gin.H{
		"conversation":  conv,
		"first_message": firstMsg,
	})
}

func (h *StorefrontHandler) get(c *gin.Context) {
	conv, ok := h.loadForCustomer(c)
	if !ok {
		return
	}
	// Customer opened the thread in their UI — they've seen all messages.
	_ = h.d.Conversations.ClearUnread(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, AudienceCustomer)
	c.JSON(http.StatusOK, gin.H{"conversation": conv})
}

func (h *StorefrontHandler) listMessages(c *gin.Context) {
	conv, ok := h.loadForCustomer(c)
	if !ok {
		return
	}
	msgs, err := h.d.Messages.ListByConversation(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	if msgs == nil {
		msgs = []message.Message{}
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

type postMessageRequest struct {
	Body string `json:"body"`
}

func (h *StorefrontHandler) postMessage(c *gin.Context) {
	conv, ok := h.loadForCustomer(c)
	if !ok {
		return
	}
	if conv.Status == StatusClosed {
		c.JSON(http.StatusConflict, gin.H{"error": "thread_closed"})
		return
	}

	var body postMessageRequest
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
		SenderType:     message.SenderCustomer,
		SenderID:       conv.Customer.UserID,
		SenderName:     displayName(conv.Customer.Name, conv.Customer.Email, "Customer"),
		Body:           body.Body,
		CreatedAt:      now,
	}
	if err := h.d.Messages.Insert(c.Request.Context(), msg); err != nil {
		h.d.Logger.Error("otto: insert customer msg", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
		return
	}
	if err := h.d.Conversations.BumpOnMessage(c.Request.Context(), conv.TenantID, conv.StoreID, conv.ID, AudienceStaff, now); err != nil {
		h.d.Logger.Warn("otto: bump conversation", "err", err)
	}

	// Fan out to whoever is watching.
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

func (h *StorefrontHandler) close(c *gin.Context) {
	conv, ok := h.loadForCustomer(c)
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

// websocketUpgrader is shared — no origin checks here because the Next.js
// proxy handles origin before the request reaches us, and CORS is
// configured at the gin engine level for direct WS clients.
var websocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (h *StorefrontHandler) websocket(c *gin.Context) {
	conv, ok := h.loadForCustomer(c)
	if !ok {
		return
	}
	conn, err := websocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := h.d.Hub.NewClient(conn, map[string]string{
		"role":            "customer",
		"conversation_id": conv.ID,
		"tenant_id":       conv.TenantID,
	})
	h.d.Hub.Subscribe(client, hub.RoomConversation(conv.ID))
	client.Run(h.d.Hub)
}

// loadForCustomer fetches a conversation and verifies the caller's session
// token owns it. Returns false after writing an error response.
func (h *StorefrontHandler) loadForCustomer(c *gin.Context) (*Conversation, bool) {
	id := c.Param("id")
	tenantID := c.GetString(auth.CtxTenantID)
	storeID := c.GetString(auth.CtxStoreID)
	token := c.GetString(auth.CtxSessionToken)

	conv, err := h.d.Conversations.GetForCustomer(c.Request.Context(), tenantID, storeID, id, token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return nil, false
		}
		h.d.Logger.Error("otto: load conversation", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return nil, false
	}
	return conv, true
}

// setSessionCookie writes the signed session cookie. HttpOnly + SameSite=Lax
// means the storefront JS never reads the cookie (mobile apps opt into the
// X-Otto-Session header path instead).
func setSessionCookie(c *gin.Context, name, value, domain string, secure bool, expiry time.Time) {
	maxAge := int(time.Until(expiry).Seconds())
	if maxAge < 60 {
		maxAge = 60
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", domain, secure, true)
}

func displayName(name, email, fallback string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if email != "" {
		return email
	}
	return fallback
}

// mapOTPErrorToResponse translates an OTP verification error into the
// JSON/status the widget understands. Keeping this in one place means
// the error contract between Go and TS stays consistent.
func mapOTPErrorToResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, otp.ErrInvalidCode):
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "otp_invalid",
			"message": "that code didn't match — double-check and try again",
		})
	case errors.Is(err, otp.ErrExpired):
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "otp_expired",
			"message": "that code has expired — request a new one",
		})
	case errors.Is(err, otp.ErrTooManyAttempts):
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "otp_too_many_attempts",
			"message": "too many attempts — request a new code",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "otp_verify_failed"})
	}
}
