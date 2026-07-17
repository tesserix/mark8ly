// Package admin — team.go: mobile team-management handler. Proxies to
// platform-api's internal team endpoints (via internal/teamproxy) so the
// mobile admin app can list members, change roles, and manage invitations
// without a second base URL. Tenant-scoped: tenant_id + actor UID come from
// the GIP bearer token, NOT from the :storeId path (the routes are mounted
// under the store group only so the mobile client's store-prefixed requests
// resolve; the store id is ignored here).
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/teamproxy"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// TeamHandler serves the mobile /team endpoints.
type TeamHandler struct {
	client *teamproxy.Client
	logger *slog.Logger
}

// NewTeamHandler constructs a TeamHandler. client is required.
func NewTeamHandler(client *teamproxy.Client, logger *slog.Logger) *TeamHandler {
	return &TeamHandler{client: client, logger: logger}
}

// respondErr maps a teamproxy error onto the wire. A platform-api 4xx/5xx is
// forwarded with its own status + message so the mobile UI shows something
// actionable (e.g. 403 role-guard). A transport failure is a 502.
func (h *TeamHandler) respondErr(c *gin.Context, err error) {
	var apiErr *teamproxy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.Code
		if code == "" {
			code = "platform_error"
		}
		msg := apiErr.Message
		if msg == "" {
			msg = "The team service rejected the request."
		}
		c.AbortWithStatusJSON(apiErr.Status, gin.H{"error": code, "message": msg})
		return
	}
	if h.logger != nil {
		h.logger.Error("team: platform-api call failed", "err", err)
	}
	c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
		"error":   "team_unavailable",
		"message": "The team service is unavailable. Please try again.",
	})
}

// ListMembers handles GET .../team/members.
func (h *TeamHandler) ListMembers(c *gin.Context) {
	members, err := h.client.ListMembers(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": members})
}

// ListInvitations handles GET .../team/invitations.
func (h *TeamHandler) ListInvitations(c *gin.Context) {
	invs, err := h.client.ListInvitations(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": invs})
}

// InviteRequest is the wire body for POST .../team/invitations.
type InviteRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role"  binding:"required,oneof=admin staff viewer"`
}

// Invite handles POST .../team/invitations.
func (h *TeamHandler) Invite(c *gin.Context) {
	var req InviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	inv, err := h.client.CreateInvitation(
		c.Request.Context(), c.GetString("tenant_id"), req.Email, req.Role, c.GetString("user_id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": inv})
}

// Revoke handles DELETE .../team/invitations/:invId.
func (h *TeamHandler) Revoke(c *gin.Context) {
	invID := c.Param("invId")
	if invID == "" {
		RespondErr(c, apperrors.ValidationFailed("invId", "required"), h.logger)
		return
	}
	if err := h.client.RevokeInvitation(
		c.Request.Context(), c.GetString("tenant_id"), invID, c.GetString("user_id")); err != nil {
		h.respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"revoked": true}})
}

// UpdateRoleRequest is the wire body for PATCH .../team/members/role.
type UpdateRoleRequest struct {
	Email   string `json:"email"    binding:"required,email"`
	NewRole string `json:"new_role" binding:"required,oneof=admin staff viewer"`
}

// UpdateRole handles PATCH .../team/members/role.
func (h *TeamHandler) UpdateRole(c *gin.Context) {
	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	res, err := h.client.UpdateMemberRole(
		c.Request.Context(), c.GetString("tenant_id"), req.Email, req.NewRole, c.GetString("user_id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}
