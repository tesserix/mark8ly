package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/consolepromo"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// promoIngestTimeout bounds one sync's work. Same reasoning as checkTimeout
// in catalog_parity.go: nothing waits on the outcome, so it can be generous,
// but it must be bounded or a hung console pins this goroutine forever.
const promoIngestTimeout = 30 * time.Second

// promoIngestLogMsg is the greppable marker for one completed ingest.
const promoIngestLogMsg = "consolepromo: promo catalog ingest"

// promoMetricsOnce guards the process-global metric registration below.
var promoMetricsOnce sync.Once

// startPromoCatalogIngest pulls the console's promo-code definitions into
// promo_codes at boot and on a ticker (#726 step 2). It returns whether it
// started a goroutine, which is what the wiring tests assert.
//
// # Why rows in a table rather than a cache
//
// promo_redemptions.promo_code_id is NOT NULL REFERENCES promo_codes(id), so
// a redemption must point at a real row. Unlike the plan catalog, this one
// cannot live in memory. See the consolepromo package doc.
//
// # Why the admin pod
//
// The promo surfaces are admin-side (internal/handlers/admin/promo.go), and
// #620's redemption lands there too. Running the ingest on the storefront
// would double the console's read load and double the writers to one table
// for a path that pod never takes. Asked through RunsAdmin rather than by
// comparing mode constants, so it cannot drift from what is actually wired.
//
// # Unconfigured is a supported state, not a degraded one
//
// No URL means one log line, no goroutine, no metrics registered, and
// behaviour identical to before this existed. Nor can a configured-but-
// unreachable console fail startup: the first sync happens inside the
// goroutine, its failure is logged and counted, and any rows a previous
// ingest wrote stay exactly as they are. Nothing on a startup path waits on
// the console.
func startPromoCatalogIngest(m mode.Mode, cfg *config.Config, db *gorm.DB, log *slog.Logger) bool {
	if !m.RunsAdmin() || db == nil {
		return false
	}

	cc := consolepromo.Config{
		CatalogURL:   cfg.ConsolePromoCatalogURL,
		TokenURL:     cfg.ConsoleCatalogTokenURL,
		ClientID:     cfg.ConsoleCatalogClientID,
		ClientSecret: cfg.ConsoleCatalogClientSecret,
		Scope:        cfg.ConsoleCatalogScope,
		Mode:         cfg.ConsoleCatalogMode,
	}
	if !cc.Configured() {
		log.Info("consolepromo: promo catalog ingest disabled (no CONSOLE_PROMO_CATALOG_URL " +
			"or console credentials); promo_codes is left exactly as it is")
		return false
	}

	interval := cfg.ConsoleCatalogInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	// After the gate, for the reason given on MustRegisterMetrics: a
	// LastSuccessTimestamp published at zero on a pod that deliberately does
	// not ingest would read as "last ingested in 1970" forever.
	//
	// Through a Once because registration is a process-global side effect
	// while this function is otherwise pure wiring: production calls it once,
	// but the mode-matrix test calls it several times and a duplicate
	// registration panics. The Once removes that coupling without weakening
	// the rule above — the first call still happens only after the gate.
	promoMetricsOnce.Do(func() {
		consolepromo.MustRegisterMetrics(prometheus.DefaultRegisterer)
	})

	syncer := consolepromo.NewSyncer(
		consolepromo.NewClient(cc, log), consolepromo.NewStore(db), log)

	log.Info("consolepromo: promo catalog ingest enabled",
		"mode", cc.Mode, "interval", interval.String())

	go func() {
		syncOnceAndLog(syncer, log)
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			syncOnceAndLog(syncer, log)
		}
	}()
	return true
}

// syncOnceAndLog performs one ingest and reports it. It never returns an
// error: a failed ingest is a logged, counted event, not something any
// caller here can act on — the correct response to an unreadable catalog is
// to change nothing and try again on the next tick.
func syncOnceAndLog(s *consolepromo.Syncer, log *slog.Logger) consolepromo.Result {
	return syncWithin(promoIngestTimeout, s, log)
}

// syncWithin is syncOnceAndLog with the deadline as a parameter, so a test
// can prove the deadline reaches the fetcher without waiting out the
// production one.
func syncWithin(timeout time.Duration, s *consolepromo.Syncer, log *slog.Logger) consolepromo.Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, err := s.Sync(ctx)
	if err != nil {
		consolepromo.SyncFailuresTotal.Inc()
		if log != nil {
			log.Warn(promoIngestLogMsg+" failed; promo_codes is unchanged", "error", err)
		}
		return res
	}
	if log != nil {
		log.Info(promoIngestLogMsg,
			"revision_id", res.RevisionID,
			"ingested", res.Ingested,
			"skipped", res.Skipped,
			"expired", res.Expired)
	}
	return res
}
