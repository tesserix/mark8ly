// Package admin — notifications.go: HTTP handler for notification
// endpoints (Settings S5).
package admin

import (
	"encoding/json"
	"fmt"
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

// allowedPreferenceKeys is the closed set of user-toggleable notification
// types. Non-toggleable types (e.g. system_alert) are omitted on purpose —
// the UI must not be able to silence them. Keep in sync with
// apps/admin/components/settings/NotificationSettingsClient.tsx.
var allowedPreferenceKeys = map[string]struct{}{
	"new_order":         {},
	"low_stock":         {},
	"return_requested":  {},
	"payment_received":  {},
	"review_submitted":  {},
}

// validatePreferences rejects unknown keys and non-boolean values. Returns
// the normalized map of keys the client submitted on success.
func validatePreferences(raw json.RawMessage) (map[string]bool, error) {
	if !json.Valid(raw) {
		return nil, apperrors.ValidationFailed("preferences", "must be valid JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, apperrors.ValidationFailed("preferences", "must be a JSON object")
	}

	out := make(map[string]bool, len(parsed))
	for key, val := range parsed {
		if _, ok := allowedPreferenceKeys[key]; !ok {
			return nil, apperrors.ValidationFailed("preferences."+key, "unknown preference key")
		}
		b, ok := val.(bool)
		if !ok {
			return nil, apperrors.ValidationFailed("preferences."+key, "must be a boolean")
		}
		out[key] = b
	}
	return out, nil
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

	normalized, err := validatePreferences(req.Preferences)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Re-marshal from the whitelisted map so arbitrary keys cannot slip
	// into the JSONB column even if a future validator regression lets
	// them through the loop above. Marshaling map[string]bool cannot
	// fail in practice; surface as 500 if it ever does.
	clean, err := json.Marshal(normalized)
	if err != nil {
		RespondErr(c, fmt.Errorf("marshal preferences: %w", err), h.logger)
		return
	}

	prefs, err := h.svc.UpsertPreferences(c.Request.Context(), storeID, tenantID, clean)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"store_id":    prefs.StoreID.String(),
		"preferences": json.RawMessage(prefs.Preferences),
	})
}
