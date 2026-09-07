// Command server is the platform-api HTTP entrypoint.
//
// It does NOT run migrations. On startup it asserts that the DB is at the
// expected schema version and refuses to start otherwise. This is the safety
// net that ensures the API never runs against a wrong schema.
//
// Domain wiring lives here. Each domain owns its handler/service/repo trio
// and registers routes onto the /api/v1 group. The outbox drainer is started
// last so it can process any events written by handlers.
package main

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"time"
	// Embed the IANA tzdata so time.LoadLocation works in scratch / slim
	// Alpine images that don't ship /usr/share/zoneinfo. Used by the store
	// timezone validator in PATCH /internal/stores/:id.
	_ "time/tzdata"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/gin-gonic/gin"
	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/account"
	"github.com/mark8ly/platform-api/internal/audit"
	"github.com/mark8ly/platform-api/internal/auth"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/estate"
	"github.com/mark8ly/platform-api/internal/estateuser"
	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/invitation"
	"github.com/mark8ly/platform-api/internal/location"
	"github.com/mark8ly/platform-api/internal/marketplaceapi"

	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/observability"
	"github.com/mark8ly/platform-api/internal/onboarding"
	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/platformadmin"
	"github.com/mark8ly/platform-api/internal/routes"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	testhelper "github.com/mark8ly/platform-api/internal/test"
	"github.com/mark8ly/platform-api/internal/verification"
	"github.com/mark8ly/platform-api/pkg/config"
	"github.com/mark8ly/platform-api/pkg/db"
	"github.com/mark8ly/platform-api/pkg/httpserver"
	"github.com/mark8ly/platform-api/pkg/logger"
	"github.com/mark8ly/platform-api/pkg/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	// ─── Observability ──────────────────────────────────────────────────
	// Initialise OpenTelemetry (traces + metrics over OTLP/gRPC) before any
	// real work so startup is traced too. No-op when
	// OTEL_EXPORTER_OTLP_ENDPOINT is unset (dev / collector-less envs).
	const serviceName = "mark8ly-platform-api"
	otelShutdown, err := observability.Init(context.Background(), serviceName)
	if err != nil {
		log.Warn("observability: init failed — continuing without telemetry", "err", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if sErr := otelShutdown(shutdownCtx); sErr != nil {
			log.Warn("observability: shutdown", "err", sErr)
		}
	}()

	// Verify schema version. Refuse to start on mismatch.
	mig, err := migrate.New(platformapi.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		panic(err)
	}
	if err := mig.AssertVersion(platformapi.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run migrations first", "err", err)
		panic(err)
	}

	// Open DB.
	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("db open", "err", err)
		panic(err)
	}

	// ─── Notification sender ────────────────────────────────────────────
	// EMAIL_PRIMARY_PROVIDER orders the provider chain; every other
	// configured provider is an always-on per-message fallback, so a
	// provider outage degrades to a provider switch instead of dropped
	// mail. In dev with no keys, emails log to stdout (so devs can grab
	// the OTP for testing). New providers plug in here: add the key to
	// this map after registering the adapter in internal/notification.
	sender := notification.NewFromConfig(map[string]string{
		notification.ProviderSendGrid: cfg.SendGridAPIKey,
		notification.ProviderResend:   cfg.ResendAPIKey,
	}, cfg.EmailPrimaryProvider, log)

	// ─── Notification template loader ───────────────────────────────────
	// DB-backed templates with embedded fallback. tesserix-home authors
	// templates over the cross-DB grant; the loader reads them with a
	// 5-minute TTL cache and falls back to the embedded version on miss
	// or DB error so emails keep flowing during outages. SeedFromEmbedded
	// is idempotent (ON CONFLICT DO NOTHING) so the first boot after
	// migration 0013 ships byte-identical output to the embedded path.
	templateLoader := notification.NewLoader(conn)
	{
		seedCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if seedErr := templateLoader.SeedFromEmbedded(seedCtx); seedErr != nil {
			log.Warn("notification: template seed failed (continuing with embedded fallback)", "err", seedErr)
		}
		cancel()
	}

	// ─── OpenFGA client ────────────────────────────────────────────────
	// FGA_STORE_ID can be set explicitly via env. If not, we discover the
	// store named "mark8ly-platform" automatically — that's the store
	// created by infra/dev/seed/fga-init.sh, so a fresh `make dev` self-
	// bootstraps without anyone hand-managing the store ID.
	storeID := cfg.FGAStoreID
	if storeID == "" {
		discoverCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		discovered, derr := authz.DiscoverStoreID(discoverCtx, cfg.FGAAPIURL, authz.FGAStoreName)
		cancel()
		if derr != nil {
			log.Warn("authz: store discovery failed; FGA outbox events will be marked dead",
				"err", derr)
		} else if discovered == "" {
			log.Warn("authz: no store named " + authz.FGAStoreName + " found; FGA outbox events will be marked dead")
		} else {
			storeID = discovered
			log.Info("authz: discovered openfga store", "store_id", storeID)
		}
	}

	var fga authz.Client
	if storeID != "" {
		fga, err = authz.New(authz.Config{
			APIURL:  cfg.FGAAPIURL,
			StoreID: storeID,
		})
		if err != nil {
			log.Error("authz: openfga client", "err", err)
			panic(err)
		}
	}

	// ─── Domains ───────────────────────────────────────────────────────
	locationHandler := location.NewHandler(location.NewService(location.NewRepository(conn)))

	tenantRepo := tenant.NewRepository(conn)
	tenantSvc := tenant.NewService(tenantRepo, fga)
	tenantHandler := tenant.NewHandler(tenantSvc, fga)

	// Phase Q — store domain.
	storeRepo := store.NewRepository(conn)
	storeSvc := store.NewService(storeRepo, fga)
	storeHandler := store.NewHandler(storeSvc, fga)

	// In dev/test environments, capture plaintext magic-link tokens so the
	// e2e suite can bypass the inbox. nil in prod — verification.Service
	// no-ops the recorder call when nil.
	var tokenRecorder *testhelper.TokenRecorder
	var invitationRec *testhelper.InvitationTokenRecorder
	if cfg.Env != "prod" {
		tokenRecorder = testhelper.NewTokenRecorder()
		invitationRec = testhelper.NewInvitationTokenRecorder()
	}

	verifSvc := verification.NewService(
		verification.NewRepository(conn),
		verification.Config{
			Sender:        sender,
			Loader:        templateLoader,
			EmailFrom:     cfg.EmailFrom,
			SupportEmail:  cfg.EmailFrom,
			VerifyURLBase: cfg.OnboardingBaseURL,
			Recorder:      tokenRecorder,
		},
	)
	verifHandler := verification.NewHandler(verifSvc)

	vendorClient := marketplaceapi.NewVendorClient(cfg.MarketplaceAPIURL, cfg.MarketplaceInternalAuthSecret)

	// Zitadel-path owner provisioning for onboarding completion (#685).
	// Nil (and every downstream behaviour byte-identical to before)
	// unless ZITADEL_ENABLED is true; a misconfiguration panics rather
	// than quietly onboarding merchants who can never sign in — see
	// newOwnerProvisioner's doc.
	ownerProvisioner, ownerProvErr := newOwnerProvisioner(cfg)
	if ownerProvErr != nil {
		log.Error("owner provisioner wiring", "err", ownerProvErr)
		panic(ownerProvErr)
	}

	onboardingSvc := onboarding.NewService(onboarding.Config{
		DB:                    conn,
		Repo:                  onboarding.NewRepository(conn),
		TenantRepo:            tenantRepo,
		StoreRepo:             storeRepo,
		Sender:                sender,
		Loader:                templateLoader,
		EmailFrom:             cfg.EmailFrom,
		AdminURLTemplate:      cfg.AdminBaseURLTemplate,
		StorefrontURLTemplate: cfg.StorefrontBaseURLTemplate,
		SupportEmail:          cfg.EmailFrom,
		VendorClient:          vendorClient,
		Provisioner:           ownerProvisioner,
	})
	onboardingHandler := onboarding.NewHandler(onboardingSvc, verifSvc)

	// ─── Invitations (Phase P) ─────────────────────────────────────────
	// The accept URL is built from cfg.AdminBaseURLTemplate. In dev
	// the template is a flat host (http://localhost:4202); in prod it
	// becomes a per-slug template (https://{slug}-admin.mark8ly.com).
	// Supports both the {slug} placeholder (Helm chart convention) and
	// the %s printf verb (legacy), so ops can pick either without a
	// code change.
	adminBase := cfg.AdminBaseURLTemplate
	acceptURL := func(slug, token string) string {
		base := adminBase
		switch {
		case strings.Contains(adminBase, "{slug}"):
			base = strings.ReplaceAll(adminBase, "{slug}", slug)
		case strings.Contains(adminBase, "%s"):
			base = fmt.Sprintf(adminBase, slug)
		}
		return base + "/accept-invite?token=" + token
	}
	// Cross-service audit client — posts staff lifecycle events to
	// marketplace-api's /internal/audit-events ingest. Empty URL or
	// secret disables it (dev convenience).
	auditClient := audit.New(cfg.MarketplaceAPIURL, cfg.AuditIngestSecret, log)

	// ─── GIP admin client (EnsureTenantClaim) ───────────────────────────
	// Hoisted: the outbox drainer needs this client to stamp the owner's
	// tenant_id GIP custom claim after onboarding completes, and the
	// invitation service needs it to stamp the same claim on accept.
	//
	// Its lifetime is governed ENTIRELY by GIP_PROJECT_ID/GIP_TENANT_ID/a
	// GIP API key being present — NEVER by cfg.ZitadelEnabled. D7 drops
	// this claim only once ZITADEL_ENABLED is true on marketplace-api too
	// (a separate service, a separate cutover); until then a Zitadel
	// deployment of platform-api still needs this client alive for
	// EnsureTenantClaim, or newly-invited merchants get a permanent "No
	// store yet" on mobile. See selectAccountProviders' doc below for the
	// two-concerns-two-lifetimes split this implies — do NOT gate this
	// construction on the Zitadel flag.
	var gipAdmin *gipadmin.AdminClient
	// GIPKey prefers the unrestricted server key and falls back to the
	// public web key. The web key is referrer-restricted, so admin calls
	// such as resetPassword fail 403 "Requests from referer <empty> are
	// blocked" when it is all that is configured.
	if cfg.GIPProjectID != "" && cfg.GIPTenantID != "" && cfg.GIPKey() != "" {
		admin, adminErr := gipadmin.New(context.Background(), gipadmin.Config{
			ProjectID: cfg.GIPProjectID,
			TenantID:  cfg.GIPTenantID,
			WebAPIKey: cfg.GIPKey(),
		})
		if adminErr != nil {
			log.Error("gipadmin: init", "err", adminErr)
			log.Warn("gip: tenant-claim client disabled — gipadmin init failed")
		} else {
			gipAdmin = admin
		}
	} else {
		log.Warn("gip: tenant-claim client disabled — missing GIP_PROJECT_ID/GIP_TENANT_ID and GIP_SERVER_API_KEY or GIP_WEB_API_KEY")
	}
	// A Zitadel cutover must not silently drop EnsureTenantClaim's GIP
	// dependency — see requireGIPForTenantClaim's doc (provider_wiring.go)
	// for why "we enabled Zitadel, so GIP_* is dead weight" is exactly the
	// deploy-time mistake this guards against. Panics, matching every
	// other startup failure in this file.
	if err := requireGIPForTenantClaim(cfg, gipAdmin); err != nil {
		log.Error("startup: gip required for tenant claim", "err", err)
		panic(err)
	}

	// ─── Password-reset / account-delete provider (#524 phase 5) ───────
	// Wires the /internal/auth/password-reset/* endpoints used by the
	// admin BFF, and the deleter behind account.Service. Selected by
	// ZITADEL_ENABLED, defaulting to GIP — see selectAccountProviders'
	// doc (provider_wiring.go) for the full reasoning, including why
	// gipAdmin above is a SEPARATE concern from what this selects.
	//
	// A misconfigured-but-enabled Zitadel must fail startup loudly rather
	// than silently keep serving merchants against GIP; panic here mirrors
	// every other startup failure in this file.
	resetProvider, accountDeleter, providerErr := selectAccountProviders(cfg, gipAdmin)
	if providerErr != nil {
		log.Error("account provider selection", "err", providerErr)
		panic(providerErr)
	}

	var authHandler *auth.Handler
	if resetProvider != nil {
		authSvc := auth.NewService(auth.Config{
			Admin:             resetProvider,
			Sender:            sender,
			Loader:            templateLoader,
			EmailFrom:         cfg.EmailFrom,
			SupportEmail:      cfg.EmailFrom,
			AdminResetBaseURL: cfg.AdminResetBaseURL,
			Logger:            log,
		})
		authHandler = auth.NewHandler(authSvc, log)
		if cfg.ZitadelEnabled {
			log.Info("auth: password reset enabled (zitadel)",
				"reset_url", cfg.AdminResetBaseURL)
		} else {
			log.Info("auth: password reset enabled (gip)",
				"project_id", cfg.GIPProjectID,
				"tenant_id", cfg.GIPTenantID,
				"reset_url", cfg.AdminResetBaseURL)
		}
	} else {
		log.Warn("auth: password reset disabled — missing GIP_PROJECT_ID/GIP_TENANT_ID and GIP_SERVER_API_KEY or GIP_WEB_API_KEY")
	}

	// newTenantClaimSetter is called UNCONDITIONALLY, with gipAdmin as its
	// only argument — see that function's doc (provider_wiring.go) for why
	// its signature has no access to cfg.ZitadelEnabled at all. Do not
	// wrap this call in an `if`, and do not change its argument: doing
	// either would (re)break EnsureTenantClaim under Zitadel, which is the
	// exact regression cmd/server/main_test.go's
	// TestMainCallsNewTenantClaimSetterUnconditionally exists to catch.
	inviteClaims := newTenantClaimSetter(gipAdmin)

	// Zitadel-path staff provisioning for invite-accept. Nil (and every
	// downstream behaviour byte-identical to before) unless
	// ZITADEL_ENABLED is true; a misconfiguration panics rather than
	// quietly provisioning nobody — see newStaffProvisioner's doc.
	staffProvisioner, provisionerErr := newStaffProvisioner(cfg)
	if provisionerErr != nil {
		log.Error("staff provisioner wiring", "err", provisionerErr)
		panic(provisionerErr)
	}

	invitationSvc := invitation.NewService(invitation.Config{
		Repo:        invitation.NewRepository(conn),
		TenantRepo:  tenantRepo,
		StoreRepo:   storeRepo,
		FGA:         fga,
		Sender:      sender,
		Loader:      templateLoader,
		EmailFrom:   cfg.EmailFrom,
		AcceptURL:   acceptURL,
		Recorder:    invitationRec,
		Audit:       auditClient,
		Claims:      inviteClaims,
		Provisioner: staffProvisioner,
	})
	invitationHandler := invitation.NewHandler(invitationSvc)

	// ─── Account teardown (Task 5) / operator tenant purge (#288) ───────
	// The operator teardown path (#288) needs neither FGA nor an
	// account-deletion provider to function — its cleanup of both is
	// best-effort post-commit — so the service is constructed
	// unconditionally and its route is mounted unconditionally below.
	// Only the MERCHANT DeleteAccount route stays gated: it calls
	// fga.GetRole and gip.DeleteAccount with no internal nil-check and
	// would panic on first call.
	//
	// accountDeleter already comes from selectAccountProviders as a
	// genuinely nil-or-real interface value (never a typed nil) — see
	// that function's doc and newAccountService's doc for the trap this
	// avoids.
	accountSvc := newAccountService(conn, tenantRepo, fga, accountDeleter, outbox.EnqueueAfter, log)
	accountHandler := account.NewHandler(accountSvc)
	merchantAccountRoutes := fga != nil && accountDeleter != nil
	if !merchantAccountRoutes {
		log.Warn("account: merchant teardown endpoint disabled — missing OpenFGA store or an account-deletion provider (GIP_PROJECT_ID/GIP_TENANT_ID/a GIP API key, or ZITADEL_ENABLED); operator teardown (#288) stays mounted")
	}

	// ─── Outbox drainer ────────────────────────────────────────────────
	drainer := outbox.NewDrainer(conn, log, outbox.Config{})
	if fga != nil {
		drainer.Register(onboarding.FGAOutboxKind, onboarding.NewFGAOutboxHandler(fga))
	}
	if gipAdmin != nil {
		drainer.Register(onboarding.GIPClaimOutboxKind, onboarding.NewGIPClaimOutboxHandler(gipAdmin))
	} else {
		// Unregistered kinds stay pending rather than erroring, so the
		// rows drain once GIP credentials are configured. Loud, because
		// mobile-admin login is broken for every new tenant until then.
		log.Warn("outbox: gip tenant-claim handler NOT registered — mobile admin login will fail for new tenants")
	}
	// vendorClient is unconditionally constructed above (Line ~189) — its
	// HTTP calls degrade gracefully (returned as errors, retried by the
	// drainer) rather than panicking, so no nil guard is needed here.
	// Registered even when accountHandler is disabled: the teardown
	// endpoint being off (missing FGA/GIP) doesn't mean no rows will ever
	// need draining — e.g. rows enqueued before a config change rolls out.
	drainer.Register(account.TenantDeletedOutboxKind, account.NewTenantDeletedHandler(vendorClient))

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	// Trace every request. No-op spans when telemetry is disabled, so this
	// is safe to install unconditionally.
	r.Use(otelgin.Middleware(serviceName))

	// The Tesserix console's HMAC-signed contract surface (#720 Task 3).
	// Mounted under platformadmin.MountPrefix (/api/v1/platform), never
	// /api/v1/admin — see the Register doc comment in
	// internal/platformadmin/routes.go for why. An empty
	// cfg.PlatformAdminSecret leaves this mounted but inert (503
	// not_configured on every request), matching marketplace-api's
	// platformadmin surface, so this binary ships before the secret exists.
	platformadmin.Register(r.Group(platformadmin.MountPrefix), platformadmin.Deps{
		DB:      conn,
		Logger:  log,
		Secret:  cfg.PlatformAdminSecret,
		Emitter: auditClient,
	})

	v1 := r.Group("/api/v1")
	// /internal/* is the in-cluster trust surface — admin BFF, auth-bff,
	// marketplace-api call into here. The route-to-guard mapping lives in
	// internal/routes so it can be TESTED (#323): while it was inline
	// here, moving an estate-wide handler onto the permissive guard left
	// the build and the whole test suite green, and the downgrade would
	// have shipped undetected.
	//
	// Handlers that also mount on /api/v1 are wrapped so that both halves
	// stay at their original call sites; MountInternal only decides which
	// /internal guard each one sits behind.
	routes.MountInternal(r, cfg.InternalAuthSecret, routes.InternalHandlers{
		TenantDirectory:     tenantHandler.RegisterDirectory,
		TenantLifecycle:     tenantHandler.RegisterLifecycle,
		OnboardingAnalytics: onboardingHandler.RegisterAnalytics,
		EstateCounts:        estate.NewHandler(estate.NewRepository(conn)).Register,
		EstateUsers:         estateuser.NewHandler(estateuser.NewRepository(conn)).Register,
		AccountOperator:     accountHandler.RegisterOperator,

		Tenant:          func(g *gin.RouterGroup) { tenantHandler.Register(v1, g) },
		Store:           func(g *gin.RouterGroup) { storeHandler.Register(v1, g) },
		Invitation:      func(g *gin.RouterGroup) { invitationHandler.Register(v1, g) },
		Auth:            authInternalRegistrar(authHandler),
		MerchantAccount: merchantAccountRegistrar(merchantAccountRoutes, accountHandler),
		Notification:    notification.NewHandler(templateLoader, sender, cfg.EmailFrom).Register,
	})

	locationHandler.Register(v1)
	verifHandler.Register(v1)
	onboardingHandler.Register(v1)

	// e2e helper routes — only mounted outside production. Gives Playwright
	// a way to grab the latest magic-link token for an email without
	// reading the inbox.
	if tokenRecorder != nil {
		testhelper.NewHandler(tokenRecorder, invitationRec).Register(v1)
	}

	// ─── Lifecycle ─────────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	drainer.Start(ctx)
	defer drainer.Stop()

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}

// authInternalRegistrar returns nil when the auth handler is not
// configured, preserving main.go's original `if authHandler != nil`
// guard. A nil Registrar is skipped by routes.MountInternal — the typed
// nil is deliberate here: returning a non-nil closure that wraps a nil
// handler would make the skip unreachable and dispatch on a nil receiver
// (the shape #341 was filed for).
func authInternalRegistrar(h *auth.Handler) routes.Registrar {
	if h == nil {
		return nil
	}
	return h.Register
}

// merchantAccountRegistrar mirrors main.go's `if merchantAccountRoutes`
// guard: the merchant-facing account routes are mounted only when both
// FGA and the GIP admin client are available.
func merchantAccountRegistrar(enabled bool, h *account.Handler) routes.Registrar {
	if !enabled || h == nil {
		return nil
	}
	return h.Register
}
