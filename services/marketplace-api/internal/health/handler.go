// Package health owns /health and /ready handlers.
//
// /health  — liveness. Returns 200 if the process is up. No DB check.
// /ready   — readiness. Returns 200 only when the DB is reachable via a
//             cheap `SELECT 1`. Used by Knative and the dev docker-compose
//             healthcheck.
package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the health/ready HTTP handler.
type Handler struct {
	db *gorm.DB
}

// New constructs a Handler bound to the given *gorm.DB. db may be nil in
// tests that only exercise /health.
func New(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// Register mounts /health and /ready on the given engine.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/health", h.health)
	r.GET("/ready", h.ready)
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ready(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable"})
		return
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable", "error": err.Error()})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unreachable", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
