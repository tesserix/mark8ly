// Command server is the auth-bff HTTP entrypoint.
package main

import (
	"context"
	"os/signal"
	"syscall"

	authbff "github.com/mark8ly/auth-bff"
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

	mig, err := migrate.New(authbff.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		panic(err)
	}
	if err := mig.AssertVersion(authbff.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run migrations first", "err", err)
		panic(err)
	}

	r := httpserver.New(cfg.Env, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}
