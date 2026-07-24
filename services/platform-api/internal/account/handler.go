package account

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// accountDeleter is the subset of *Service the handler depends on. Defined
// as an interface (rather than depending on *Service directly) so tests
// can inject a fake and exercise the handler without a real DB/FGA/GIP.
type accountDeleter interface {
	DeleteAccount(ctx context.Context, tenantID, actorUID string) error
}

// Handler is the HTTP layer for account teardown.
type Handler struct {
	svc accountDeleter
}

// NewHandler constructs a Handler. svc is typically *account.Service.
func NewHandler(svc accountDeleter) *Handler {
	return &Handler{svc: svc}
}

// Register mounts DELETE /tenants/:id/account onto the given internal
// gin.RouterGroup. This is an internal-only route — the caller (admin BFF
// / auth-bff) is expected to have already authenticated the actor and
// supplies their GIP uid in the request body.
func (h *Handler) Register(internal *gin.RouterGroup) {
	internal.Group("/tenants").DELETE("/:id/account", h.delete)
}

// deleteRequest is the body for DELETE /tenants/:id/account.
type deleteRequest struct {
	UID string `json:"uid"`
}

func (h *Handler) delete(c *gin.Context) {
	var req deleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_uid", "message": "uid is required"})
		return
	}
	if err := h.svc.DeleteAccount(c.Request.Context(), c.Param("id"), req.UID); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// respondError maps apperrors typed errors to their HTTP status. This is
// a deliberate copy of internal/tenant/handler.go's unexported
// respondError rather than a shared extraction: the mapping is six lines,
// package-private in tenant, and extracting a shared package would mean
// touching tenant/handler.go too (out of scope for this task) for a
// helper this small. See task-5-report.md for the full rationale.
func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{"error": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
