package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondInternal renders a generic 500 envelope. Typed error codes from
// apperrors are intentionally NOT leaked onto the wire for anonymous
// storefront traffic — the handler logs structured detail server-side
// and returns an opaque body to the caller.
func respondInternal(c *gin.Context, logger *slog.Logger, err error) {
	if logger != nil {
		logger.ErrorContext(c.Request.Context(), "storefront: internal error",
			slog.String("path", c.Request.URL.Path),
			slog.String("err", err.Error()),
		)
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}
