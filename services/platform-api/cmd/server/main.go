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

	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/invitation"
	"github.com/mark8ly/platform-api/internal/location"
	"github.com/mark8ly/platform-api/internal/notification"
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
	// In dev with no SendGrid key, log emails to stdout (so devs can grab
	// the OTP for testing). In prod or with a key set, send for real.
	var sender notification.Sender
	if cfg.SendGridAPIKey != "" {
		sender = notification.NewSendGridSender(cfg.SendGridAPIKey)
	} else {
		log.Warn("notification: no SENDGRID_API_KEY set — using LogSender (emails will be printed to stdout)")
		sender = notification.NewLogSender(log)
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
			Sender:       sender,
			EmailFrom:    cfg.EmailFrom,
			SupportEmail: cfg.EmailFrom,
			Recorder:     tokenRecorder,
		},
	)
	verifHandler := verification.NewHandler(verifSvc)

	onboardingSvc := onboarding.NewService(onboarding.Config{
		DB:                    conn,
		Repo:                  onboarding.NewRepository(conn),
		TenantRepo:            tenantRepo,
		StoreRepo:             storeRepo,
		Sender:                sender,
		EmailFrom:             cfg.EmailFrom,
		AdminURLTemplate:      cfg.AdminBaseURLTemplate,
		StorefrontURLTemplate: cfg.StorefrontBaseURLTemplate,
		SupportEmail:          cfg.EmailFrom,
	})
	onboardingHandler := onboarding.NewHandler(onboardingSvc, verifSvc)

	// ─── Invitations (Phase P) ─────────────────────────────────────────
	// The accept URL is built from cfg.AdminBaseURLTemplate. In dev
	// the template is a flat host (http://localhost:4202); in prod it
	// becomes a per-slug template (https://%s-admin.mark8ly.com). We
	// detect which shape we've been given by checking for the %s
	// verb so ops can pick either model without a code change.
	adminBase := cfg.AdminBaseURLTemplate
	acceptURL := func(slug, token string) string {
		base := adminBase
		if strings.Contains(adminBase, "%s") {
			base = fmt.Sprintf(adminBase, slug)
		}
		return base + "/accept-invite?token=" + token
	}
	invitationSvc := invitation.NewService(invitation.Config{
		Repo:       invitation.NewRepository(conn),
		TenantRepo: tenantRepo,
		StoreRepo:  storeRepo,
		FGA:        fga,
		Sender:     sender,
		EmailFrom:  cfg.EmailFrom,
		AcceptURL:  acceptURL,
		Recorder:   invitationRec,
	})
	invitationHandler := invitation.NewHandler(invitationSvc)

	// ─── Outbox drainer ────────────────────────────────────────────────
	drainer := outbox.NewDrainer(conn, log, outbox.Config{})
	if fga != nil {
		drainer.Register(onboarding.FGAOutboxKind, onboarding.NewFGAOutboxHandler(fga))
	}

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	v1 := r.Group("/api/v1")
	internal := r.Group("/internal")

	locationHandler.Register(v1)
	tenantHandler.Register(v1, internal)
	storeHandler.Register(v1, internal)
	verifHandler.Register(v1)
	onboardingHandler.Register(v1)
	invitationHandler.Register(v1, internal)

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
