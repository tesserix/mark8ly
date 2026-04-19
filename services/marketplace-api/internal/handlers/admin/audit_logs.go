// Package admin — audit_logs.go: HTTP handler for the Settings -> Audit
// Logs page. Reads from the in-process audit_logs table via the audit
// repository (no external service).
package admin

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// AuditLogsHandler serves /admin/stores/:storeId/audit-logs endpoints.
type AuditLogsHandler struct {
	db     *gorm.DB
	repo   audit.Repository
	logger *slog.Logger
}

// NewAuditLogsHandler constructs an AuditLogsHandler. db and repo are
// required; logger is required for diagnostics.
func NewAuditLogsHandler(db *gorm.DB, repo audit.Repository, logger *slog.Logger) *AuditLogsHandler {
	return &AuditLogsHandler{db: db, repo: repo, logger: logger}
}

// listResponse mirrors the AuditLogsResponse interface in
// apps/admin/lib/api/settings-tier2-api.ts. Keep field names in sync.
type listResponse struct {
	Data []audit.Response `json:"data"`
	Meta listMeta         `json:"meta"`
}

type listMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// List handles GET /admin/stores/:storeId/audit-logs.
func (h *AuditLogsHandler) List(c *gin.Context) {
	filter, err := h.parseFilter(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	result, err := h.repo.List(c.Request.Context(), h.db, filter)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := listResponse{
		Data: make([]audit.Response, 0, len(result.Entries)),
		Meta: listMeta{
			Page:       max(filter.Page, 1),
			PageSize:   filter.PageSize,
			Total:      result.Total,
			TotalPages: totalPages(result.Total, filter.PageSize),
		},
	}
	for _, e := range result.Entries {
		out.Data = append(out.Data, e.ToResponse())
	}

	c.JSON(http.StatusOK, out)
}

// ExportCSV handles GET /admin/stores/:storeId/audit-logs/export.
// Streams matching rows as CSV. Pagination is ignored.
func (h *AuditLogsHandler) ExportCSV(c *gin.Context) {
	filter, err := h.parseFilter(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	// Export ignores pagination — clear so the stream returns all rows.
	filter.Page = 0
	filter.PageSize = 0

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	c.Status(http.StatusOK)

	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	header := []string{
		"id", "timestamp", "user_email", "action", "resource_type",
		"resource_id", "status", "severity", "ip_address", "user_agent",
	}
	if err := w.Write(header); err != nil {
		h.logger.Error("audit export: write header", "err", err)
		return
	}

	streamErr := h.repo.Stream(c.Request.Context(), h.db, filter, func(e *audit.Entry) error {
		r := e.ToResponse()
		return w.Write([]string{
			escapeCSVCell(r.ID),
			escapeCSVCell(r.Timestamp),
			escapeCSVCell(r.UserEmail),
			escapeCSVCell(r.Action),
			escapeCSVCell(r.ResourceType),
			escapeCSVCell(r.ResourceID),
			escapeCSVCell(r.Status),
			escapeCSVCell(r.Severity),
			escapeCSVCell(r.IPAddress),
			escapeCSVCell(r.UserAgent),
		})
	})
	if streamErr != nil {
		// Headers already sent; just log so the operator can see it.
		h.logger.Error("audit export: stream", "err", streamErr)
	}
}

// escapeCSVCell defuses CSV formula-injection when the exported file is
// opened in Excel/LibreOffice/Sheets: cells beginning with '=', '+', '-',
// '@' or a tab are interpreted as a formula and can call out to external
// resources (DDE, WEBSERVICE, HYPERLINK). Prefixing with a single-quote
// ties the cell to plain-text rendering. Applied to every column in the
// export since any user-supplied field (email local-part, user-agent,
// action descriptor) can land here.
func escapeCSVCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t':
		return "'" + s
	}
	return s
}

// parseFilter pulls tenant + store from gin context and the rest from
// query string. Returns an apperrors.* error suitable for RespondErr.
func (h *AuditLogsHandler) parseFilter(c *gin.Context) (audit.ListFilter, error) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		return audit.ListFilter{}, apperrors.ValidationFailed("storeId", "invalid uuid")
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		return audit.ListFilter{}, apperrors.ValidationFailed("tenant_id", "invalid uuid")
	}

	f := audit.ListFilter{
		TenantID:     tenantID,
		StoreID:      storeID,
		UserEmail:    strings.TrimSpace(c.Query("user")),
		Action:       strings.TrimSpace(c.Query("action")),
		ResourceType: strings.TrimSpace(c.Query("resource_type")),
		Severity:     strings.TrimSpace(c.Query("severity")),
		Search:       strings.TrimSpace(c.Query("search")),
	}

	if v := c.Query("date_from"); v != "" {
		t, err := parseDate(v)
		if err != nil {
			return audit.ListFilter{}, apperrors.ValidationFailed("date_from", "invalid date")
		}
		f.DateFrom = t
	}
	if v := c.Query("date_to"); v != "" {
		t, err := parseDate(v)
		if err != nil {
			return audit.ListFilter{}, apperrors.ValidationFailed("date_to", "invalid date")
		}
		// Treat as end-of-day so a single-day filter matches all events that day.
		f.DateTo = t.Add(24*time.Hour - time.Second)
	}

	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return audit.ListFilter{}, apperrors.ValidationFailed("page", "invalid")
		}
		f.Page = n
	} else {
		f.Page = 1
	}
	if v := c.Query("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		// Fail fast on a clearly-abusive page size rather than silently
		// capping it in the repository. Upper-bound matches the repo's
		// internal clamp (audit/repository.go) so the contract is visible
		// in the handler.
		if err != nil || n < 1 || n > 100 {
			return audit.ListFilter{}, apperrors.ValidationFailed("page_size", "must be between 1 and 100")
		}
		f.PageSize = n
	} else {
		f.PageSize = 25
	}

	return f, nil
}

// parseDate accepts either RFC3339 timestamps or YYYY-MM-DD dates.
func parseDate(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}

func totalPages(total int64, pageSize int) int64 {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	pages := total / int64(pageSize)
	if total%int64(pageSize) != 0 {
		pages++
	}
	return pages
}
