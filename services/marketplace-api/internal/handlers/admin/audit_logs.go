// Package admin — audit_logs.go: HTTP handler for audit log endpoints
// (Settings S4). Proxies requests to the audit-service.
package admin

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// AuditLogsHandler handles /admin/stores/:storeId/audit-logs endpoints.
// All calls proxy to the audit-service.
type AuditLogsHandler struct {
	auditServiceURL string
	httpClient      *http.Client
	logger          *slog.Logger
}

// NewAuditLogsHandler constructs an AuditLogsHandler.
func NewAuditLogsHandler(auditServiceURL string, logger *slog.Logger) *AuditLogsHandler {
	return &AuditLogsHandler{
		auditServiceURL: strings.TrimRight(auditServiceURL, "/"),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		logger:          logger,
	}
}

// List handles GET /admin/stores/:storeId/audit-logs — proxies to audit-service.
func (h *AuditLogsHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	h.proxyToAuditService(c, "GET", fmt.Sprintf("/api/v1/stores/%s/audit-logs?%s", storeID, c.Request.URL.RawQuery))
}

// ExportCSV handles GET /admin/stores/:storeId/audit-logs/export — proxies to audit-service.
func (h *AuditLogsHandler) ExportCSV(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	h.proxyToAuditService(c, "GET", fmt.Sprintf("/api/v1/stores/%s/audit-logs/export?%s", storeID, c.Request.URL.RawQuery))
}

// proxyToAuditService forwards the request to the audit-service.
func (h *AuditLogsHandler) proxyToAuditService(c *gin.Context, method, path string) {
	if h.auditServiceURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "audit service not configured",
		})
		return
	}

	targetURL := h.auditServiceURL + path

	req, err := http.NewRequestWithContext(c.Request.Context(), method, targetURL, nil)
	if err != nil {
		h.logger.Error("failed to create audit proxy request", "url", targetURL, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to proxy request",
		})
		return
	}

	// Forward tenant context headers.
	if tenantID := c.GetString("tenant_id"); tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	if userID := c.GetString("user_id"); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Error("audit service proxy failed", "url", targetURL, "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "bad_gateway",
			"message": "audit service unavailable",
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
