package estate

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for estate-wide counts.
type Handler struct {
	repo Repository
}

// NewHandler constructs a Handler.
func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// Register mounts GET /estate/counts onto the given gin.RouterGroup. The
// CALLER is responsible for gating it — main.go wraps it in the strict
// internal group (middleware.RequireInternalAuthStrict), because these
// counts are estate-wide reads and must not be reachable on an
// unconfigured deploy.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/estate/counts", h.get)
}

func (h *Handler) get(c *gin.Context) {
	counts, err := h.repo.Get(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": counts})
}

// respondError mirrors internal/tenant/handler.go's error-mapping
// convention: a typed apperrors.AppError maps to its own status/code/
// message, everything else collapses to a generic 500 so internals never
// leak into the response.
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
