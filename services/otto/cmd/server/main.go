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
	"github.com/mark8ly/otto/internal/message"
	ottomongo "github.com/mark8ly/otto/internal/mongo"
	"github.com/mark8ly/otto/internal/session"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	signer := session.NewSigner(cfg.CustomerSessionSecret, 30*24*time.Hour)
	h := hub.New(log)

	// ── HTTP server ────────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log, cfg.CORSAllowedOrigins)

	// Storefront (customer) group: tenant + store come from headers, the
	// session cookie binds the caller to a specific thread.
	storefront := r.Group("/api/v1/storefront/otto")
	storefront.Use(auth.CustomerContext(cfg.InternalAuthSecret))
	conversation.NewStorefrontHandler(conversation.StorefrontDeps{
		Conversations: convRepo,
		Messages:      msgRepo,
		Hub:           h,
		Signer:        signer,
		CookieName:    cfg.CustomerSessionCookie,
		CookieDomain:  cfg.CustomerCookieDomain,
		CookieSecure:  cfg.CustomerCookieSecure,
		Logger:        log,
	}).Register(storefront)

	// Admin (staff) group: identity trusted from the proxy, store locked
	// per request.
	admin := r.Group("/api/v1/admin/otto")
	admin.Use(auth.StaffAuth(cfg.InternalAuthSecret), auth.StoreResolver())
	conversation.NewAdminHandler(conversation.AdminDeps{
		Conversations: convRepo,
		Messages:      msgRepo,
		Hub:           h,
		Logger:        log,
	}).Register(admin)

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http", "err", err)
		panic(err)
	}
}
