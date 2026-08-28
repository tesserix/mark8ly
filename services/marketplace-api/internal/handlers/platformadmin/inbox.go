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

// inboxListResponse is fixed by the Product Admin Integration Contract's
// "items-total" envelope for this endpoint (design-system
// packages/admin-conformance/src/contract.ts:29-36). The conformance check
// requires the body to have EXACTLY these two top-level keys — a missing key
// fails, and so does an extra one, because an extra key is how a second,
// undeclared envelope starts and the console has no way to know which one to
// read (packages/admin-conformance/src/assertions/envelope.ts:64-89).
//
// That leaves no room in the body for Degraded, which names the inbox kinds
// that could not be reached. It is NOT dropped: dropping it silently would
// let a partially-failed aggregation read as a healthy short queue, which is
// worse than an error. Instead it is carried on the response header
// X-Inbox-Degraded (comma-separated kind names, set only when non-empty). If
// you came here looking for "degraded" in the JSON body, it isn't there by
// contract — read the header instead.
type inboxListResponse struct {
	Items []inbox.Item `json:"items"`
	Total int64        `json:"total"`
}

// InboxDegradedHeader is the header name that replaces the old body-level
// "degraded" field. See inboxListResponse for why it moved. Exported so
// callers (including tests in the external platformadmin_test package) don't
// hardcode the header name.
const InboxDegradedHeader = "X-Inbox-Degraded"

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
	items := make([]inbox.Item, 0, len(res.Items))
	items = append(items, res.Items...)

	if len(res.Degraded) > 0 {
		c.Header(InboxDegradedHeader, strings.Join(res.Degraded, ","))
	}

	c.JSON(http.StatusOK, inboxListResponse{
		Items: items,
		Total: res.Total,
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
