// Package health owns /health and /ready handlers.
//
// /health  — liveness. Returns 200 if the process is up. No DB check.
// /ready   — readiness. Returns 200 only when the DB is reachable via a
//             cheap `SELECT 1`. Used by Knative and the dev docker-compose
//             healthcheck.
package health

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the health/ready HTTP handler.
type Handler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// New constructs a Handler bound to the given *gorm.DB. db may be nil in
// tests that only exercise /health. logger is optional — nil falls back to
// slog.Default().
func New(db *gorm.DB) *Handler {
	return &Handler{db: db, logger: slog.Default()}
}

// WithLogger attaches a structured logger so ready-probe failures can be
// emitted server-side instead of being echoed back to the caller.
func (h *Handler) WithLogger(l *slog.Logger) *Handler {
	if l != nil {
		h.logger = l
	}
	return h
}

// Register mounts /health and /ready on the given engine.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/health", h.health)
	r.GET("/ready", h.ready)
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ready returns a coarse-grained db status without leaking DSN fragments,
// DB driver error text, or other implementation detail back to the caller.
// The detailed error is logged server-side so ops can still triage from
// the pod logs.
func (h *Handler) ready(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable"})
		return
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		h.logger.Error("readiness: db handle unavailable", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable"})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		h.logger.Error("readiness: db ping failed", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unreachable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
