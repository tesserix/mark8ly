// Package platformadmin mounts platform-api's half of the Tesserix
// console's HMAC-signed contract surface (#720 Task 3). It consumes
// github.com/mark8ly/platformauth for signature verification, replay
// defence and the mount-prefix constant — the same module marketplace-api's
// own platformadmin surface (services/marketplace-api/internal/handlers/
// platformadmin) is built on — rather than reimplementing any of the
// signing scheme here.
//
// This package started small: a single conformance/health read route
// proving the auth chain works end to end. Task 5 (mark8ly#720) adds the
// email-template routes — welcome, email_verification, invitation,
// password_reset, login_otp, new_device_login — mirroring the shape
// marketplace-api's own platformadmin surface already serves for its half
// of the same registry (orderdoc_*, giftcard_delivery, the billing keys).
package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/audit"
	"github.com/mark8ly/platform-api/internal/emailtemplates"
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
	// /internal/audit-events ingest. It does NOT gate the email-template
	// write below, even though the surface's general rule is "a write
	// that cannot be attributed to an operator must not mount" — see the
	// EmailTemplates/EmailTemplateRegistry doc comment below for why
	// Emitter specifically cannot carry that attribution for this route,
	// which is the same reason marketplace-api's own routes.go declines
	// to require an Emitter for its email-templates PUT. It remains
	// plumbed through here for whatever future write route on this
	// surface deals in tenant-scoped changes, where it DOES apply.
	Emitter *audit.Client

	// EmailTemplates, EmailTemplateRegistry and EmailTemplateTestSender
	// together serve /admin/email-templates (mark8ly#720 Task 5) — the
	// auth/onboarding half of the registry (welcome, email_verification,
	// invitation, password_reset, login_otp, new_device_login).
	//
	// Both EmailTemplates and EmailTemplateRegistry must be non-nil for any
	// route to mount, matching marketplace-api's pairing: the registry is
	// the only source for the fixed six-key list this handler treats as
	// "registered" (see emailtemplates.Registry.RegisteredKeys), so
	// mounting the read without it would report an incomplete list without
	// looking incomplete.
	//
	// EmailTemplateTestSender may be nil: the test-send route still mounts
	// and answers 503 not_configured. In production it never actually is
	// nil, because internal/notification.NewFromConfig never returns a nil
	// Sender (it falls back to a LogSender in dev) — see
	// emailtemplates.NewNotificationTestSender's doc comment for what that
	// means for a test-send in an environment with no provider key.
	EmailTemplates          EmailTemplateStore
	EmailTemplateRegistry   EmailTemplateRegistry
	EmailTemplateTestSender emailtemplates.TestSender
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

	if deps.EmailTemplates != nil && deps.EmailTemplateRegistry != nil {
		// The PUT mounts only with a DB, exactly matching
		// marketplace-api's EmailTemplates gate (services/marketplace-api/
		// internal/handlers/platformadmin/routes.go) and for the SAME
		// reason, not the surface's general Emitter rule:
		//
		// audit.Client.Emit requires a non-empty TenantID and no-ops with a
		// logged warning otherwise (internal/audit/client.go) — the same
		// constraint marketplace-api's audit_logs table enforces with a
		// NOT NULL column. An email template key is estate-wide, so there
		// is no tenant to supply either way. Requiring Emitter here would
		// gate this route on a dependency that, once past the gate, could
		// never actually attribute the write — a guard that guards
		// nothing, which is worse than an honest one.
		//
		// What actually attributes the write is the revision row
		// emailtemplates.Store.Upsert inserts on the SAME transaction as
		// the change (migration 0018): a failed insert rolls the change
		// back. That needs a database, so the database is the gate.
		NewEmailTemplatesHandler(
			deps.EmailTemplates, deps.EmailTemplateRegistry,
			deps.EmailTemplateTestSender, deps.DB != nil, deps.DB, deps.Logger,
		).Register(group)
		if deps.DB == nil && deps.Logger != nil {
			deps.Logger.Warn("platformadmin: email template write route not mounted — DB is required to record the change against an operator")
		}
	}
}
