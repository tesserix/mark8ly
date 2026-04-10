// Package httpserver provides Gin setup with the conventions used across
// mark8ly services. marketplace-api runs two engines — admin and
// storefront — on the same port in MODE=both (local dev) or on one port
// per Knative Service in prod.
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/middleware"
	"github.com/mark8ly/marketplace-api/internal/mode"
)

// Engines holds the Gin engines constructed for the current mode.
// Admin is nil when mode does not run admin; same for Storefront.
type Engines struct {
	Admin      *gin.Engine
	Storefront *gin.Engine
}

// New constructs the Gin engines appropriate for the given mode. Shared
// /health and /ready handlers are mounted on every active engine. Request
// logging and panic recovery are applied uniformly.
func New(env string, m mode.Mode, log *slog.Logger) Engines {
	if env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	build := func(label string) *gin.Engine {
		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(middleware.SecurityHeaders())
		r.Use(requestLogger(log.With(slog.String("engine", label))))
		return r
	}

	var e Engines
	if m.RunsAdmin() {
		e.Admin = build("admin")
	}
	if m.RunsStorefront() {
		e.Storefront = build("storefront")
	}
	return e
}

// MergedForBoth returns a single engine hosting both admin and storefront
// route groups. Used only in MODE=both for local dev convenience where we
// listen on a single port. In production each mode runs its own process
// on its own Knative Service, so MergedForBoth is not called.
func MergedForBoth(env string, log *slog.Logger) *gin.Engine {
	if env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())
	r.Use(requestLogger(log))
	return r
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("dur", time.Since(start)),
		)
	}
}

// OK returns a 200 JSON response with a canonical body. Use for /health.
func OK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
