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

	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// OutboxLister is the subset of the outbox platform read this handler
// needs. Narrowed to one method for the same reason as NotificationLister
// in notifications.go and TicketLister in tickets.go.
type OutboxLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
		asOf time.Time) (outbox.PlatformListResult, error)
}

// OutboxListerFunc adapts a plain function to OutboxLister, so
// outbox.ListPlatform — which is a package function, not a method — can be
// wired directly in main.go. Same pattern as TrialListerFunc.
type OutboxListerFunc func(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
	asOf time.Time) (outbox.PlatformListResult, error)

func (fn OutboxListerFunc) ListPlatform(ctx context.Context, db *gorm.DB,
	f outbox.PlatformListFilter, asOf time.Time) (outbox.PlatformListResult, error) {
	return fn(ctx, db, f, asOf)
}

// OutboxHandler serves GET /admin/outbox to the platform console — a
// cross-tenant read of outbox_events answering "what is stuck, what failed,
// and why" (#331).
//
// This surface exists because a row with published_at IS NULL and a non-null
// error, hours old, means a downstream integration is silently not
// happening, and nothing outside mark8ly could see it. Before #336 the
// `failed` state could not occur at all: nothing wrote outbox_events.error.
type OutboxHandler struct {
	db     *gorm.DB
	repo   OutboxLister
	logger *slog.Logger
	now    func() time.Time
}

// NewOutboxHandler constructs the handler. logger may be nil.
func NewOutboxHandler(db *gorm.DB, repo OutboxLister, logger *slog.Logger) *OutboxHandler {
	return &OutboxHandler{db: db, repo: repo, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group.
func (h *OutboxHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/outbox", h.List)
}

// outboxRow is the pinned contract shape.
//
// `payload` is DELIBERATELY absent, and absent by CONSTRUCTION: this struct
// is populated field by field from outbox.PlatformRow, which has no payload
// field either, so a column added to the model tomorrow cannot leak. It is
// arbitrary JSONB that may carry customer data, and a governance surface
// listing stuck events does not need it. Same reasoning that keeps
// `message` out of #332 and `description` out of #329.
//
// `error` is emitted as an OPAQUE string. outbox_events.error has no CHECK
// constraint and the operator requeue path is a raw UPDATE, so the codes
// this service writes are not the only values a consumer can observe. The
// console must render it with an unknown-value fallback, never a switch.
//
// `age_seconds` is absent for a published row: that row is settled and has
// no waiting time. A number that grew forever there would read as "stuck"
// beside a genuinely stuck row.
type outboxRow struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Aggregate   string  `json:"aggregate"`
	AggregateID string  `json:"aggregate_id"`
	EventType   string  `json:"event_type"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	AgeSeconds  *int64  `json:"age_seconds,omitempty"`
	PublishedAt *string `json:"published_at,omitempty"`
	Error       *string `json:"error,omitempty"`
}

type outboxListResponse struct {
	Data       []outboxRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

// List handles GET /admin/outbox.
func (h *OutboxHandler) List(c *gin.Context) {
	// One instant for both the age and the older_than_minutes cutoff, so a
	// rendered age can never disagree with the filter that selected it.
	asOf := h.now().UTC()
	filter := h.parseFilter(c, asOf)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter, asOf)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform outbox list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read outbox events",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` precisely when there is no data.
	rows := make([]outboxRow, 0, len(result.Rows))
	for _, r := range result.Rows {
		rows = append(rows, toOutboxRow(r))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = outbox.DefaultPlatformPageSize
	}
	if limit > outbox.MaxPlatformPageSize {
		limit = outbox.MaxPlatformPageSize
	}

	c.JSON(http.StatusOK, outboxListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: limit,
			Total: result.Total,
		},
	})
}

// toOutboxRow maps a query row to the pinned contract shape, FIELD BY
// FIELD. Nothing iterates the source struct, so the absence of payload is a
// property of this projection rather than of what the query happened to
// select.
func toOutboxRow(r outbox.PlatformRow) outboxRow {
	row := outboxRow{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Aggregate:   r.Aggregate,
		AggregateID: r.AggregateID,
		EventType:   r.EventType,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
		AgeSeconds:  r.AgeSeconds,
		Error:       r.Error,
	}
	if r.PublishedAt != nil {
		s := r.PublishedAt.UTC().Format(time.RFC3339)
		row.PublishedAt = &s
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes the
// default, an unparseable one is ignored, and an oversized limit clamps
// downstream rather than refusing — matching audit logs (#276), tickets
// (#329) and notifications (#332). It takes asOf rather than reading the
// clock so that since_hours, older_than_minutes and age_seconds are all
// measured from the SAME instant — a response whose rendered age disagreed
// with the window that selected it would be quietly wrong.
func (h *OutboxHandler) parseFilter(c *gin.Context, asOf time.Time) outbox.PlatformListFilter {
	f := outbox.PlatformListFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		EventType: strings.TrimSpace(c.Query("event_type")),
		Page:      1,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("older_than_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.OlderThanMinutes = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			from := asOf.Add(-time.Duration(n) * time.Hour)
			f.From = &from
		}
	}
	// tenant_id NARROWS rather than scopes — this endpoint is cross-tenant
	// by design.
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = &id
		}
	}
	return f
}
