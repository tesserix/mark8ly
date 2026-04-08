// Command server is the auth-bff HTTP entrypoint.
//
// Wires GIP verifier + OpenFGA client + session manager + autologin into
// the HTTP server. The session manager and OpenFGA store ID are constructed
// at startup; the schema version is asserted before the server binds.
package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	authbff "github.com/mark8ly/auth-bff"
	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/autologin"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/session"
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

	// ─── Autologin ─────────────────────────────────────────────────────
	autologinSvc := autologin.NewService(autologin.Config{
		GIP:      verifier,
		FGA:      fgaClient,
		Sessions: sessions,
	})
	autologinHandler := autologin.NewHandler(autologinSvc)

	// ─── Session introspection + logout ────────────────────────────────
	sessionHandler := session.NewHandler(sessions, fgaClient)

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	v1 := r.Group("/auth")
	autologinHandler.Register(v1)
	sessionHandler.Register(v1)

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}
