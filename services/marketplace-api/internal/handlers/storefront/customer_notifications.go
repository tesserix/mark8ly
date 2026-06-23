// Package storefront — customer_notifications.go: the storefront customer
// notification bell. Lists / counts / marks-read a logged-in customer's
// notifications (recipient_user_id = customer profile id). All routes require
// RequireCustomerAuth + StoreContext.
package storefront

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// CustomerNotificationsHandler serves the storefront /account/notifications routes.
type CustomerNotificationsHandler struct {
	svc *notification.Service
}

// NewCustomerNotificationsHandler constructs the handler. svc may be nil
// (notifications disabled) — handlers degrade to empty results.
func NewCustomerNotificationsHandler(svc *notification.Service) *CustomerNotificationsHandler {
	return &CustomerNotificationsHandler{svc: svc}
}

type notificationResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	Message      *string `json:"message"`
	IsRead       bool    `json:"is_read"`
	CreatedAt    string  `json:"created_at"`
	ResourceType *string `json:"resource_type"`
	ResourceID   *string `json:"resource_id"`
}

func toResponse(n notification.Notification) notificationResponse {
	r := notificationResponse{
		ID:           n.ID.String(),
		Type:         string(n.Type),
		Title:        n.Title,
		Message:      n.Message,
		IsRead:       n.IsRead,
		CreatedAt:    n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ResourceType: n.ResourceType,
	}
	if n.ResourceID != nil {
		s := n.ResourceID.String()
		r.ResourceID = &s
	}
	return r
}

// resolve returns (storeID, customerProfileID, ok). On failure it writes the
// response and returns ok=false.
func (h *CustomerNotificationsHandler) resolve(c *gin.Context) (uuid.UUID, string, bool) {
	storeVal, _ := c.Get("store")
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "store_not_resolved"})
		return uuid.Nil, "", false
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "store_not_resolved"})
		return uuid.Nil, "", false
	}
	val, exists := c.Get(CustomerProfileKey)
	profile, pok := val.(*customer.CustomerProfile)
	if !exists || !pok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.Nil, "", false
	}
	return storeID, profile.ID.String(), true
}

// List handles GET /account/notifications.
func (h *CustomerNotificationsHandler) List(c *gin.Context) {
	storeID, recipient, ok := h.resolve(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusOK, gin.H{"notifications": []notificationResponse{}})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	res, err := h.svc.ListForCustomer(c.Request.Context(), storeID, recipient, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	out := make([]notificationResponse, 0, len(res.Notifications))
	for _, n := range res.Notifications {
		out = append(out, toResponse(n))
	}
	c.JSON(http.StatusOK, gin.H{"notifications": out, "total": res.Total})
}

// UnreadCount handles GET /account/notifications/unread-count.
func (h *CustomerNotificationsHandler) UnreadCount(c *gin.Context) {
	storeID, recipient, ok := h.resolve(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusOK, gin.H{"unread_count": 0})
		return
	}
	count, err := h.svc.GetUnreadCountForCustomer(c.Request.Context(), storeID, recipient)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

// MarkRead handles PATCH /account/notifications/:id/read.
func (h *CustomerNotificationsHandler) MarkRead(c *gin.Context) {
	storeID, recipient, ok := h.resolve(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_id"})
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err := h.svc.MarkReadForCustomer(c.Request.Context(), storeID, recipient, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkAllRead handles PATCH /account/notifications/read-all.
func (h *CustomerNotificationsHandler) MarkAllRead(c *gin.Context) {
	storeID, recipient, ok := h.resolve(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err := h.svc.MarkAllReadForCustomer(c.Request.Context(), storeID, recipient); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mark_all_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
