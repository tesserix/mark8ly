package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark8ly/otto/internal/auth"
	"github.com/mark8ly/otto/internal/config"
	"github.com/mark8ly/otto/internal/conversation"
	"github.com/mark8ly/otto/internal/httpserver"
	"github.com/mark8ly/otto/internal/hub"
	"github.com/mark8ly/otto/internal/logger"
	"github.com/mark8ly/otto/internal/mailer"
	"github.com/mark8ly/otto/internal/message"
	ottomongo "github.com/mark8ly/otto/internal/mongo"
	"github.com/mark8ly/otto/internal/observability"
	"github.com/mark8ly/otto/internal/otp"
	"github.com/mark8ly/otto/internal/session"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// serviceName identifies otto in traces/metrics; matches the chart name.
const serviceName = "mark8ly-otto"

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Observability ──────────────────────────────────────────────────
	// No-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset (local/dev).
	otelShutdown, err := observability.Init(ctx, serviceName)
	if err != nil {
		log.Error("observability: init", "err", err)
		panic(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = otelShutdown(shutdownCtx)
	}()

	// ── Mongo ──────────────────────────────────────────────────────────
	mongoClient, err := ottomongo.Connect(ctx, cfg.MongoURL, cfg.MongoDatabase)
	if err != nil {
		log.Error("mongo: connect", "err", err)
		panic(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mongoClient.Close(shutdownCtx)
	}()
	if err := mongoClient.EnsureIndexes(ctx); err != nil {
		log.Error("mongo: indexes", "err", err)
		panic(err)
	}

	// ── Core services ──────────────────────────────────────────────────
	convRepo := conversation.NewRepository(mongoClient.Conversations())
	msgRepo := message.NewRepository(mongoClient.Messages())
	availRepo := conversation.NewAvailabilityRepository(mongoClient.StaffAvailability())
	auditRepo := conversation.NewAuditRepository(mongoClient.Audit())

	// OTP: repo + mailer + service. Two providers; EMAIL_PRIMARY_PROVIDER
	// picks which sends first and the other is the always-on per-message
	// fallback. When neither key is set we fall back to a stdout log
	// mailer so the service remains bootable.
	otpRepo, err := otp.NewRepository(ctx, mongoClient.OTP())
	if err != nil {
		log.Error("otp: repo", "err", err)
		panic(err)
	}
	mail := mailer.NewFromConfig(map[string]string{
		mailer.ProviderSendGrid: cfg.SendgridAPIKey,
		mailer.ProviderResend:   cfg.ResendAPIKey,
	}, cfg.EmailPrimaryProvider, cfg.OTPFromEmail, cfg.OTPFromName, log)
	otpSvc := otp.NewService(otpRepo, mail, otp.Config{
		TTL:            time.Duration(cfg.OTPCodeTTL) * time.Second,
		MaxAttempts:    cfg.OTPMaxAttempts,
		ResendCooldown: time.Duration(cfg.OTPResendCooldown) * time.Second,
	})

	signer := session.NewSigner(cfg.CustomerSessionSecret, 30*24*time.Hour)
	// Short TTL — the client opens the WS immediately after minting the
	// ticket, so 2 minutes is plenty and keeps the replay window tight.
	ticketSigner := session.NewTicketSigner(cfg.CustomerSessionSecret, 2*time.Minute)
	h := hub.New(log)

	// ── HTTP server ────────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log, cfg.CORSAllowedOrigins)
	// Trace inbound HTTP. No-op spans when the TracerProvider is the
	// default no-op (observability disabled), so this is safe to attach
	// unconditionally.
	r.Use(otelgin.Middleware(serviceName))

	// Storefront REST routes — all run the CustomerContext middleware so
	// the Next.js proxy's tenant/store/internal-auth headers gate entry.
	storefront := r.Group("/api/v1/storefront/otto")
	storefront.Use(auth.CustomerContext(cfg.InternalAuthSecret))
	storefrontHandler := conversation.NewStorefrontHandler(conversation.StorefrontDeps{
		Conversations: convRepo,
		Availability:  availRepo,
		Audit:         auditRepo,
		Messages:      msgRepo,
		Hub:           h,
		Signer:        signer,
		Tickets:       ticketSigner,
		OTP:           otpSvc,
		CookieName:    cfg.CustomerSessionCookie,
		CookieDomain:  cfg.CustomerCookieDomain,
		CookieSecure:  cfg.CustomerCookieSecure,
		Logger:        log,
	})
	storefrontHandler.Register(storefront)
	otp.NewHandler(otpSvc, log).Register(storefront)

	// Admin REST routes — StaffAuth + StoreResolver enforce identity and
	// scope.
	admin := r.Group("/api/v1/admin/otto")
	admin.Use(auth.StaffAuth(cfg.InternalAuthSecret), auth.StoreResolver())
	adminHandler := conversation.NewAdminHandler(conversation.AdminDeps{
		Conversations: convRepo,
		Availability:  availRepo,
		Audit:         auditRepo,
		Messages:      msgRepo,
		Hub:           h,
		Tickets:       ticketSigner,
		Logger:        log,
	})
	adminHandler.Register(admin)

	// Internal REST routes — server-to-server only, gated by the same
	// shared secret. Used by marketplace-api to pull the chat
	// transcript that spawned a customer ticket so the storefront's
	// /account/tickets/:id view can show the live conversation
	// inline. CustomerContext supplies the same tenant/store scope
	// checks the storefront routes use, without the otto_session
	// cookie requirement (the customer's browser never hits this).
	internalGroup := r.Group("/api/v1/internal/otto")
	internalGroup.Use(auth.CustomerContext(cfg.InternalAuthSecret))
	internalHandler := conversation.NewInternalHandler(conversation.InternalDeps{
		Conversations: convRepo,
		Messages:      msgRepo,
		Logger:        log,
	})
	internalHandler.Register(internalGroup)

	// Inactivity sweeper — closes active conversations the customer
	// has gone quiet on for 15 min. One goroutine per process; no
	// leader election needed at v1 volume, the CloseForInactivity
	// CAS handles multi-pod races.
	sweeper := &conversation.InactivitySweeper{
		Conversations:    convRepo,
		Availability:     availRepo,
		Audit:            auditRepo,
		Messages:         msgRepo,
		Hub:              h,
		Logger:           log,
		InactivityWindow: 15 * time.Minute,
		SweepInterval:    60 * time.Second,
	}
	go sweeper.Run(ctx)

	// WebSocket routes are deliberately mounted on a no-middleware group:
	// Istio routes /api/v1/otto/.../ws directly to Otto, bypassing the
	// Next.js proxies that would otherwise inject our auth headers. Ticket
	// auth takes over instead — see session.TicketSigner.
	adminWS := r.Group("/api/v1/admin/otto")
	adminHandler.RegisterWS(adminWS)
	storefrontHandler.RegisterWS(r.Group("/api/v1/storefront/otto"))

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http", "err", err)
		panic(err)
	}
}
