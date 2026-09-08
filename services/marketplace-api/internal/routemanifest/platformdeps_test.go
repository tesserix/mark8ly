package routemanifest_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/emaillog"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/estatecounts"
	"github.com/mark8ly/marketplace-api/internal/estateuserdir"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/internal/ticket"
)

// ---------------------------------------------------------------------------
// platformadmin.Deps is the one surface in this manifest whose registrar
// cannot be driven by depsfill: Register gates almost every route on an
// INTERFACE field (see routes.go), and reflection cannot invent an
// implementation. So the stubs below exist for exactly one purpose — being
// non-nil at mount time.
//
// Nothing here is ever CALLED. Register only takes method values and passes
// them to gin; no request is ever served against this router. Every method
// therefore panics rather than returning a zero value, so a future test that
// does try to serve a request against this wiring fails loudly at the call
// instead of silently asserting against fabricated data.
//
// platformadmin's own test package has equivalent stubs, but they are
// unexported test-package types and unreachable from here. Duplicating them
// is acceptable in a way duplicating depsfill would not be: these have no
// logic to drift, and the compiler pins every signature the moment an
// interface changes.
// ---------------------------------------------------------------------------

const notCalled = "routemanifest: platformadmin dependency invoked — this wiring exists only to mount routes, never to serve a request"

type stubTenantDirectory struct{}

func (stubTenantDirectory) List(context.Context, tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	panic(notCalled)
}
func (stubTenantDirectory) Get(context.Context, string) (*tenantdirectory.TenantDetail, error) {
	panic(notCalled)
}
func (stubTenantDirectory) FindByOwnerEmail(context.Context, string) (*tenantdirectory.Tenant, error) {
	panic(notCalled)
}

type stubOnboardingFunnel struct{}

func (stubOnboardingFunnel) GetFunnel(context.Context, onboardingfunnel.FunnelParams) (*onboardingfunnel.FunnelStats, error) {
	panic(notCalled)
}
func (stubOnboardingFunnel) ListSessions(context.Context, onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	panic(notCalled)
}

type stubEstateCounts struct{}

func (stubEstateCounts) Get(context.Context) (*estatecounts.Counts, error) { panic(notCalled) }

type stubTenantLifecycle struct{}

func (stubTenantLifecycle) Suspend(context.Context, string) (*tenantlifecycle.Result, error) {
	panic(notCalled)
}
func (stubTenantLifecycle) Unsuspend(context.Context, string) (*tenantlifecycle.Result, error) {
	panic(notCalled)
}

type stubTenantTeardown struct{}

func (stubTenantTeardown) Teardown(context.Context, string, []string) (*tenantlifecycle.TeardownResult, error) {
	panic(notCalled)
}

type stubPurger struct{}

func (stubPurger) Purge(context.Context, string, []string) (tenantpurge.Report, error) {
	panic(notCalled)
}
func (stubPurger) Count(context.Context, string, []string) (tenantpurge.Report, error) {
	panic(notCalled)
}

type stubGateInvalidator struct{}

func (stubGateInvalidator) Invalidate(string) { panic(notCalled) }

type stubTicketLister struct{}

func (stubTicketLister) ListPlatform(context.Context, *gorm.DB, ticket.PlatformListFilter) (ticket.ListResult, error) {
	panic(notCalled)
}

type stubNotificationLister struct{}

func (stubNotificationLister) ListPlatform(context.Context, *gorm.DB, notification.PlatformListFilter) (notification.ListResult, error) {
	panic(notCalled)
}

type stubOutboxWriter struct{}

func (stubOutboxWriter) RequeueOne(context.Context, *gorm.DB, string) (outbox.RequeueResult, error) {
	panic(notCalled)
}
func (stubOutboxWriter) RequeueBatch(context.Context, *gorm.DB, []string) []outbox.RequeueOutcome {
	panic(notCalled)
}
func (stubOutboxWriter) DeadLetterOne(context.Context, *gorm.DB, string, string) (outbox.DeadLetterResult, error) {
	panic(notCalled)
}

type stubInboxAggregator struct{}

func (stubInboxAggregator) List(context.Context, inbox.Filter) (inbox.Result, error) {
	panic(notCalled)
}

type stubInboxItemSource struct{}

func (stubInboxItemSource) Get(context.Context, string, string) (inbox.Item, error) {
	panic(notCalled)
}

type stubInboxIdempotency struct{}

func (stubInboxIdempotency) Claim(context.Context, platformadmin.InboxActionRecord) (bool, *platformadmin.InboxActionRecord, error) {
	panic(notCalled)
}
func (stubInboxIdempotency) Complete(context.Context, string, json.RawMessage) error {
	panic(notCalled)
}

type stubEmailTemplateStore struct{}

func (stubEmailTemplateStore) List(context.Context) ([]emailtemplates.Row, error) { panic(notCalled) }
func (stubEmailTemplateStore) Get(context.Context, string) (emailtemplates.Row, bool, error) {
	panic(notCalled)
}
func (stubEmailTemplateStore) Upsert(context.Context, emailtemplates.UpsertInput) (emailtemplates.Row, error) {
	panic(notCalled)
}

type stubEmailTemplateRegistry struct{}

func (stubEmailTemplateRegistry) RegisteredKeys() []string { panic(notCalled) }
func (stubEmailTemplateRegistry) Fallback(string) (emailtemplates.EmbeddedFallback, bool) {
	panic(notCalled)
}
func (stubEmailTemplateRegistry) Invalidate(string) { panic(notCalled) }
func (stubEmailTemplateRegistry) Render(context.Context, string, any) (emailtemplates.Rendered, error) {
	panic(notCalled)
}

type stubTestSender struct{}

func (stubTestSender) SendTest(context.Context, string, emailtemplates.Rendered) error {
	panic(notCalled)
}

type stubEstateUsers struct{}

func (stubEstateUsers) List(context.Context, estateuserdir.ListParams) (*estateuserdir.ListResult, error) {
	panic(notCalled)
}

type stubBreakGlassRotator struct{}

func (stubBreakGlassRotator) RotateOne(context.Context, uuid.UUID) error { panic(notCalled) }

type stubBreakGlassWriter struct{}

func (stubBreakGlassWriter) Disable(context.Context, uuid.UUID, string) error { panic(notCalled) }
func (stubBreakGlassWriter) Enable(context.Context, uuid.UUID) error          { panic(notCalled) }
func (stubBreakGlassWriter) ClearIPLock(context.Context, []byte) (int64, error) {
	panic(notCalled)
}

type stubBreakGlassRateLimiter struct{}

func (stubBreakGlassRateLimiter) Reset(context.Context, breakglass.LoginKey) error {
	panic(notCalled)
}

type stubCatalogResolver struct{}

func (stubCatalogResolver) Resolve(context.Context) consolecatalog.Resolution { panic(notCalled) }

// platformDeps returns a platformadmin.Deps with EVERY mount guard in
// Register satisfied, so the manifest sees the whole surface. A field left
// nil here silently shrinks the manifest to the routes it happened to see,
// which is the vacuous-pass hazard every route guard in this service warns
// about — TestPlatformDepsLeavesNothingNil is what catches it.
func platformDeps(t *testing.T) platformadmin.Deps {
	t.Helper()

	emitter, err := audit.NewEmitter(audit.EmitterConfig{Repo: audit.NewRepository()})
	if err != nil {
		t.Fatalf("audit.NewEmitter: %v", err)
	}
	t.Cleanup(func() { emitter.Stop(context.Background()) })

	return platformadmin.Deps{
		DB:     &gorm.DB{},
		Repo:   audit.NewRepository(),
		Logger: slog.Default(),
		// Non-empty so RequirePlatformAuth does not short-circuit; the
		// value is never verified because no request is served.
		Secret: "route-manifest-not-a-real-secret",

		TenantDirectory:  stubTenantDirectory{},
		OnboardingFunnel: stubOnboardingFunnel{},
		EstateCounts:     stubEstateCounts{},
		Subscriptions: platformadmin.SubscriptionsFunc(
			func(context.Context, *gorm.DB, time.Time, time.Duration) (int64, error) {
				panic(notCalled)
			}),
		Trials: platformadmin.TrialListerFunc(
			func(context.Context, *gorm.DB, time.Time, time.Duration, int, int, trial.ListOptions) ([]trial.ExpiringRow, int64, error) {
				panic(notCalled)
			}),
		AllSubscriptions: platformadmin.SubscriptionListerFunc(
			func(context.Context, *gorm.DB, subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error) {
				panic(notCalled)
			}),

		TenantLifecycle:       stubTenantLifecycle{},
		Emitter:               emitter,
		TenantGateInvalidator: stubGateInvalidator{},

		Tickets:       stubTicketLister{},
		Notifications: stubNotificationLister{},
		Outbox: platformadmin.OutboxListerFunc(
			func(context.Context, *gorm.DB, outbox.PlatformListFilter, time.Time) (outbox.PlatformListResult, error) {
				panic(notCalled)
			}),
		OutboxWriter: stubOutboxWriter{},

		TrialExtender: platformadmin.TrialExtenderFunc(
			func(context.Context, *gorm.DB, uuid.UUID, time.Time, time.Time, string) (trial.ExtendResult, error) {
				panic(notCalled)
			}),
		TenantDiscount: &platformadmin.TenantDiscounterFuncs{
			ApplyFunc: func(context.Context, tenantdiscount.Input) (tenantdiscount.Result, error) {
				panic(notCalled)
			},
			RemoveFunc: func(context.Context, tenantdiscount.Input) (tenantdiscount.Result, error) {
				panic(notCalled)
			},
		},

		TenantTeardown: stubTenantTeardown{},
		Purger:         stubPurger{},

		Inbox:           stubInboxAggregator{},
		InboxItems:      stubInboxItemSource{},
		InboxActionIdem: stubInboxIdempotency{},

		EmailSends: platformadmin.EmailSendListerFunc(
			func(context.Context, *gorm.DB, emaillog.PlatformListFilter, time.Time) (emaillog.PlatformListResult, error) {
				panic(notCalled)
			}),
		EmailTemplates:          stubEmailTemplateStore{},
		EmailTemplateRegistry:   stubEmailTemplateRegistry{},
		EmailTemplateTestSender: stubTestSender{},

		EstateUsers: stubEstateUsers{},
		BreakGlass: platformadmin.BreakGlassListerFunc(
			func(context.Context, *gorm.DB, breakglass.PlatformListFilter, time.Time) (breakglass.PlatformListResult, error) {
				panic(notCalled)
			}),
		BreakGlassRotator:     stubBreakGlassRotator{},
		BreakGlassWriter:      stubBreakGlassWriter{},
		BreakGlassRateLimiter: stubBreakGlassRateLimiter{},
		BreakGlassIPHMACKey:   breakglass.HMACKey("route-manifest-not-a-real-key"),

		PriceCatalog: stubCatalogResolver{},
		CatalogMode:  "test",
	}
}

// ---------------------------------------------------------------------------
// storefront.Deps interface fields. Same rule as the platformadmin stubs
// above: non-nil at mount time, never called.
// ---------------------------------------------------------------------------

type stubCountryLister struct{}

func (stubCountryLister) ListSupported(*gin.Context) { panic(notCalled) }

type stubCustomerProfileService struct{}

func (stubCustomerProfileService) LookupProfile(context.Context, uuid.UUID, string) (*customer.CustomerProfile, error) {
	panic(notCalled)
}
func (stubCustomerProfileService) JoinStore(context.Context, customer.JoinStoreInput, *gin.Context) (*customer.CustomerProfile, error) {
	panic(notCalled)
}
