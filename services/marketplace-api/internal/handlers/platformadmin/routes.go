package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/stores"
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

	// TenantLifecycle serves POST /admin/tenants/:id/{suspend,unsuspend}
	// (#287) — this surface's first WRITE endpoints. Nil leaves those
	// routes unmounted, matching the nil-safe pattern used for the other
	// client-backed routes above. Requires DB and Emitter (both non-nil)
	// too — see Emitter below for why Emitter is required here and nowhere
	// else in this struct.
	TenantLifecycle TenantLifecycle

	// Emitter is the async audit-log writer used for write-endpoint audit
	// rows (currently only tenant suspend/unsuspend). Unlike every other
	// optional dependency in this struct, a nil Emitter does NOT leave the
	// surface merely degraded — Register refuses to mount the
	// TenantLifecycle routes at all when Emitter is nil (see the guard
	// below), because a write endpoint that cannot be attributed to an
	// operator should not exist, not run silently unaudited. (The
	// underlying EmitOperatorAction/Emit machinery does still tolerate a
	// nil *audit.Emitter as a fire-and-forget no-op — logged loudly rather
	// than silent, since #287 — for callers reached other than through
	// this Register guard.)
	Emitter *audit.Emitter

	// TenantGateInvalidator drops a tenant's cached status so a suspension
	// takes effect immediately instead of at the next cache refresh (#287
	// fix-round-1). Unlike Emitter above, nil here is a genuine no-op, not
	// a mount guard: an unwired invalidator just leaves today's TTL-lag
	// behaviour in place (a delay, never a lost audit record), so it does
	// NOT gate whether the TenantLifecycle routes mount.
	TenantGateInvalidator TenantGateInvalidator

	// Tickets serves /admin/tickets (#329), the cross-store support ticket
	// read. Nil leaves that route unmounted, matching the nil-safe pattern
	// used for the other optional client-backed routes above.
	Tickets TicketLister

	// Notifications serves /admin/notifications (#332), the cross-store
	// in-app notification log. Nil leaves that route unmounted, matching
	// the nil-safe pattern used for the other optional client-backed
	// routes above.
	//
	// This is the notification BELL, not a sent-mail log — no record of
	// outbound email exists anywhere in this estate. See #348.
	Notifications NotificationLister
}

// TenantGateInvalidator drops a tenant's cached admin-gate status. Declared
// here (not importing internal/tenantgate's concrete type) so this package
// stays decoupled from the admin group's gate implementation — the same
// reason TenantDirectory, TenantLifecycle etc. are declared as local
// interfaces above.
type TenantGateInvalidator interface {
	Invalidate(tenantID string)
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
	// routes below. The source's nil-DB guard (errNoDB -> `unknown`) is
	// defensive: it is not reachable in the main.go wiring, because neither
	// call site sets NonceStore, so a nil DB leaves it nil and
	// RequirePlatformAuth answers 503 not_configured before any handler
	// runs. It IS reachable by a caller that supplies its own NonceStore
	// alongside a nil DB — Deps.NonceStore is exported and documented
	// optional — and by anyone constructing the source directly, where a
	// nil *gorm.DB would otherwise panic on WithContext.
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

	if deps.Tickets != nil {
		NewTicketsHandler(deps.DB, deps.Tickets, deps.Logger).Register(group)
	}

	if deps.Notifications != nil {
		NewNotificationsHandler(deps.DB, deps.Notifications, deps.Logger).Register(group)
	}

	switch {
	case deps.TenantLifecycle != nil && deps.DB != nil && deps.Emitter != nil:
		NewTenantLifecycleHandler(
			deps.TenantLifecycle,
			stores.NewRepository(deps.DB),
			NewOperatorActionAuditFunc(deps.Emitter),
			deps.TenantGateInvalidator,
			deps.Logger,
		).Register(group)
	case deps.TenantLifecycle != nil:
		// TenantLifecycle is wired but DB or Emitter isn't: mounting a
		// write endpoint that cannot be attributed is worse than not
		// having it (#287, F1) — this is not the nil-safe "just degraded"
		// pattern the other routes above use.
		if deps.Logger != nil {
			deps.Logger.Warn("platformadmin: tenant lifecycle routes not mounted — DB and Emitter are both required",
				"db_nil", deps.DB == nil, "emitter_nil", deps.Emitter == nil)
		}
	}
}
