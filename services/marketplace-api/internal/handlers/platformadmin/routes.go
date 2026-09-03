package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
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
	// This is the notification BELL, not a sent-mail log — no
	// delivery-outcome record exists anywhere in this estate. Two partial
	// per-email handoff records do exist (campaign_recipients.status,
	// shipments.dispatched_email_sent_at), but neither reports delivery,
	// bounce or drop. See #348.
	Notifications NotificationLister

	// Outbox serves /admin/outbox (#331), the cross-tenant read of stuck and
	// failed outbox events. Nil leaves that route unmounted, matching the
	// nil-safe pattern used for the other optional read routes above.
	//
	// This became answerable only with #336: before it, nothing wrote
	// outbox_events.error, so the `failed` status could never have matched a
	// row and this endpoint would have reported a permanently empty set
	// while looking as though it worked.
	Outbox OutboxLister

	// OutboxWriter serves the requeue/dead-letter writes on top of Outbox
	// above: POST /admin/outbox/:id/requeue, POST /admin/outbox/requeue
	// (batch), and POST /admin/outbox/:id/dead-letter (#405). Nil leaves
	// those three routes unmounted while List (#331) keeps working — the
	// nil-safe pattern used across this surface. Like TrialExtender, it
	// ALSO requires DB and Emitter to be non-nil for the write routes to
	// mount: a write endpoint that cannot be attributed to an operator
	// should not exist rather than run silently unaudited (#287, F1).
	OutboxWriter OutboxWriter

	// TrialExtender serves POST /admin/billing/trials/:storeID/extend (#286),
	// this surface's second WRITE. Like TenantLifecycle it needs DB and
	// Emitter as well: a write endpoint that cannot be attributed to an
	// operator should not exist rather than run silently unaudited (#287,
	// F1) — mounting is hard-gated on both being non-nil, matching
	// TenantLifecycle exactly, not the nil-safe "just degraded" pattern
	// used for the read routes in this struct.
	TrialExtender TrialExtender

	// TenantTeardown and Purger together serve POST /admin/tenants/:id/purge
	// and GET /admin/tenants/:id/purge/preview (#288) — the surface's
	// IRREVERSIBLE endpoint. Both must be non-nil, along with DB, Emitter
	// and TenantDirectory, for either route to mount.
	//
	// Emitter is required for the same reason it is required by
	// TenantLifecycle, and more so: a purge that cannot be audited is an
	// irreversible destruction with no record, which is the exact gap this
	// series exists to close. An unmounted route is the right failure.
	TenantTeardown TenantTeardown
	Purger         Purger

	// Inbox serves /admin/inbox (#280). Nil leaves the route unmounted,
	// matching the nil-safe pattern used for TenantDirectory above.
	Inbox InboxAggregator

	// InboxItems and InboxActionExecutors together serve
	// POST /admin/inbox/:kind/:id/actions/:actionId (#281a).
	//
	// InboxItems must be non-nil for the route to mount: without a
	// single-item read there is no way to check an action against the item's
	// own declared actions, and validating against the executor registry
	// instead would turn that declaration back into documentation.
	//
	// An EMPTY executor list still mounts. The route then answers 501 per
	// kind, which is the honest state for a queue that is readable but not
	// yet actionable — and it is the state most kinds are in.
	InboxItems           InboxItemSource
	InboxActionExecutors []InboxActionExecutor

	// InboxActionIdem records idempotency keys for destructive actions. A
	// Postgres-backed one is built from DB when nil, matching NonceStore.
	InboxActionIdem InboxActionIdempotency

	// EmailSends serves /admin/email-sends (#348D), the cross-tenant outbound
	// mail log. Nil leaves the route unmounted, matching the nil-safe pattern
	// used for Outbox above.
	EmailSends EmailSendLister

	// EstateUsers serves /admin/entities/users (#278). Nil leaves the route
	// unmounted, matching the nil-safe pattern used for TenantDirectory.
	//
	// Mounting this ALSO requires declaring `users` under entities in
	// admin-conformance.json AND in the chart's declaration in tesserix-k8s.
	// See docs/admin-conformance.md for why that declaration is required and
	// what this surface can and cannot declare; conformance_declaration_test.go
	// is the guard that catches the two drifting apart (#415).
	EstateUsers EstateUserDirectory

	// BreakGlass serves GET /admin/break-glass (#333), the cross-tenant
	// emergency-account inventory. Nil leaves the route unmounted, matching
	// the nil-safe pattern used for EmailSends above. It also requires
	// TenantDirectory (non-nil) for tenant-name enrichment, matching the
	// Trials/AllSubscriptions pattern above: both must be non-nil for the
	// route to mount.
	BreakGlass BreakGlassLister

	// BreakGlassRotator, BreakGlassWriter, BreakGlassRateLimiter and
	// BreakGlassIPHMACKey together serve the break-glass write endpoints
	// (#404): POST /admin/break-glass/:tenantId/{rotate,disable,enable}
	// and POST /admin/break-glass/clear-lockout. Nil BreakGlassRotator or
	// BreakGlassWriter leaves those four routes unmounted while List
	// (#333) keeps working — matching OutboxWriter's pattern. Like
	// OutboxWriter, this ALSO requires DB and Emitter to be non-nil for
	// the write routes to mount: a write endpoint that cannot be
	// attributed to an operator should not exist rather than run silently
	// unaudited (#287, F1). BreakGlassRateLimiter may be nil — the
	// handler degrades to clearing only the durable DB lock and logs
	// loudly rather than panicking (see BreakGlassWriteHandler's doc).
	BreakGlassRotator     BreakGlassRotator
	BreakGlassWriter      BreakGlassWriter
	BreakGlassRateLimiter BreakGlassRateLimiter
	BreakGlassIPHMACKey   breakglass.HMACKey

	// PriceCatalog resolves the plan catalog the two billing read routes
	// display amounts from (tesserix-home#328 phase C). Unlike every other
	// optional field in this struct, nil does NOT unmount anything: it
	// prices from the catalog compiled into internal/billing/pricing, which
	// is what those routes did before the cutover. That is the rollback —
	// unsetting CONSOLE_CATALOG_* in the chart reverts this surface to the
	// compiled amounts without a code change. See compiledPriceCatalog.
	PriceCatalog CatalogResolver
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

	// §8.8. Mounted beside health and for the same reason: it depends on
	// nothing. See LifecycleReasonCodesHandler.Register for why it is not
	// gated on the writes it describes.
	NewLifecycleReasonCodesHandler().Register(group)

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
		NewBillingTrialsHandler(deps.Trials, deps.TenantDirectory, deps.DB, nil, deps.PriceCatalog, deps.Logger).Register(group)
	}

	if deps.AllSubscriptions != nil && deps.TenantDirectory != nil {
		NewBillingSubscriptionsHandler(deps.AllSubscriptions, deps.TenantDirectory, deps.DB, deps.PriceCatalog, deps.Logger).Register(group)
	}

	if deps.Tickets != nil {
		NewTicketsHandler(deps.DB, deps.Tickets, deps.Logger).Register(group)
	}

	if deps.Notifications != nil {
		NewNotificationsHandler(deps.DB, deps.Notifications, deps.Logger).Register(group)
	}

	if deps.Outbox != nil {
		var writer OutboxWriter
		var auditFn outboxAuditFunc
		if deps.OutboxWriter != nil && deps.DB != nil && deps.Emitter != nil {
			writer = deps.OutboxWriter
			auditFn = NewOperatorActionAuditFunc(deps.Emitter)
		}
		NewOutboxHandler(deps.DB, deps.Outbox, writer, auditFn, deps.Logger).Register(group)
	}

	if deps.EmailSends != nil {
		NewEmailSendsHandler(deps.DB, deps.EmailSends, deps.Logger).Register(group)
	}

	if deps.BreakGlass != nil && deps.TenantDirectory != nil {
		NewBreakGlassHandler(deps.DB, deps.BreakGlass, deps.TenantDirectory, deps.Logger).Register(group)
	}

	switch {
	case deps.BreakGlassRotator != nil && deps.BreakGlassWriter != nil && deps.DB != nil && deps.Emitter != nil:
		NewBreakGlassWriteHandler(
			deps.DB, deps.BreakGlassWriter, deps.BreakGlassRotator,
			deps.BreakGlassRateLimiter, deps.BreakGlassIPHMACKey,
			NewOperatorActionAuditFunc(deps.Emitter), deps.Logger,
		).Register(group)
	case deps.BreakGlassRotator != nil || deps.BreakGlassWriter != nil:
		// A write endpoint that cannot be attributed is worse than not
		// having it (#287, F1) — not the nil-safe "just degraded" pattern
		// the read routes above use.
		if deps.Logger != nil {
			deps.Logger.Warn("platformadmin: break-glass write routes not mounted — BreakGlassRotator, BreakGlassWriter, DB and Emitter are all required",
				"rotator_nil", deps.BreakGlassRotator == nil, "writer_nil", deps.BreakGlassWriter == nil,
				"db_nil", deps.DB == nil, "emitter_nil", deps.Emitter == nil)
		}
	}

	if deps.EstateUsers != nil {
		NewEntitiesUsersHandler(deps.EstateUsers, deps.Logger).Register(group)
	}

	if deps.InboxItems != nil {
		idem := deps.InboxActionIdem
		if idem == nil && deps.DB != nil {
			idem = NewInboxActionIdempotency(deps.DB)
		}
		NewInboxActionsHandler(
			deps.InboxItems, deps.InboxActionExecutors, idem, deps.Emitter, deps.Logger,
		).Register(group)
	}

	if deps.Inbox != nil {
		NewInboxHandler(deps.Inbox, deps.Logger).Register(group)
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

	switch {
	case deps.TrialExtender != nil && deps.DB != nil && deps.Emitter != nil:
		NewBillingTrialExtendHandler(
			deps.DB, deps.TrialExtender,
			NewOperatorActionAuditFunc(deps.Emitter),
			deps.Logger,
		).Register(group)
	case deps.TrialExtender != nil:
		// TrialExtender is wired but DB or Emitter isn't: mounting a write
		// endpoint that cannot be attributed is worse than not having it
		// (#287, F1) — this is not the nil-safe "just degraded" pattern the
		// read routes above use. A trial extension is a billing decision
		// made against a merchant; an unattributed one should not be
		// reachable.
		if deps.Logger != nil {
			deps.Logger.Warn("platformadmin: trial extend route not mounted — DB and Emitter are both required",
				"db_nil", deps.DB == nil, "emitter_nil", deps.Emitter == nil)
		}
	}

	switch {
	case deps.TenantTeardown != nil && deps.Purger != nil && deps.Emitter != nil &&
		deps.DB != nil && deps.TenantDirectory != nil:
		NewTenantPurgeHandler(
			deps.TenantTeardown, deps.Purger, deps.TenantDirectory,
			NewOperatorActionSyncFunc(deps.Emitter), deps.TenantGateInvalidator, deps.Logger,
		).Register(group)
	case deps.TenantTeardown != nil || deps.Purger != nil:
		// TenantTeardown or Purger is wired but one of DB, Emitter or
		// TenantDirectory isn't: a handler that cannot audit an
		// irreversible destruction must not exist on this surface at all
		// (#287, F1, and more so here) — not the nil-safe "just degraded"
		// pattern the read routes above use.
		if deps.Logger != nil {
			deps.Logger.Warn("platformadmin: tenant purge routes not mounted — TenantTeardown, Purger, DB, Emitter and TenantDirectory are all required",
				"teardown_nil", deps.TenantTeardown == nil, "purger_nil", deps.Purger == nil,
				"db_nil", deps.DB == nil, "emitter_nil", deps.Emitter == nil,
				"tenant_directory_nil", deps.TenantDirectory == nil)
		}
	}
}
