// Command server is the auth-bff HTTP entrypoint.
//
// Wires GIP verifier + OpenFGA client + session manager + autologin into
// the HTTP server. The session manager and OpenFGA store ID are constructed
// at startup; the schema version is asserted before the server binds.
package main

import (
	"context"
	"database/sql"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	authbff "github.com/mark8ly/auth-bff"
	"github.com/mark8ly/auth-bff/internal/audit"
	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/autologin"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/usermfa"
	"github.com/mark8ly/auth-bff/internal/usersessions"
	"github.com/mark8ly/auth-bff/pkg/config"
	"github.com/mark8ly/auth-bff/pkg/httpserver"
	"github.com/mark8ly/auth-bff/pkg/logger"
	"github.com/mark8ly/auth-bff/pkg/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	// ─── Schema version gate ───────────────────────────────────────────
	mig, err := migrate.New(authbff.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		panic(err)
	}
	if err := mig.AssertVersion(authbff.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run migrations first", "err", err)
		panic(err)
	}

	// ─── DB connection for the user_sessions registry ──────────────────
	// database/sql + lib/pq — lightweight, no ORM. The auth flow does
	// not need one; the registry just needs simple insert/select.
	dbConn, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Error("db: open", "err", err)
		panic(err)
	}
	dbConn.SetMaxOpenConns(5)
	dbConn.SetMaxIdleConns(2)
	dbConn.SetConnMaxLifetime(30 * time.Minute)
	if err := dbConn.Ping(); err != nil {
		log.Error("db: ping", "err", err)
		panic(err)
	}
	defer dbConn.Close()

	sessionRegistry := usersessions.NewRepository(dbConn)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ─── GIP verifier ──────────────────────────────────────────────────
	verifier, err := gip.New(ctx, gip.Config{
		ProjectID: cfg.GIPProjectID,
	})
	if err != nil {
		log.Error("gip: new verifier", "err", err)
		panic(err)
	}

	// ─── OpenFGA client (auto-discover store) ──────────────────────────
	storeID := cfg.FGAStoreID
	if storeID == "" {
		discoverCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		discovered, derr := authz.DiscoverStoreID(discoverCtx, cfg.FGAAPIURL, authz.FGAStoreName)
		cancel()
		if derr != nil {
			log.Warn("authz: store discovery failed", "err", derr)
		} else if discovered == "" {
			log.Warn("authz: no store named " + authz.FGAStoreName + " found — autologin will return 503")
		} else {
			storeID = discovered
			log.Info("authz: discovered openfga store", "store_id", storeID)
		}
	}

	var fgaClient authz.Client
	if storeID != "" {
		fgaClient, err = authz.New(authz.Config{
			APIURL:  cfg.FGAAPIURL,
			StoreID: storeID,
		})
		if err != nil {
			log.Error("authz: openfga client", "err", err)
			panic(err)
		}
	}

	// ─── Session manager ───────────────────────────────────────────────
	// Secure flag: only set in non-dev. Plain HTTP localhost dev needs
	// Secure=false or the browser silently drops the cookie.
	sessions, err := session.NewManager(session.Config{
		CookieName: cfg.SessionCookieName,
		Domain:     cfg.SessionCookieDomain,
		Secure:     cfg.Env != "dev",
		EncryptKey: cfg.SessionEncryptKey,
	})
	if err != nil {
		log.Error("session: new manager", "err", err)
		panic(err)
	}

	// ─── MFA (TOTP) ────────────────────────────────────────────────────
	// Constructed before autologin so the MFA gate can be injected into
	// the login path. Reuses the session encrypt key for at-rest secret
	// encryption — fatal-on-init-error because MFA cannot degrade to
	// plaintext storage safely.
	mfaSvc, err := usermfa.NewService(dbConn, cfg.SessionEncryptKey)
	if err != nil {
		log.Error("usermfa: new service", "err", err)
		panic(err)
	}
	mfaHandler := usermfa.NewHandler(mfaSvc, log)

	// ─── Cross-service audit client ────────────────────────────────────
	// Posts user.signed_in / user.signed_out events to marketplace-api's
	// /internal/audit-events ingest endpoint. Empty MarketplaceAPIURL
	// disables it so dev environments without marketplace-api still boot.
	auditClient := audit.New(cfg.MarketplaceAPIURL, cfg.AuditIngestSecret, log)

	// ─── Autologin ─────────────────────────────────────────────────────
	autologinSvc := autologin.NewService(autologin.Config{
		GIP:      verifier,
		FGA:      fgaClient,
		Sessions: sessions,
		Registry: sessionRegistry,
		MFA:      mfaSvc,
		Audit:    auditClient,
		Logger:   log,
	})
	autologinHandler := autologin.NewHandler(autologinSvc)

	// ─── Session introspection + logout ────────────────────────────────
	sessionHandler := session.NewHandler(sessions, fgaClient).
		WithRegistry(sessionRegistry, log).
		WithMFA(mfaSvc).
		WithAudit(auditClient).
		WithGIPLookup(cfg.GIPWebAPIKey, cfg.GIPInternalTenantID)

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	v1 := r.Group("/auth")
	autologinHandler.Register(v1)
	sessionHandler.Register(v1)

	// /api/v1 surface consumed by marketplace-api's account handler,
	// which proxies admin UI requests through to us. Kept separate from
	// /auth so the existing login + cookie routes stay untouched.
	apiV1 := r.Group("/api/v1")
	sessionHandler.RegisterAPI(apiV1)
	mfaHandler.Register(apiV1)

	// /internal surface for service-to-service calls. Guarded per-route
	// by the X-Internal-Auth header; the handler fails closed when the
	// secret is unset. marketplace-api calls DELETE /internal/users/:id
	// when the merchant triggers "reset my profile".
	internalGroup := r.Group("/internal")
	internalUsers := session.NewInternalUsersHandler(
		sessionRegistry,
		mfaSvc,
		cfg.MarketplaceInternalAuthSecret,
		log,
	)
	internalUsers.Register(internalGroup)

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}
