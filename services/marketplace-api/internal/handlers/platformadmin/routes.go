package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// Deps groups everything the platform admin surface needs. Constructed in
// cmd/marketplace-api/main.go.
type Deps struct {
	DB     *gorm.DB
	Repo   audit.Repository
	Logger *slog.Logger

	// Secret is MARKETPLACE_PLATFORM_ADMIN_SECRET. Empty leaves the surface
	// mounted but inert — every request answers 503 not_configured. That is
	// what lets the binary ship before the secret exists.
	Secret string

	// NonceStore is optional; a Postgres-backed one is built from DB when nil.
	NonceStore NonceStore

	// TenantDirectory serves /admin/entities/tenants (#277). Nil leaves those
	// routes unmounted, matching the nil-safe pattern used for Repo above.
	TenantDirectory TenantDirectory

	// OnboardingFunnel serves /admin/onboarding/funnel and /sessions (#283).
	// Nil leaves those routes unmounted, matching the nil-safe pattern used
	// for TenantDirectory above.
	OnboardingFunnel OnboardingFunnel

	// EstateCounts and Subscriptions, together with OnboardingFunnel above,
	// serve /admin/kpis (#282). All three must be non-nil for that route to
	// mount — a partial KPI handler is worse than no handler at all.
	EstateCounts  EstateCounts
	Subscriptions Subscriptions

	// Trials serves /admin/billing/trials (#285), alongside TenantDirectory
	// above for the tenant-name lookup. Both must be non-nil for that route
	// to mount, matching the EstateCounts/Subscriptions/OnboardingFunnel
	// pattern above.
	Trials TrialLister

	// AllSubscriptions serves /admin/billing/subscriptions (#284), alongside
	// TenantDirectory above for the tenant-name lookup. Both must be
	// non-nil for that route to mount, matching the Trials pattern above.
	AllSubscriptions SubscriptionLister
}

// Register mounts the platform console's /admin/* surface behind
// RequirePlatformAuth. A nil Repo leaves everything unmounted, matching the
// nil-safe pattern used for optional handlers in internal/handlers/admin.
//
// Callers always mount this under /api/v1/platform (see cmd/marketplace-api/
// main.go) — never under /api/v1/admin, deliberately, for two reasons:
//
//  1. The mesh. An Istio AuthorizationPolicy in the cluster,
//     `require-customer-auth` (namespace istio-ingress, repo tesserix-k8s),
//     denies any request without a valid JWT to a fixed list of prefixes
//     that includes /api/v1/admin/*. This surface authenticates by HMAC
//     signature, not JWT, so mounting it under /api/v1/admin/* gets every
//     request rejected with 403 "RBAC: access denied" at the mesh — before
//     it ever reaches this package. See docs/architecture.md, "Gateway JWT
//     gate (istio-ingress)", for the full writeup. This is invisible
//     locally and in CI since Istio isn't part of either.
//  2. The router. The merchant admin tree already registers
//     /admin/tenants/:tenantId/... under a wildcard name that a later
//     platform endpoint would collide with under a different wildcard name
//     at the same path position — gin panics at router build time when
//     that happens.
//
// Do not "tidy" this back onto /api/v1/admin.
func Register(g *gin.RouterGroup, deps Deps) {
	if deps.Repo == nil {
		return
	}

	nonces := deps.NonceStore
	if nonces == nil && deps.DB != nil {
		nonces = NewNonceStore(deps.DB)
	}

	group := g.Group("", RequirePlatformAuth(AuthConfig{
		Secret:     deps.Secret,
		NonceStore: nonces,
		Logger:     deps.Logger,
	}))

	NewAuditLogsHandler(deps.DB, deps.Repo, deps.Logger).Register(group)

	// Health needs only the DB, so it mounts alongside the surface itself
	// rather than behind a nil-dependency guard like the client-backed
	// routes below. A nil DB is handled inside the source, which returns
	// errNoDB from every check rather than dereferencing a nil *gorm.DB;
	// the handler renders that as `unknown` — the honest non-answer, and
	// never a fabricated ok.
	NewHealthHandler(NewDBHealthSource(deps.DB), deps.Logger).Register(group)

	if deps.TenantDirectory != nil {
		NewEntitiesTenantsHandler(deps.TenantDirectory, deps.Logger).Register(group)
		NewConversionsHandler(deps.TenantDirectory, deps.Logger).Register(group)
	}

	if deps.OnboardingFunnel != nil {
		NewOnboardingFunnelHandler(deps.OnboardingFunnel, deps.Logger).Register(group)
	}

	if deps.EstateCounts != nil && deps.OnboardingFunnel != nil && deps.Subscriptions != nil {
		NewKPIsHandler(deps.EstateCounts, deps.OnboardingFunnel, deps.Subscriptions, deps.DB, deps.Logger).Register(group)
	}

	if deps.Trials != nil && deps.TenantDirectory != nil {
		NewBillingTrialsHandler(deps.Trials, deps.TenantDirectory, deps.DB, nil, deps.Logger).Register(group)
	}

	if deps.AllSubscriptions != nil && deps.TenantDirectory != nil {
		NewBillingSubscriptionsHandler(deps.AllSubscriptions, deps.TenantDirectory, deps.DB, deps.Logger).Register(group)
	}
}
