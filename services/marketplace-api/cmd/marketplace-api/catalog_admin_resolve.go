package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// resolveTimeout bounds one Resolve's network work. Same reasoning as
// checkTimeout in catalog_parity.go: nothing waits on the outcome, so it can
// be generous, but it must be bounded or a hung console pins this goroutine
// forever.
const resolveTimeout = 30 * time.Second

// adminResolveLogMsg is the greppable marker for the evidence this whole file
// exists to produce. `kubectl logs deploy/mark8ly-marketplace-api-admin |
// grep 'catalog resolve'` is the check that the console read path works; keep
// the string stable, things outside this repo grep it.
const adminResolveLogMsg = "consolecatalog: catalog resolve"

// startAdminCatalogResolve exercises consolecatalog.Cache.Resolve on a ticker
// (tesserix-home#328 phase B). It returns whether it started a goroutine,
// which is what the wiring tests assert.
//
// # The gap it closes
//
// The parallel run (startCatalogParityRun) validates the DATA: it fetches the
// console catalog directly and diffs it against the compiled one. Nothing has
// ever exercised the code path phase C will actually serve from — the Cache:
// its cold start, its TTL refresh, its fail-open to last-known, its fallback
// to the compiled catalog. A missing credential, a blocked egress or a
// contract-guard rejection would first surface at the moment serving started
// depending on it. This ticker runs that path now, in production, while
// nothing depends on the answer.
//
// # Why the admin pod, and not the storefront
//
// Because every runtime reader of internal/billing/pricing is admin-side.
// `go list -deps ./internal/handlers/storefront` has no path to
// billing/pricing or to this package at all; the readers are
// internal/handlers/platformadmin (money.go), internal/subscription/planchange
// (the orchestrator and the downgrade-recheck cron, both wired inside the
// mode.Admin/mode.Both block in main.go) and internal/reconciliation. The
// plan catalog is what a MERCHANT pays Mark8ly. The storefront sells a
// merchant's products to that merchant's customers — a different pricing
// concept that never touches this catalog. Running this on the storefront
// would exercise a read path that pod will never take, at the cost of
// doubling the console's read load. So its absence there is deliberate;
// please do not "fix" the asymmetry.
//
// # It decides nothing
//
// The Resolution is logged and discarded. Prices still come from
// internal/billing/pricing through the existing call sites; phase C is what
// moves those onto Cache.Resolve. This is phase C's code path being
// EXERCISED, not taken — so a fault found here costs a log line, not a
// merchant's plan change.
//
// # Why this and the parity monitor both run, on the same pod
//
// They look redundant and are not. The monitor fetches DIRECTLY every 15m,
// so each run re-proves the console is reachable and its data still agrees
// with the compiled catalog. This resolver goes THROUGH the cache, so most
// ticks are memory hits and it proves something the monitor cannot: that the
// caching, degradation and fallback layer phase C will serve from behaves.
// Deliberately no Diff here — the monitor already does that comparison,
// against the same console catalog and the same compiled catalog in the same
// process, so repeating it would add no signal and double console load.
// Neither call site substitutes for the other; collapsing them loses one of
// the two properties.
//
// # Unconfigured is a supported state, not a degraded one
//
// Absent credentials mean one log line, no goroutine, and behaviour identical
// to before this existed — the same contract as startCatalogParityRun, and
// for the same reason: enabling and disabling this is a config change, with
// no code path that behaves differently once it is off.
func startAdminCatalogResolve(m mode.Mode, cfg *config.Config, log *slog.Logger) bool {
	// mode.Admin and mode.Both — the modes that mount the handlers and start
	// the crons which read the plan catalog. Asked through RunsAdmin rather
	// than by comparing constants so this cannot drift from what is actually
	// wired. main.go around line 613 records what the sloppy version of a
	// mode gate costs: the audit emitter was built inside the admin-mode
	// block, so MODE=storefront pods ran with a nil emitter and silently
	// dropped every storefront event.
	if !m.RunsAdmin() {
		return false
	}

	cc := consolecatalog.Config{
		CatalogURL:   cfg.ConsoleCatalogURL,
		TokenURL:     cfg.ConsoleCatalogTokenURL,
		ClientID:     cfg.ConsoleCatalogClientID,
		ClientSecret: cfg.ConsoleCatalogClientSecret,
		Scope:        cfg.ConsoleCatalogScope,
		Mode:         cfg.ConsoleCatalogMode,
	}
	if !cc.Configured() {
		log.Info("consolecatalog: cache read-path exercise disabled " +
			"(no console credentials); prices come from the compiled catalog, as before")
		return false
	}

	interval := cfg.ConsoleCatalogInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	// Ticking faster than the cache TTL is intentional and cheap: most ticks
	// are answered from memory (SourceFresh, no console call), so the console
	// still sees at most one read per TTL from this resolver, while every
	// tick republishes its state to the logs. The ticks that DO cross the TTL
	// are the ones that exercise the fetch.
	cache := consolecatalog.NewCache(
		consolecatalog.NewClient(cc, log), cfg.ConsoleCatalogCacheTTL, cc.Mode, log)

	log.Info("consolecatalog: cache read-path exercise enabled",
		"mode", cc.Mode, "interval", interval.String(),
		"cache_ttl", cfg.ConsoleCatalogCacheTTL.String())

	go func() {
		// One resolve immediately. This is the interesting one: an empty
		// cache means it takes the cold-start path — token, fetch, and on
		// failure the compiled fallback — which is exactly what a freshly
		// rolled pod would do at the first plan change after the cutover.
		resolveOnceAndLog(cache, log)
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			resolveOnceAndLog(cache, log)
		}
	}()
	return true
}

// resolveOnceAndLog performs one Resolve and logs its outcome, which is the
// entire product of this file. The Resolution is returned only so tests can
// assert on it; the caller above discards it, because nothing in this process
// is allowed to act on a price from here yet.
//
// Resolve never returns an error — a degradation is reported through Source
// and Err — so this cannot fail, only report.
func resolveOnceAndLog(c *consolecatalog.Cache, log *slog.Logger) consolecatalog.Resolution {
	return resolveWithin(resolveTimeout, c, log)
}

// resolveWithin is resolveOnceAndLog with the deadline as a parameter, so a
// test can prove the deadline actually reaches the fetcher without waiting
// out the production one.
func resolveWithin(timeout time.Duration, c *consolecatalog.Cache, log *slog.Logger) consolecatalog.Resolution {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res := c.Resolve(ctx)

	attrs := []any{
		"source", string(res.Source),
		"stale", res.Stale,
		"revision_id", res.Catalog.RevisionID,
		"prices", len(res.Catalog.Prices),
		"revision_unexpected", res.RevisionUnexpected,
	}
	if res.Err != nil {
		attrs = append(attrs, "error", res.Err)
	}
	if log != nil {
		// Info for a fresh read, warn for anything else. Every non-fresh
		// resolve is a rehearsal of a post-cutover degradation, so it should
		// be findable without knowing to look: after phase C the same line at
		// warn means a plan change was priced from a stale or compiled
		// catalog rather than from the console.
		if res.Source == consolecatalog.SourceFresh {
			log.Info(adminResolveLogMsg, attrs...)
		} else {
			log.Warn(adminResolveLogMsg, attrs...)
		}
	}
	return res
}
