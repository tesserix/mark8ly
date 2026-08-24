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

	"github.com/mark8ly/marketplace-api/internal/ticket"
)

// TicketLister is the subset of ticket.Repository this handler needs.
// Narrowed to one method for the same reason as EstateCounts in kpis.go and
// OnboardingFunnel in onboarding.go.
type TicketLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f ticket.PlatformListFilter) (ticket.ListResult, error)
}

// TicketsHandler serves GET /admin/tickets to the platform console — a
// cross-store, cross-tenant read of support tickets.
type TicketsHandler struct {
	db     *gorm.DB
	repo   TicketLister
	logger *slog.Logger
}

// NewTicketsHandler constructs the handler. logger may be nil.
func NewTicketsHandler(db *gorm.DB, repo TicketLister, logger *slog.Logger) *TicketsHandler {
	return &TicketsHandler{db: db, repo: repo, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *TicketsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/tickets", h.List)
}

// ticketRow is the pinned contract shape. description and replies are
// DELIBERATELY absent: a cross-tenant governance surface must not become a
// way to read every merchant's customer correspondence. Same reasoning that
// keeps `payload` out of #331 and message bodies out of #332. A body view
// needs its own endpoint, capability and justification.
//
// There is no assignee field because no such column exists anywhere in the
// ticket schema — #329 asked for one; tickets have a submitter (#329 comment).
type ticketRow struct {
	ID             string  `json:"id"`
	TicketNumber   string  `json:"ticket_number"`
	TenantID       string  `json:"tenant_id"`
	StoreID        string  `json:"store_id"`
	Subject        string  `json:"subject"`
	Status         string  `json:"status"`
	Priority       string  `json:"priority"`
	RequesterName  string  `json:"requester_name"`
	RequesterEmail string  `json:"requester_email"`
	ConversationID *string `json:"conversation_id,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ResolvedAt     *string `json:"resolved_at,omitempty"`
}

type ticketListResponse struct {
	Data       []ticketRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

// List handles GET /admin/tickets.
func (h *TicketsHandler) List(c *gin.Context) {
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform tickets list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read tickets",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which defeats
	// a caller's `?? []` and crashes their page precisely when there is no
	// data.
	rows := make([]ticketRow, 0, len(result.Tickets))
	for _, tk := range result.Tickets {
		rows = append(rows, toTicketRow(tk))
	}

	c.JSON(http.StatusOK, ticketListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: filter.Limit,
			Total: result.Total,
		},
	})
}

// toTicketRow maps a stored ticket to the pinned contract shape, field by
// field. description and replies are intentionally not read from tk.
func toTicketRow(tk ticket.Ticket) ticketRow {
	row := ticketRow{
		ID:             tk.ID.String(),
		TicketNumber:   tk.TicketNumber,
		TenantID:       tk.TenantID.String(),
		StoreID:        tk.StoreID.String(),
		Subject:        tk.Subject,
		Status:         string(tk.Status),
		Priority:       string(tk.Priority),
		RequesterName:  tk.SubmittedByName,
		RequesterEmail: tk.SubmittedByEmail,
		ConversationID: tk.ConversationID,
		CreatedAt:      tk.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      tk.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if tk.ResolvedAt != nil {
		resolved := tk.ResolvedAt.UTC().Format(time.RFC3339)
		row.ResolvedAt = &resolved
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes our default,
// and an oversized limit clamps rather than refusing — matching the audit
// logs contract (#276).
func (h *TicketsHandler) parseFilter(c *gin.Context) ticket.PlatformListFilter {
	f := ticket.PlatformListFilter{
		Status:   strings.TrimSpace(c.Query("status")),
		Priority: strings.TrimSpace(c.Query("priority")),
		Page:     1,
		Limit:    ticket.DefaultPlatformPageSize,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, ticket.MaxPlatformPageSize)
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
	if t, ok := parseTicketTime(c.Query("from")); ok {
		f.From = &t
	}
	if t, ok := parseTicketTime(c.Query("to")); ok {
		f.To = &t
	}
	// store_id NARROWS rather than scopes — see ticket.PlatformListFilter.
	if v := strings.TrimSpace(c.Query("store_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.StoreID = &id
		}
	}
	return f
}

func parseTicketTime(v string) (time.Time, bool) {
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
