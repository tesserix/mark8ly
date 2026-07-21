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

	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/audit"
	"github.com/mark8ly/platform-api/internal/auth"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/invitation"
	"github.com/mark8ly/platform-api/internal/location"
	"github.com/mark8ly/platform-api/internal/marketplaceapi"
	"github.com/mark8ly/platform-api/internal/middleware"
	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/observability"
	"github.com/mark8ly/platform-api/internal/onboarding"
	"github.com/mark8ly/platform-api/internal/outbox"
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

	// ─── Password reset (Phase: GIP-aware branded flow) ────────────────
	// Wires the /internal/auth/password-reset/* endpoints used by the
	// admin BFF. Requires GIP_PROJECT_ID, GIP_TENANT_ID, and
	// GIP_WEB_API_KEY. If any are missing the handler is skipped —
	// dev machines without real GIP credentials fall through to GIP's
	// default behaviour via auth-bff.
	var authHandler *auth.Handler
	// Hoisted: the outbox drainer needs this client to stamp the owner's
	// tenant_id GIP custom claim after onboarding completes, and the
	// invitation service needs it to stamp the same claim on accept.
	var gipAdmin *gipadmin.AdminClient
	if cfg.GIPProjectID != "" && cfg.GIPTenantID != "" && cfg.GIPWebAPIKey != "" {
		admin, adminErr := gipadmin.New(context.Background(), gipadmin.Config{
			ProjectID: cfg.GIPProjectID,
			TenantID:  cfg.GIPTenantID,
			WebAPIKey: cfg.GIPWebAPIKey,
		})
		if adminErr != nil {
			log.Error("gipadmin: init", "err", adminErr)
			log.Warn("auth: password reset disabled — gipadmin init failed")
		} else {
			gipAdmin = admin
			authSvc := auth.NewService(auth.Config{
				Admin:             admin,
				Sender:            sender,
				Loader:            templateLoader,
				EmailFrom:         cfg.EmailFrom,
				SupportEmail:      cfg.EmailFrom,
				AdminResetBaseURL: cfg.AdminResetBaseURL,
				Logger:            log,
			})
			authHandler = auth.NewHandler(authSvc, log)
			log.Info("auth: password reset enabled",
				"project_id", cfg.GIPProjectID,
				"tenant_id", cfg.GIPTenantID,
				"reset_url", cfg.AdminResetBaseURL)
		}
	} else {
		log.Warn("auth: password reset disabled — missing GIP_PROJECT_ID/GIP_TENANT_ID/GIP_WEB_API_KEY")
	}

	// Assign through a typed interface variable only when non-nil, so the
	// invitation service's nil check isn't defeated by a typed-nil pointer.
	var inviteClaims invitation.TenantClaimSetter
	if gipAdmin != nil {
		inviteClaims = gipAdmin
	}

	invitationSvc := invitation.NewService(invitation.Config{
		Repo:       invitation.NewRepository(conn),
		TenantRepo: tenantRepo,
		StoreRepo:  storeRepo,
		FGA:        fga,
		Sender:     sender,
		Loader:     templateLoader,
		EmailFrom:  cfg.EmailFrom,
		AcceptURL:  acceptURL,
		Recorder:   invitationRec,
		Audit:      auditClient,
		Claims:     inviteClaims,
	})
	invitationHandler := invitation.NewHandler(invitationSvc)

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

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	// Trace every request. No-op spans when telemetry is disabled, so this
	// is safe to install unconditionally.
	r.Use(otelgin.Middleware(serviceName))
	v1 := r.Group("/api/v1")
	// /internal/* is the in-cluster trust surface — admin BFF, auth-bff,
	// marketplace-api call into here. RequireInternalAuth gates every
	// route with a constant-time X-Internal-Auth check; an empty
	// secret no-ops the gate so the binary stays bootable in dev and
	// during the cutover before the secret is provisioned.
	internal := r.Group("/internal", middleware.RequireInternalAuth(cfg.InternalAuthSecret))

	locationHandler.Register(v1)
	tenantHandler.Register(v1, internal)
	storeHandler.Register(v1, internal)
	verifHandler.Register(v1)
	onboardingHandler.Register(v1)
	invitationHandler.Register(v1, internal)
	if authHandler != nil {
		authHandler.Register(internal)
	}
	notification.NewHandler(templateLoader, sender, cfg.EmailFrom).Register(internal)

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
