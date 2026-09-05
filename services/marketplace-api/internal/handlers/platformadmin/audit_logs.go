package platformadmin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// AuditLogsHandler serves GET /admin/audit-logs to the platform console.
//
// The row shape here is NOT audit.Response — that belongs to the merchant
// Settings page. This one follows the contract pinned on #276, which renames
// or drops most fields. Do not consolidate the two.
type AuditLogsHandler struct {
	db     *gorm.DB
	repo   audit.Repository
	logger *slog.Logger
}

// NewAuditLogsHandler constructs the handler. logger may be nil.
func NewAuditLogsHandler(db *gorm.DB, repo audit.Repository, logger *slog.Logger) *AuditLogsHandler {
	return &AuditLogsHandler{db: db, repo: repo, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *AuditLogsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/audit-logs", h.List)
}

// auditRow is the pinned wire shape. Fields we hold but the contract does not
// name — status, severity, ip_address, user_agent, actor_type — are
// deliberately absent. Adding fields unilaterally is what the contract exists
// to prevent.
//
// `metadata` is a STRING carrying compact JSON, not an object — settled on
// #313 by reading the consumer rather than by asking again. All three layers
// of tesserix-home agree:
//
//   - platform-api internal/modules/audit/internal/domain/entry.go —
//     `Metadata string`, and that struct is the federation decode target
//   - apps/console/lib/audit.ts — `optionalStr(row.metadata, ...)`
//   - the console's own audit writer renders compact JSON into the column
//
// The cost of the other choice is not a mis-rendered field. platform-api
// decodes an entire page with one json.Unmarshal, so an object where a string
// is expected fails that decode and mark8ly is reported as a federation
// FAILURE — every mark8ly row disappears from the console at once.
type auditRow struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Target    string `json:"target,omitempty"`
	Metadata  string `json:"metadata,omitempty"`
}

type pagination struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type listResponse struct {
	Data       []auditRow `json:"data"`
	Pagination pagination `json:"pagination"`
}

// List handles GET /admin/audit-logs.
func (h *AuditLogsHandler) List(c *gin.Context) {
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform audit logs list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read audit logs",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no data.
	rows := make([]auditRow, 0, len(result.Entries))
	for _, e := range result.Entries {
		rows = append(rows, toRow(e))
	}

	c.JSON(http.StatusOK, listResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: filter.Limit,
			Total: result.Total,
		},
	})
}

// toRow maps a stored entry to the pinned contract shape.
func toRow(e audit.Entry) auditRow {
	return auditRow{
		// Bare id. The platform API namespaces as <slug>:<id> on arrival;
		// prefixing here yields "mark8ly:mark8ly:9f2".
		ID:        e.ID.String(),
		Actor:     actorOf(e),
		Action:    e.Action,
		Timestamp: e.CreatedAt.UTC().Format(time.RFC3339),
		Target:    targetOf(e),
		Metadata:  metadataOf(e),
	}
}

// metadataOf renders the jsonb blob as the compact JSON string the consumer
// reads, or "" to omit the field.
//
// "" for an empty map as well as a nil one: "this event carried no detail"
// and "this event carried a detail object containing nothing" are the same
// fact to a reader, and the contract marks the field optional. Sending "{}"
// would put a literal empty-braces string in front of an operator.
//
// A marshal failure also yields "" rather than propagating. Metadata is
// loaded from jsonb so it should always re-marshal, but this runs per row on
// a page: one unserialisable value must cost that row its detail, never the
// whole page its rows.
func metadataOf(e audit.Entry) string {
	if len(e.Metadata) == 0 {
		return ""
	}
	// encoding/json sorts map keys, so the output is byte-stable across calls
	// and a poller does not see a phantom change on an unchanged row.
	encoded, err := json.Marshal(e.Metadata)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// actorOf resolves the single "who did it" string the contract asks for.
// A merchant has an email; a platform operator has an opaque id; anything
// else was the system acting on its own.
func actorOf(e audit.Entry) string {
	if e.ActorEmail != nil && *e.ActorEmail != "" {
		return *e.ActorEmail
	}
	if e.ActorOperatorID != nil && *e.ActorOperatorID != "" {
		return *e.ActorOperatorID
	}
	return "system"
}

// targetOf collapses resource_type + resource_id into the contract's single
// `target`. The pinned example shows a bare id ("prod_123"), so the id wins
// when present and the type is the fallback for rows that have none.
func targetOf(e audit.Entry) string {
	if e.ResourceID != nil && strings.TrimSpace(*e.ResourceID) != "" {
		return *e.ResourceID
	}
	return e.ResourceType
}

// parseFilter never returns an error. The contract states a missing parameter
// takes our default, and an oversized limit clamps rather than refusing — a
// ceiling on our side is the backstop for a caller asking for too much.
func (h *AuditLogsHandler) parseFilter(c *gin.Context) audit.PlatformListFilter {
	f := audit.PlatformListFilter{
		Action:       strings.TrimSpace(c.Query("action")),
		Actor:        strings.TrimSpace(c.Query("actor")),
		ResourceType: strings.TrimSpace(c.Query("resource_type")),
		Page:         1,
		Limit:        audit.DefaultPlatformPageSize,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, audit.MaxPlatformPageSize)
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.DateFrom = time.Now().Add(-time.Duration(n) * time.Hour)
		}
	}
	// Explicit from/to win over since_hours when both are supplied.
	if t, ok := parseTime(c.Query("from")); ok {
		f.DateFrom = t
	}
	if t, ok := parseTime(c.Query("to")); ok {
		f.DateTo = t
	}
	if v := strings.TrimSpace(c.Query("store_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.StoreID = id
		}
	}
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = id
		}
	}
	return f
}

func parseTime(v string) (time.Time, bool) {
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
