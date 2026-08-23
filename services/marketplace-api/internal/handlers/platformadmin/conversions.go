package platformadmin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// ConversionsHandler answers the CRM's question "has this lead email
// converted to a tenant?" for the platform console (#279). It owns only the
// wire shape; platform-api owns the owner-email lookup and its
// normalisation.
type ConversionsHandler struct {
	dir    TenantDirectory
	logger *slog.Logger
}

// NewConversionsHandler constructs the handler. logger may be nil.
func NewConversionsHandler(dir TenantDirectory, logger *slog.Logger) *ConversionsHandler {
	return &ConversionsHandler{dir: dir, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *ConversionsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/conversions", h.get)
}

// conversionResponse is the CRM's answer for one lead email.
//
// The zero-value response is {"state":"none"} — every other field is
// omitempty, so a miss cannot leak an empty ref the console would render.
type conversionResponse struct {
	State      string `json:"state"`
	Ref        string `json:"ref,omitempty"`
	Label      string `json:"label,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
}

func (h *ConversionsHandler) get(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "validation_error", "message": "email is required",
		})
		return
	}

	// Deliberately not lowercased here: platform-api's repository owns the
	// normalisation that backs its unique index. A second normalisation
	// here could drift from that one and silently change matching.
	t, err := h.dir.FindByOwnerEmail(c.Request.Context(), email)
	if err != nil {
		switch {
		case errors.Is(err, tenantdirectory.ErrNotFound):
			// A miss is a definite, positive answer ("this email has not
			// converted"), not the absence of a route. Reporting it as 404
			// would be indistinguishable on the wire from a route that
			// doesn't exist, so it must come back 200 with an explicit
			// state instead.
			c.JSON(http.StatusOK, conversionResponse{State: "none"})
		case errors.Is(err, tenantdirectory.ErrUnavailable):
			if h.logger != nil {
				h.logger.Error("tenant directory upstream unavailable", "err", err)
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "upstream_unavailable", "message": "tenant directory is unavailable",
			})
		default:
			if h.logger != nil {
				h.logger.Error("tenant directory", "err", err)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "internal_error", "message": "could not check conversion status",
			})
		}
		return
	}

	c.JSON(http.StatusOK, conversionResponse{
		State: "converted",
		// Bare id. The platform API namespaces as <slug>:<id> on arrival;
		// prefixing here yields "mark8ly:mark8ly:...".
		Ref:        t.ID,
		Label:      t.Name,
		ObservedAt: t.CreatedAt.UTC().Format(time.RFC3339),
	})
}
