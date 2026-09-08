package ssousers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler exposes the two internal routes marketplace-api's SSO JIT needs.
type Handler struct{ svc *Service }

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts both routes on an /internal group.
//
// They belong behind the STRICT guard, not the permissive one that the other
// tenant-scoped routes use. The permissive guard no-ops on an empty secret,
// which is right for a read you had to know a tenant id to ask for — but
// Provision CREATES an account and GRANTS it a role on a tenant. An
// unconfigured deploy must refuse that rather than serve it to anything that
// reaches the pod.
func (h *Handler) Register(g *gin.RouterGroup) {
	t := g.Group("/tenants/:id/sso-users")
	{
		t.POST("/lookup", h.lookup)
		t.POST("", h.provision)
	}
}

type lookupRequest struct {
	Email string `json:"email" binding:"required"`
}

// lookup answers "is this email already a member of this tenant".
//
// found=false is a successful answer, not a 404: the caller's next step is to
// provision, and a 404 would read as "this route is wrong".
func (h *Handler) lookup(c *gin.Context) {
	var req lookupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}

	res, err := h.svc.Lookup(c.Request.Context(), c.Param("id"), req.Email)
	if err != nil {
		respondError(c, mapErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"user_id": res.UserID,
		"found":   res.Found,
	}})
}

type provisionRequest struct {
	Email     string `json:"email" binding:"required"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role" binding:"required"`
}

func (h *Handler) provision(c *gin.Context) {
	var req provisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}

	userID, err := h.svc.Provision(c.Request.Context(), ProvisionInput{
		TenantID:  c.Param("id"),
		Email:     req.Email,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Role:      req.Role,
	})
	if err != nil {
		respondError(c, mapErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"user_id": userID}})
}

// mapErr turns a service error into a status.
//
// ErrAmbiguousEmail is a 409 and deliberately not a 500: two accounts share
// the address, and which one an IdP assertion refers to is not a question this
// service may answer by picking. It needs a human.
func mapErr(err error) error {
	switch {
	case errors.Is(err, zitadeladmin.ErrAmbiguousEmail):
		return apperrors.Conflict("ambiguous_email",
			"more than one account uses this email address")
	case strings.Contains(err.Error(), "owner cannot be granted"),
		strings.Contains(err.Error(), "unknown role"):
		return apperrors.BadRequest("invalid_role", err.Error())
	default:
		return apperrors.Internal("sso_user_failed", err.Error())
	}
}

func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.AbortWithStatusJSON(ae.Status, gin.H{
			"error":   ae.Code,
			"message": ae.Message,
		})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
}
