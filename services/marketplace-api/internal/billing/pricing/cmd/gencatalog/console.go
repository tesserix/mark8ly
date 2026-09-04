package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// consoleFetchTimeout bounds one -source=console run. This is a developer
// command, not a request path, so the only thing the bound protects is a
// human waiting at a terminal.
const consoleFetchTimeout = 30 * time.Second

// catalogFetcher is what this command needs of consolecatalog.Client: one
// read of one mode's catalog. Narrowed to an interface so the mapping below
// can be tested against a client pointed at a fixture server.
type catalogFetcher interface {
	Fetch(ctx context.Context) (consolecatalog.Catalog, error)
}

// consoleConfigFromEnv reads the same CONSOLE_CATALOG_* variables the service
// itself runs on, so a run of this command uses exactly the credentials and
// endpoint the parity monitor uses and cannot silently read somewhere else.
//
// Env rather than flags because the secret is one of them: a client secret
// passed on a command line lands in shell history and in ps output.
func consoleConfigFromEnv() consolecatalog.Config {
	cfg := consolecatalog.Config{
		CatalogURL:   os.Getenv("CONSOLE_CATALOG_URL"),
		TokenURL:     os.Getenv("CONSOLE_CATALOG_TOKEN_URL"),
		ClientID:     os.Getenv("CONSOLE_CATALOG_CLIENT_ID"),
		ClientSecret: os.Getenv("CONSOLE_CATALOG_CLIENT_SECRET"),
		Scope:        os.Getenv("CONSOLE_CATALOG_SCOPE"),
		Mode:         os.Getenv("CONSOLE_CATALOG_MODE"),
	}
	if cfg.Mode == "" {
		cfg.Mode = "test"
	}
	return cfg
}

// generateFromConsole reads the catalog from the console and renders
// catalog_data.go from it.
//
// Every failure here returns an error and produces no file. A console that is
// unreachable, unpublished, or answering short must not be able to shrink the
// fallback catalog: it is what the serving path drops to when the console is
// down, so degrading it during a console outage is the one thing this command
// must never do.
func generateFromConsole(ctx context.Context, client catalogFetcher) (string, error) {
	catalog, err := client.Fetch(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the console catalog: %w", err)
	}
	return pricing.GenerateCatalogDataFromRows(rowsFromConsole(catalog))
}

// rowsFromConsole restates the console's rows in the generator's shape. It is
// a field copy and nothing more: every value the generator needs is stated by
// the payload, and lookup_key is deliberately dropped because catalog.go
// derives keys itself.
func rowsFromConsole(catalog consolecatalog.Catalog) []pricing.CatalogRow {
	rows := make([]pricing.CatalogRow, 0, len(catalog.Prices))
	for _, p := range catalog.Prices {
		rows = append(rows, pricing.CatalogRow{
			Plan:            p.Plan,
			Period:          p.Period,
			Tier:            p.Tier,
			Currency:        p.Currency,
			UnitAmountMinor: p.UnitAmountMinor,
			TaxBehavior:     p.TaxBehavior,
		})
	}
	return rows
}

// newConsoleClient builds the real client. consolecatalog logs through an
// slog.Logger; this command discards those records so the generated file can
// still be written to stdout unpolluted.
func newConsoleClient(cfg consolecatalog.Config) *consolecatalog.Client {
	return consolecatalog.NewClient(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
