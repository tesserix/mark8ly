package main

import (
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// newServingCatalogResolver builds the cache the platform console's billing
// READ routes price from (tesserix-home#328 phase C, step 1), or an
// explicitly nil interface when the console catalog is unconfigured.
//
// # Unconfigured returns a TRUE nil, not a nil pointer in an interface
//
// The unconfigured path returns the untyped nil literal rather than a nil
// *consolecatalog.Cache. A nil pointer assigned into an interface field is a
// NON-nil interface value, so platformadmin's `r == nil` check would miss it
// and the first request would call Resolve on a nil *Cache and panic on its
// mutex. main.go already records this exact trap for tenantTeardownClient
// (#323); it is the same shape here, with a worse blast radius, because the
// nil case is the ROLLBACK path — the one that has to work when everything
// else is broken.
//
// # Why this is a second cache, beside startAdminCatalogResolve's
//
// The phase-B ticker owns its own Cache and keeps it, deliberately. Its job
// is to exercise and report on the read path on a fixed schedule whether or
// not anyone is using the console; this one's job is to answer requests.
// Sharing one instance would make the ticker's refresh schedule part of the
// serving path's behaviour and its log lines ambiguous about which cache
// they describe. The cost of not sharing is one extra console read per pod
// per cache TTL (6h by default) — see consolecatalog.DefaultTTL for why that
// interval is generous rather than tight.
//
// # It is not gated on mode here
//
// Both platformadmin.Register call sites are already inside admin-mode
// blocks, so a storefront pod never reaches this. The ticker carries its own
// m.RunsAdmin() gate because it is started from a block that does not
// otherwise state one.
func newServingCatalogResolver(cfg *config.Config, log *slog.Logger) platformadmin.CatalogResolver {
	cc := consolecatalog.Config{
		CatalogURL:   cfg.ConsoleCatalogURL,
		TokenURL:     cfg.ConsoleCatalogTokenURL,
		ClientID:     cfg.ConsoleCatalogClientID,
		ClientSecret: cfg.ConsoleCatalogClientSecret,
		Scope:        cfg.ConsoleCatalogScope,
		Mode:         cfg.ConsoleCatalogMode,
	}
	if !cc.Configured() {
		log.Info("consolecatalog: platform-admin billing amounts come from the compiled " +
			"catalog (no console credentials); this is the supported rollback state")
		return nil
	}

	log.Info("consolecatalog: platform-admin billing amounts resolve from the console catalog",
		"mode", cc.Mode, "cache_ttl", cfg.ConsoleCatalogCacheTTL.String())
	return consolecatalog.NewCache(
		consolecatalog.NewClient(cc, log), cfg.ConsoleCatalogCacheTTL, cc.Mode, log)
}
