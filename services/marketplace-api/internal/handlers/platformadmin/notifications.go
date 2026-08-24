package platformadmin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
)

// NotificationLister is the subset of notification.Repository this handler
// needs. Narrowed to one method for the same reason as TicketLister in
// tickets.go and EstateCounts in kpis.go.
type NotificationLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f notification.PlatformListFilter) (notification.ListResult, error)
}

// NotificationsHandler serves GET /admin/notifications to the platform
// console — a cross-store, cross-tenant read of the in-app notification
// log.
//
// This is the IN-APP notification bell, not a sent-mail log. #332 asked for
// one; no record of outbound mail exists anywhere in this estate —
// transactional mail is fire-and-forget through internal/email, no provider
// event webhook was ever rebuilt, and campaign_recipients only ever writes
// `sent`. That work is #348. Nothing here reports a delivery outcome, and
// nothing here should be made to look as though it does.
type NotificationsHandler struct {
	db     *gorm.DB
	repo   NotificationLister
	logger *slog.Logger
}

// NewNotificationsHandler constructs the handler. logger may be nil.
func NewNotificationsHandler(db *gorm.DB, repo NotificationLister, logger *slog.Logger) *NotificationsHandler {
	return &NotificationsHandler{db: db, repo: repo, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *NotificationsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/notifications", h.List)
}

// notificationRow is the pinned contract shape.
//
// `message` is DELIBERATELY absent: it is the interpolated body, the only
// field in the row carrying customer detail, and a cross-tenant governance
// surface must not become a way to read every merchant's correspondence.
// Same reasoning that keeps `description` out of #329 and `payload` out of
// #331.
//
// There is no `status` field. No delivery status exists in this estate
// (#348); emitting is_read under that name would put a governance label on
// a metric answering a different question, and an operator would act on it.
//
// `audience` is always present so an absent recipient_user_id reads as
// "this went to the store" rather than "the recipient lookup failed".
type notificationRow struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id"`
	StoreID         string  `json:"store_id"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Audience        string  `json:"audience"`
	RecipientUserID *string `json:"recipient_user_id,omitempty"`
	ResourceType    *string `json:"resource_type,omitempty"`
	ResourceID      *string `json:"resource_id,omitempty"`
	IsRead          bool    `json:"is_read"`
	CreatedAt       string  `json:"created_at"`
}

type notificationListResponse struct {
	Data       []notificationRow `json:"data"`
	Pagination pagination        `json:"pagination"`
}

// List handles GET /admin/notifications.
func (h *NotificationsHandler) List(c *gin.Context) {
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform notifications list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read notifications",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` and crashes their page precisely when
	// there is no data.
	rows := make([]notificationRow, 0, len(result.Notifications))
	for _, n := range result.Notifications {
		rows = append(rows, toNotificationRow(n))
	}

	c.JSON(http.StatusOK, notificationListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: filter.Limit,
			Total: result.Total,
		},
	})
}

// toNotificationRow maps a stored notification to the pinned contract
// shape, FIELD BY FIELD. n.Message is never read — the body's absence is a
// property of this projection, not of what the query happened to select, so
// a column added to notification.Notification tomorrow cannot leak.
func toNotificationRow(n notification.Notification) notificationRow {
	row := notificationRow{
		ID:        n.ID.String(),
		TenantID:  n.TenantID.String(),
		StoreID:   n.StoreID.String(),
		Type:      string(n.Type),
		Title:     n.Title,
		Audience:  notification.AudienceStore,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
	}
	if n.RecipientUserID != nil && *n.RecipientUserID != "" {
		row.Audience = notification.AudienceCustomer
		row.RecipientUserID = n.RecipientUserID
	}
	if n.ResourceType != nil {
		row.ResourceType = n.ResourceType
	}
	if n.ResourceID != nil {
		id := n.ResourceID.String()
		row.ResourceID = &id
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes our
// default, and an oversized limit clamps rather than refusing — matching
// the audit logs contract (#276) and tickets (#329).
func (h *NotificationsHandler) parseFilter(c *gin.Context) notification.PlatformListFilter {
	f := notification.PlatformListFilter{
		Type:            strings.TrimSpace(c.Query("type")),
		RecipientUserID: strings.TrimSpace(c.Query("recipient_user_id")),
		Page:            1,
		Limit:           notification.DefaultPlatformPageSize,
	}

	// An unrecognised audience narrows nothing rather than erroring,
	// matching how every other unknown parameter here behaves.
	switch strings.TrimSpace(c.Query("audience")) {
	case notification.AudienceStore:
		f.Audience = notification.AudienceStore
	case notification.AudienceCustomer:
		f.Audience = notification.AudienceCustomer
	}

	// read=false must narrow to unread rows, so the pointer is set for
	// BOTH values — not only for "true".
	if v := strings.TrimSpace(c.Query("read")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			f.Read = &b
		}
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, notification.MaxPlatformPageSize)
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			from := time.Now().Add(-time.Duration(n) * time.Hour)
			f.From = &from
		}
	}
	// Explicit from/to win over since_hours when both are supplied.
	if t, ok := parseNotificationTime(c.Query("from")); ok {
		f.From = &t
	}
	if t, ok := parseNotificationTime(c.Query("to")); ok {
		f.To = &t
	}
	// tenant_id and store_id NARROW rather than scope — see
	// notification.PlatformListFilter.
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = &id
		}
	}
	if v := strings.TrimSpace(c.Query("store_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.StoreID = &id
		}
	}
	return f
}

func parseNotificationTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
