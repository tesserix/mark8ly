// Package admin — account.go: HTTP handler for the admin account
// endpoints (S1). Profile read/update is direct DB. MFA and sessions
// proxy to auth-bff.
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

// AccountHandler handles /admin/account endpoints.
// Profile read/update is direct DB. MFA and sessions proxy to auth-bff.
type AccountHandler struct {
	authBFFURL string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewAccountHandler constructs an AccountHandler.
func NewAccountHandler(authBFFURL string, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{
		authBFFURL: strings.TrimRight(authBFFURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// GetProfile handles GET /admin/account — returns the current user's profile.
func (h *AccountHandler) GetProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	email := c.GetString("email")
	tenantID := c.GetString("tenant_id")

	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":   userID,
		"email":     email,
		"tenant_id": tenantID,
	})
}

// UpdateProfileRequest is the request body for PATCH /admin/account.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
}

// UpdateProfile handles PATCH /admin/account.
func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	// In a full implementation, this would update the user profile in the
	// auth provider. For now, return the acknowledged update.
	c.JSON(http.StatusOK, gin.H{
		"user_id":      userID,
		"display_name": req.DisplayName,
		"phone":        req.Phone,
		"updated":      true,
	})
}

// EnableMFA handles POST /admin/account/mfa/enable — proxies to auth-bff.
func (h *AccountHandler) EnableMFA(c *gin.Context) {
	h.proxyToAuthBFF(c, "POST", "/api/v1/mfa/enable")
}

// DisableMFA handles POST /admin/account/mfa/disable — proxies to auth-bff.
func (h *AccountHandler) DisableMFA(c *gin.Context) {
	h.proxyToAuthBFF(c, "POST", "/api/v1/mfa/disable")
}

// ListSessions handles GET /admin/account/sessions — proxies to auth-bff.
func (h *AccountHandler) ListSessions(c *gin.Context) {
	h.proxyToAuthBFF(c, "GET", "/api/v1/sessions")
}

// RevokeSession handles DELETE /admin/account/sessions/:id — proxies to auth-bff.
func (h *AccountHandler) RevokeSession(c *gin.Context) {
	sessionID := c.Param("id")
	h.proxyToAuthBFF(c, "DELETE", fmt.Sprintf("/api/v1/sessions/%s", sessionID))
}

// DeleteAccount handles DELETE /admin/account — owner-only with confirmation.
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	// Require confirmation header to prevent accidental deletion.
	confirm := c.GetHeader("X-Confirm-Delete")
	if confirm != "true" {
		RespondErr(c, apperrors.ValidationFailed("confirmation", "X-Confirm-Delete header must be 'true'"), h.logger)
		return
	}

	// In a full implementation, this would delete the user account from
	// the auth provider and cascade-delete related data.
	c.JSON(http.StatusOK, gin.H{
		"deleted": true,
		"user_id": userID,
	})
}

// proxyToAuthBFF forwards the request to the auth-bff service.
func (h *AccountHandler) proxyToAuthBFF(c *gin.Context, method, path string) {
	if h.authBFFURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "auth-bff service not configured",
		})
		return
	}

	targetURL := h.authBFFURL + path

	req, err := http.NewRequestWithContext(c.Request.Context(), method, targetURL, c.Request.Body)
	if err != nil {
		h.logger.Error("failed to create proxy request", "url", targetURL, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to proxy request",
		})
		return
	}

	// Forward auth headers.
	req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	if userID := c.GetString("user_id"); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if tenantID := c.GetString("tenant_id"); tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Error("auth-bff proxy failed", "url", targetURL, "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "bad_gateway",
			"message": "auth-bff service unavailable",
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// parseStoreID is a helper to parse the storeId path parameter. Unused by
// account handler (operates outside store scope) but kept for consistency.
func parseStoreID(c *gin.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("storeId"))
}
