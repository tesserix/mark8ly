// Package admin — notifications.go: HTTP handler for notification
// endpoints (Settings S5).
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// NotificationsHandler handles /admin/stores/:storeId/notifications endpoints.
type NotificationsHandler struct {
	svc    *notification.Service
	logger *slog.Logger
}

// NewNotificationsHandler constructs a NotificationsHandler.
func NewNotificationsHandler(svc *notification.Service, logger *slog.Logger) *NotificationsHandler {
	return &NotificationsHandler{svc: svc, logger: logger}
}

// NotificationResponse is the wire DTO for a notification.
type NotificationResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	Message      *string `json:"message,omitempty"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID   *string `json:"resource_id,omitempty"`
	IsRead       bool    `json:"is_read"`
	CreatedAt    string  `json:"created_at"`
}

func toNotificationResponse(n notification.Notification) NotificationResponse {
	resp := NotificationResponse{
		ID:           n.ID.String(),
		Type:         string(n.Type),
		Title:        n.Title,
		Message:      n.Message,
		ResourceType: n.ResourceType,
		IsRead:       n.IsRead,
		CreatedAt:    n.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if n.ResourceID != nil {
		s := n.ResourceID.String()
		resp.ResourceID = &s
	}
	return resp
}

// List handles GET /admin/stores/:storeId/notifications.
func (h *NotificationsHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	result, err := h.svc.List(c.Request.Context(), notification.ListFilter{
		StoreID: storeID,
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]NotificationResponse, 0, len(result.Notifications))
	for _, n := range result.Notifications {
		out = append(out, toNotificationResponse(n))
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": out,
		"total":         result.Total,
		"page":          page,
		"per_page":      perPage,
	})
}

// GetUnreadCount handles GET /admin/stores/:storeId/notifications/unread-count.
func (h *NotificationsHandler) GetUnreadCount(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	count, err := h.svc.GetUnreadCount(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

// MarkRead handles PATCH /admin/stores/:storeId/notifications/:id/read.
func (h *NotificationsHandler) MarkRead(c *gin.Context) {
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

	if err := h.svc.MarkRead(c.Request.Context(), storeID, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"marked": true})
}

// MarkAllRead handles PATCH /admin/stores/:storeId/notifications/read-all.
func (h *NotificationsHandler) MarkAllRead(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.MarkAllRead(c.Request.Context(), storeID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"marked_all": true})
}

// GetPreferences handles GET /admin/stores/:storeId/notification-preferences.
func (h *NotificationsHandler) GetPreferences(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	prefs, err := h.svc.GetPreferences(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"store_id":    prefs.StoreID.String(),
		"preferences": json.RawMessage(prefs.Preferences),
	})
}

// UpdatePreferencesRequest is the request body for PATCH .../notification-preferences.
type UpdatePreferencesRequest struct {
	Preferences json.RawMessage `json:"preferences" binding:"required"`
}

// UpdatePreferences handles PATCH /admin/stores/:storeId/notification-preferences.
func (h *NotificationsHandler) UpdatePreferences(c *gin.Context) {
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

	var req UpdatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	// Validate that preferences is valid JSON.
	if !json.Valid(req.Preferences) {
		RespondErr(c, apperrors.ValidationFailed("preferences", "must be valid JSON"), h.logger)
		return
	}

	prefs, err := h.svc.UpsertPreferences(c.Request.Context(), storeID, tenantID, req.Preferences)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"store_id":    prefs.StoreID.String(),
		"preferences": json.RawMessage(prefs.Preferences),
	})
}
