package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// checkTimeout bounds one comparison's network work. Generous, because
// nothing waits on it — but bounded, so a hung console cannot pin a
// goroutine indefinitely.
const checkTimeout = 30 * time.Second

// startCatalogParityRun launches the console/compiled catalog comparison
// (#304, #392), or does nothing when unconfigured.
//
// # It decides nothing
//
// Prices continue to come from internal/billing/pricing. This only reads the
// console's published catalog, compares it, and logs. The cutover is a
// separate, deliberate change gated on this reporting durably zero — the
// same evidence pattern the console's own parity check established.
//
// # Why a ticker and not a request hook
//
// BACKLOG §P: nothing on the request path of a customer payment may depend
// on the console being reachable. A comparison driven by requests would put
// the console one bad deploy away from the payment path even when cached.
// On its own ticker it cannot, whatever it does.
//
// # Unconfigured is a supported state, not a degraded one
//
// Absent credentials mean no goroutine, no reads, and behaviour identical to
// before this existed. That is deliberate: it makes enabling the parallel
// run a config change that can be reverted by removing config, with no code
// path that behaves differently once it is off.
func startCatalogParityRun(cfg *config.Config, log *slog.Logger) {
	cc := consolecatalog.Config{
		CatalogURL:   cfg.ConsoleCatalogURL,
		TokenURL:     cfg.ConsoleCatalogTokenURL,
		ClientID:     cfg.ConsoleCatalogClientID,
		ClientSecret: cfg.ConsoleCatalogClientSecret,
		Scope:        cfg.ConsoleCatalogScope,
		Mode:         cfg.ConsoleCatalogMode,
	}
	if !cc.Configured() {
		log.Info("consolecatalog: parallel run disabled (no console credentials); " +
			"prices come from the compiled catalog, as before")
		return
	}

	interval := cfg.ConsoleCatalogInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	monitor := consolecatalog.NewMonitor(consolecatalog.NewClient(cc, log), interval, log)
	log.Info("consolecatalog: parallel run enabled",
		"mode", cc.Mode, "interval", interval.String())

	go func() {
		// One check immediately, so a deploy produces evidence without
		// waiting out the first interval.
		runOneCheck(monitor, log)
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			runOneCheck(monitor, log)
		}
	}()
}

func runOneCheck(m *consolecatalog.Monitor, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	// The Monitor logs its own outcome; the result is ignored here precisely
	// because nothing in this process is allowed to act on it yet.
	_ = m.Check(ctx)
}
