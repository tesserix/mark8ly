package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
// # Why this fetches directly and NOT through consolecatalog.Cache
//
// Deliberate, and revisited when the cache landed (tesserix-home#328). This
// monitor's output is evidence of two things: that the console's data agrees
// with the compiled catalog, and that the console is reachable from this
// service every interval. Routing it through the Cache would destroy the
// second: with a 6h TTL and a 15m interval, ~23 of every 24 checks would be
// answered from memory and would re-compare bytes already compared, while a
// console that went down would keep producing "parity clean" until the TTL
// expired. A monitor that reports healthy through an outage is worse than no
// monitor.
//
// The cache is exercised instead by startAdminCatalogResolve in
// catalog_admin_resolve.go, which runs on this same pod and has the opposite
// job: it proves the READ PATH — token, fetch, cache, fail-open, compiled
// fallback — works, which is what phase C will serve from. Two jobs, two
// call sites on one pod, neither substituting for the other. When phase C
// moves serving onto the Cache, that is where the cached read belongs; this
// monitor should keep reading directly for as long as it exists.
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

	// Registered here rather than beside campaignbudget.MustRegisterMetrics in
	// main.go, and specifically AFTER the credentials gate above. A registered
	// gauge publishes zero at once, and a "last successful comparison" of zero
	// reads as 1970 — so registering on a pod where the parallel run is
	// switched off would keep the staleness alert firing against a deployment
	// doing exactly what it is configured to do. Absent series say "not
	// enabled here"; present-and-stale says "enabled and not working".
	//
	// Safe to call once: this function has a single call site, in main.go's
	// admin-mode block.
	consolecatalog.MustRegisterMetrics(prometheus.DefaultRegisterer)

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
