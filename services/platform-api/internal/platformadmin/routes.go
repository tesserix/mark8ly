// Package platformadmin mounts platform-api's half of the Tesserix
// console's HMAC-signed contract surface (#720 Task 3). It consumes
// github.com/mark8ly/platformauth for signature verification, replay
// defence and the mount-prefix constant — the same module marketplace-api's
// own platformadmin surface (services/marketplace-api/internal/handlers/
// platformadmin) is built on — rather than reimplementing any of the
// signing scheme here.
//
// This package intentionally starts small: a single conformance/health
// read route proving the auth chain works end to end. Task 5 adds the
// email-template routes; this file's Register and Deps are shaped so that
// task only has to add fields and a mount guard, not restructure this one.
package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/audit"
	"github.com/mark8ly/platformauth"
)

// MountPrefix is re-exported from platformauth so callers in cmd/server
// mount this group without importing platformauth directly. See the
// Register doc comment below for why it must be this value and never
// /api/v1/admin.
const MountPrefix = platformauth.MountPrefix

// Deps groups everything this surface needs. Constructed in
// cmd/server/main.go.
type Deps struct {
	DB     *gorm.DB
	Logger *slog.Logger

	// Secret is PLATFORM_API_PLATFORM_ADMIN_SECRET. Empty leaves the
	// surface mounted but inert — every request answers 503
	// not_configured, matching marketplace-api's platformadmin surface.
	// That is what lets this binary ship before the secret exists.
	Secret string

	// NonceStore is optional; a Postgres-backed one is built from DB when
	// nil, matching marketplace-api's pattern.
	NonceStore platformauth.NonceStore

	// Emitter is platform-api's existing fire-and-forget audit client
	// (internal/audit.Client), which posts events to marketplace-api's
	// /internal/audit-events ingest. It is not read by Register today —
	// no write route exists on this surface yet — but it is plumbed
	// through here so a future write route (Task 5) has an audit path to
	// gate on immediately, matching marketplace-api's discipline: a write
	// endpoint that cannot be attributed to an operator must not mount
	// (see Deps.Emitter's doc comment in marketplace-api's routes.go). A
	// nil Emitter must gate any future write route's mounting the same
	// way; it does not gate the read route this task adds.
	Emitter *audit.Client
}

// Register mounts the platform console's surface behind
// RequirePlatformAuth. Only a conformance health read is mounted today —
// see the package doc comment.
//
// Callers always mount this under /api/v1/platform (MountPrefix; see
// cmd/server/main.go) — never under /api/v1/admin, for the same reason
// marketplace-api's platformadmin surface documents at
// services/marketplace-api/internal/handlers/platformadmin/routes.go: an
// Istio AuthorizationPolicy in istio-ingress (`require-customer-auth`)
// denies any request without a valid JWT to a fixed list of prefixes that
// includes /api/v1/admin/*. This surface authenticates by HMAC signature,
// not JWT, so mounting it under /api/v1/admin/* gets every request
// rejected with 403 "RBAC: access denied" at the mesh, before it ever
// reaches this package. That is invisible locally and in CI since Istio
// isn't part of either. Do not "tidy" this back onto /api/v1/admin.
func Register(g *gin.RouterGroup, deps Deps) {
	nonces := deps.NonceStore
	if nonces == nil && deps.DB != nil {
		nonces = platformauth.NewNonceStore(deps.DB)
	}

	group := g.Group("", platformauth.RequirePlatformAuth(platformauth.AuthConfig{
		Secret:     deps.Secret,
		NonceStore: nonces,
		Logger:     deps.Logger,
	}))

	// Mounted unconditionally (no dependency to be nil-safe against): it
	// needs only the DB handle already required to build NonceStore above,
	// and a nil DB degrades its answer rather than failing to mount — see
	// health.go.
	NewHealthHandler(deps.DB, deps.Logger).Register(group)
}
