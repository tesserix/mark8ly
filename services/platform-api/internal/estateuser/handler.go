package estateuser

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler serves the estate staff directory over HTTP.
type Handler struct{ repo Repository }

// NewHandler constructs the handler.
func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

// Register mounts GET /users on the supplied group.
//
// The CALLER gates it — main.go wraps this in RequireInternalAuthStrict,
// because this route returns every staff identity on the platform and must
// not be reachable on an unconfigured deploy. Same reasoning as
// tenant.RegisterDirectory (#277).
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/users", h.list)
}

// list serves GET /internal/users. Standard envelope: data + pagination.
// An empty result is 200 with data: [], never null.
func (h *Handler) list(c *gin.Context) {
	f := parseFilter(c)
	res, err := h.repo.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "an unexpected error occurred",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": res.Users,
		"pagination": gin.H{
			"page":  max(f.Page, 1),
			"limit": effectiveLimit(f.Limit),
			"total": res.Total,
		},
	})
}

// parseFilter never errors: a missing or malformed parameter takes the
// default, matching parseFunnelFilter in internal/onboarding.
func parseFilter(c *gin.Context) Filter {
	f := Filter{Q: strings.TrimSpace(c.Query("q")), Page: 1, Limit: DefaultPageSize}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	return f
}

// effectiveLimit reports the limit actually applied, so total/limit is a
// correct page count even when the caller asked for more than the ceiling.
func effectiveLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	default:
		return limit
	}
}
