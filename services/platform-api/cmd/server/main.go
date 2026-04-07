// Command server is the platform-api HTTP entrypoint.
//
// It does NOT run migrations. On startup it asserts that the DB is at the
// expected schema version and refuses to start otherwise. This is the safety
// net that ensures the API never runs against a wrong schema.
package main

import (
	"context"
	"os/signal"
	"syscall"

	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/location"
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

	// Wire up domains. Each domain owns its handler/service/repo trio and
	// registers its routes onto the /api/v1 group.
	locationHandler := location.NewHandler(location.NewService(location.NewRepository(conn)))

	r := httpserver.New(cfg.Env, log)
	v1 := r.Group("/api/v1")
	locationHandler.Register(v1)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}
