package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// validInboxKinds names every registered provider kind, for the
// unknown_kind error response. Built from the exported Kind* constants so it
// cannot drift from the provider registry.
var validInboxKinds = []string{
	inbox.KindSEAManualReview,
	inbox.KindMigrationFastPath,
	inbox.KindErasureRequest,
	inbox.KindArbitrageAppeal,
	inbox.KindOnboardingStalled,
}

// InboxAggregator is the slice of inbox.Aggregator this handler needs.
type InboxAggregator interface {
	List(ctx context.Context, f inbox.Filter) (inbox.Result, error)
}

// InboxHandler answers GET /admin/inbox (#280).
type InboxHandler struct {
	agg    InboxAggregator
	logger *slog.Logger
}

// NewInboxHandler constructs the handler. logger may be nil.
func NewInboxHandler(agg InboxAggregator, logger *slog.Logger) *InboxHandler {
	return &InboxHandler{agg: agg, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *InboxHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/inbox", h.List)
}

// inboxListResponse is the house envelope plus Degraded.
//
// Degraded names the kinds that could not be reached. It is omitted when
// empty, so a healthy response is the same shape every other list endpoint
// returns.
type inboxListResponse struct {
	Data       []inbox.Item `json:"data"`
	Pagination pagination   `json:"pagination"`
	Degraded   []string     `json:"degraded,omitempty"`
}

// List handles GET /admin/inbox.
func (h *InboxHandler) List(c *gin.Context) {
	f := h.parseFilter(c)

	res, err := h.agg.List(c.Request.Context(), f)
	if err != nil {
		switch {
		case errors.Is(err, inbox.ErrPageTooDeep):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "page_too_deep",
				"message": "aggregate inbox pagination is bounded; narrow the request with ?kind=",
			})
		case errors.Is(err, inbox.ErrUnknownKind):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "unknown_kind",
				"message": "unknown kind; valid kinds are: " + strings.Join(validInboxKinds, ", "),
			})
		case errors.Is(err, inbox.ErrAllSourcesFailed):
			if h.logger != nil {
				h.logger.Error("platform inbox list", "err", err)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "internal server error",
			})
		default:
			if h.logger != nil {
				h.logger.Error("platform inbox list", "err", err)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "internal server error",
			})
		}
		return
	}

	// A nil slice marshals to null; the console renders an array.
	items := res.Items
	if items == nil {
		items = []inbox.Item{}
	}

	c.JSON(http.StatusOK, inboxListResponse{
		Data:       items,
		Pagination: pagination{Page: f.Page, Limit: f.Limit, Total: res.Total},
		Degraded:   res.Degraded,
	})
}

// parseFilter never returns an error. A missing parameter takes the
// default, an unparseable one is ignored, and limit is clamped to 100
// rather than refused.
func (h *InboxHandler) parseFilter(c *gin.Context) inbox.Filter {
	f := inbox.Filter{
		Kind:     c.Query("kind"),
		TenantID: c.Query("tenant_id"),
		Status:   c.Query("status"),
		Page:     1,
		Limit:    25,
	}
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		f.Page = v
	}
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > 100 {
			v = 100
		}
		f.Limit = v
	}
	return f
}
