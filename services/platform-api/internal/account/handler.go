package account

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// accountDeleter is the subset of *Service the handler depends on. Defined
// as an interface (rather than depending on *Service directly) so tests
// can inject a fake and exercise the handler without a real DB/FGA/GIP.
type accountDeleter interface {
	DeleteAccount(ctx context.Context, tenantID, actorUID string) error
	PurgeTenant(ctx context.Context, tenantID string, storeSlugs []string) (*PurgeResult, error)
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

// teardownRequest is the body for POST /internal/tenants/:id/teardown.
//
// ABSENT store_slugs and an EMPTY array mean different things and both
// must survive to the check: absent is a client that dropped the
// confirmation and must fail; empty is a deliberate assertion that this
// tenant has no stores, and matches only a tenant that genuinely has
// none. encoding/json already distinguishes the two without the pointer
// (nil for absent, a non-nil empty slice for []); StoreSlugs is a POINTER
// anyway so the requirement lives in the TYPE rather than depending on a
// JSON decoder's nil-vs-empty convention for slices.
type teardownRequest struct {
	StoreSlugs *[]string `json:"store_slugs"`
}

// RegisterOperator mounts the operator-initiated teardown.
//
// Mounted on the STRICT internal group (which answers 503 when the shared
// secret is unset), and mounted UNCONDITIONALLY — unlike Register, whose
// merchant route is gated on FGA and GIP being wired. An absent route
// answers 404, and the caller cannot tell that apart from "no such
// tenant", which on an irreversible endpoint would be a silent lie.
// PurgeTenant tolerates nil FGA and GIP clients for exactly this reason.
func (h *Handler) RegisterOperator(internal *gin.RouterGroup) {
	internal.Group("/tenants").POST("/:id/teardown", h.teardown)
}

func (h *Handler) teardown(c *gin.Context) {
	var req teardownRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.StoreSlugs == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "store_slugs is required; send [] to assert the tenant has no stores",
		})
		return
	}

	res, err := h.svc.PurgeTenant(c.Request.Context(), c.Param("id"), *req.StoreSlugs)
	if err != nil {
		var me *MismatchError
		if errors.As(err, &me) {
			c.JSON(http.StatusConflict, gin.H{
				"error":    "confirmation_mismatch",
				"message":  "supplied store_slugs do not match the tenant's current stores",
				"expected": me.Expected,
			})
			return
		}
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"tenant_id":   res.TenantID,
		"tenant_name": res.TenantName,
		"store_ids":   res.StoreIDs,
		"store_slugs": res.StoreSlugs,
	}})
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
