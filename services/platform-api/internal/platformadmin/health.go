package platformadmin

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Health status values. Deliberately just three today — "unknown" exists
// so a nil DB (never wired) and a DB that failed to answer (wired but
// broken) are distinguishable from a healthy one, matching the
// ok/degraded/unknown vocabulary marketplace-api's platformadmin health
// handler uses, without borrowing its marketplace-specific dependency
// registry (outbox, CSV jobs, Stripe webhooks) — platform-api has none of
// those and inventing entries for them here would just be padding.
const (
	statusOK       = "ok"
	statusDegraded = "degraded"
	statusUnknown  = "unknown"
)

// HealthHandler serves GET /admin/health — the conformance route this task
// exists to prove: that a request signed with the console's scheme reaches
// an authenticated handler on this surface end to end. It is intentionally
// minimal; see the package doc comment for what a later task adds.
type HealthHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewHealthHandler constructs the handler. db and logger may both be nil;
// a nil db reports statusUnknown rather than panicking.
func NewHealthHandler(db *gorm.DB, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{db: db, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *HealthHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/health", h.health)
}

func (h *HealthHandler) health(c *gin.Context) {
	status := statusUnknown
	if h.db != nil {
		status = statusOK
		sqlDB, err := h.db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			status = statusDegraded
			if h.logger != nil {
				h.logger.Error("platformadmin: health check could not reach the database", "err", err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"status":     status,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}})
}
