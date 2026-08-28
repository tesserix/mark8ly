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

	"github.com/mark8ly/marketplace-api/internal/emaillog"
)

// EmailSendLister is the subset of the send-log platform read this handler
// needs. Narrowed to one method for the same reason as OutboxLister.
type EmailSendLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f emaillog.PlatformListFilter,
		asOf time.Time) (emaillog.PlatformListResult, error)
}

// EmailSendListerFunc adapts a plain function, so emaillog.ListPlatform — a
// package function, not a method — can be wired directly in main.go. Same
// pattern as OutboxListerFunc.
type EmailSendListerFunc func(ctx context.Context, db *gorm.DB,
	f emaillog.PlatformListFilter, asOf time.Time) (emaillog.PlatformListResult, error)

func (fn EmailSendListerFunc) ListPlatform(ctx context.Context, db *gorm.DB,
	f emaillog.PlatformListFilter, asOf time.Time) (emaillog.PlatformListResult, error) {
	return fn(ctx, db, f, asOf)
}

// EmailSendsHandler serves GET /admin/email-sends — the cross-tenant outbound
// mail log answering "did the merchant get the email?" (#348D).
//
// Before #348 nothing recorded a send at all: every mailer handed an envelope
// to email.Sender and no row was written, so the question was unanswerable
// from our own data. This is the read over what piece A records and piece B
// advances.
type EmailSendsHandler struct {
	db     *gorm.DB
	repo   EmailSendLister
	logger *slog.Logger
	now    func() time.Time
}

// NewEmailSendsHandler constructs the handler. logger may be nil.
func NewEmailSendsHandler(db *gorm.DB, repo EmailSendLister, logger *slog.Logger) *EmailSendsHandler {
	return &EmailSendsHandler{db: db, repo: repo, logger: logger, now: time.Now}
}

func (h *EmailSendsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/email-sends", h.List)
}

// emailSendRow is the pinned contract shape.
//
// `subject` and the rendered body are absent BY CONSTRUCTION, not by
// omission: migration 000108 never stores them, and this struct is populated
// field by field from emaillog.PlatformRow, which has no such field either.
// A column added to the table tomorrow cannot leak through here. Subject
// lines are interpolated customer content ("Your order #1234 from Acme
// Ltd") — the same reasoning that keeps `message` out of #332, `description`
// out of #329 and `payload` out of #331.
//
// `error` is emitted as an OPAQUE string: it is provider text, so the values
// a consumer can observe are not a set this service controls. The console
// must render it with an unknown-value fallback, never a switch.
//
// `age_seconds` is absent for a settled row. A number growing forever beside
// a genuinely stuck row would read as stuck.
type emailSendRow struct {
	ID         string  `json:"id"`
	TenantID   *string `json:"tenant_id"`
	StoreID    *string `json:"store_id"`
	Recipient  string  `json:"recipient"`
	Kind       string  `json:"kind"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	SentAt     *string `json:"sent_at,omitempty"`
	EventAt    *string `json:"event_at,omitempty"`
	AgeSeconds *int64  `json:"age_seconds,omitempty"`
	Error      *string `json:"error,omitempty"`
}

type emailSendListResponse struct {
	Data       []emailSendRow `json:"data"`
	Pagination pagination     `json:"pagination"`
}

// List serves GET /admin/email-sends.
func (h *EmailSendsHandler) List(c *gin.Context) {
	f, asOf := h.parseFilter(c)

	res, err := h.repo.ListPlatform(c.Request.Context(), h.db, f, asOf)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform email send list failed", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "could not load the send log",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which defeats
	// a caller's `?? []` exactly when there is no data.
	rows := make([]emailSendRow, 0, len(res.Sends))
	for _, s := range res.Sends {
		rows = append(rows, toEmailSendRow(s))
	}

	c.JSON(http.StatusOK, emailSendListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(f.Page, 1),
			Limit: effectiveSendLimit(f.Limit),
			Total: res.Total,
		},
	})
}

func toEmailSendRow(s emaillog.PlatformRow) emailSendRow {
	row := emailSendRow{
		ID:         s.ID.String(),
		Recipient:  s.Recipient,
		Kind:       s.Kind,
		Status:     s.Status,
		CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339),
		AgeSeconds: s.AgeSeconds,
		Error:      s.Error,
	}
	if s.TenantID != nil {
		v := s.TenantID.String()
		row.TenantID = &v
	}
	if s.StoreID != nil {
		v := s.StoreID.String()
		row.StoreID = &v
	}
	if s.SentAt != nil {
		v := s.SentAt.UTC().Format(time.RFC3339)
		row.SentAt = &v
	}
	if s.EventAt != nil {
		v := s.EventAt.UTC().Format(time.RFC3339)
		row.EventAt = &v
	}
	return row
}

// parseFilter never errors: a malformed parameter takes the default, matching
// every other read on this surface.
func (h *EmailSendsHandler) parseFilter(c *gin.Context) (emaillog.PlatformListFilter, time.Time) {
	f := emaillog.PlatformListFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Kind:   strings.TrimSpace(c.Query("kind")),
		Page:   1,
	}
	if raw := strings.TrimSpace(c.Query("tenant_id")); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			f.TenantID = &id
		}
	}
	for _, p := range []struct {
		key string
		dst *int
	}{{"page", &f.Page}, {"limit", &f.Limit}, {"stuck_minutes", &f.StuckMinutes}} {
		if v := strings.TrimSpace(c.Query(p.key)); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*p.dst = n
			}
		}
	}
	if t, ok := parseSendTime(c.Query("from")); ok {
		f.From = &t
	}
	if t, ok := parseSendTime(c.Query("to")); ok {
		f.To = &t
	}
	return f, h.now().UTC()
}

func parseSendTime(v string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// effectiveSendLimit reports the limit actually applied, so total/limit is a
// correct page count even when the caller asked for more than the ceiling.
func effectiveSendLimit(limit int) int {
	switch {
	case limit <= 0:
		return emaillog.DefaultPlatformPageSize
	case limit > emaillog.MaxPlatformPageSize:
		return emaillog.MaxPlatformPageSize
	default:
		return limit
	}
}
