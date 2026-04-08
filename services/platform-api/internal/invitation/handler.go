package invitation

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for invitation endpoints.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts invitation routes onto the given gin.RouterGroups.
//
// Public routes (unauthenticated callers, e.g. the accept page):
//   - GET  /api/v1/invitations/verify?token=…
//   - POST /api/v1/invitations/accept
//
// Internal routes (trusted in-cluster callers, i.e. admin BFF):
//   - POST   /internal/tenants/:id/invitations
//   - GET    /internal/tenants/:id/invitations
//   - DELETE /internal/tenants/:id/invitations/:inv_id
//   - GET    /internal/tenants/:id/members
func (h *Handler) Register(public *gin.RouterGroup, internal *gin.RouterGroup) {
	pub := public.Group("/invitations")
	{
		pub.GET("/verify", h.verify)
		pub.POST("/accept", h.accept)
	}

	inv := internal.Group("/tenants/:id/invitations")
	{
		inv.POST("", h.create)
		inv.GET("", h.listPending)
		inv.DELETE("/:inv_id", h.revoke)
	}
	internal.GET("/tenants/:id/members", h.listMembers)
}

func (h *Handler) listMembers(c *gin.Context) {
	rows, err := h.svc.ListMembers(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

type createRequest struct {
	Email   string `json:"email"`
	Role    string `json:"role"`
	UID     string `json:"uid"`
	StoreID string `json:"store_id,omitempty"`
}

func (h *Handler) create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	inv, err := h.svc.Create(c.Request.Context(), CreateInput{
		TenantID:        c.Param("id"),
		StoreID:         req.StoreID,
		Email:           req.Email,
		Role:            req.Role,
		InvitedByUserID: req.UID,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": inv})
}

func (h *Handler) listPending(c *gin.Context) {
	rows, err := h.svc.ListPending(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

type revokeRequest struct {
	UID string `json:"uid"`
}

func (h *Handler) revoke(c *gin.Context) {
	var req revokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), c.Param("id"), c.Param("inv_id"), req.UID); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"revoked": true}})
}

func (h *Handler) verify(c *gin.Context) {
	res, err := h.svc.Verify(c.Request.Context(), c.Query("token"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}

type acceptRequest struct {
	Token         string `json:"token"`
	UID           string `json:"uid"`
	VerifiedEmail string `json:"verified_email"`
}

func (h *Handler) accept(c *gin.Context) {
	var req acceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	res, err := h.svc.Accept(c.Request.Context(), AcceptInput{
		Token:         req.Token,
		UID:           req.UID,
		VerifiedEmail: req.VerifiedEmail,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}

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
