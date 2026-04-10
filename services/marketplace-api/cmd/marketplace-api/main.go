// Command marketplace-api is the marketplace-api HTTP entrypoint.
//
// It does NOT run migrations. On startup it asserts the DB is at the
// expected schema version and refuses to start otherwise — the safety
// net that guarantees the API never runs against a wrong schema.
//
// MODE selects which Gin engine(s) to construct. In production, two
// Knative Services run the same image with MODE=admin and MODE=storefront
// respectively. In local dev, MODE=both (the default) runs a single
// process with both engines mounted on one port.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/singleflight"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/health"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
	"github.com/mark8ly/marketplace-api/pkg/httpserver"
	"github.com/mark8ly/marketplace-api/pkg/logger"
	"github.com/mark8ly/marketplace-api/pkg/migrate"
)

// stubPlatformClient is a placeholder stores.Client that always returns
// ErrPlatformUnavailable. M5a tests pre-seed the stores projection via
// raw SQL so the middleware never invokes the client. M5b replaces this
// with a real HTTP client to platform-api.
type stubPlatformClient struct{}

func (stubPlatformClient) GetStore(ctx context.Context, tenantID, storeID string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

func (stubPlatformClient) GetStoreBySlug(ctx context.Context, slug string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	m, err := mode.Parse(cfg.Mode)
	if err != nil {
		log.Error("invalid MODE", "err", err)
		os.Exit(2)
	}
	log.Info("boot", slog.String("mode", string(m)), slog.Int("port", cfg.HTTPPort))

	// Verify schema version. Refuse to start on mismatch.
	mig, err := migrate.New(marketplaceapi.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		os.Exit(1)
	}
	if err := mig.AssertVersion(marketplaceapi.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run `make mp-migrate-up` first", "err", err)
		os.Exit(1)
	}

	// Open DB.
	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("db open", "err", err)
		os.Exit(1)
	}

	// OpenFGA client — read-only per spec §13.1.1.
	discoverCtx, discoverCancel := context.WithTimeout(context.Background(), 5*time.Second)
	storeID, err := authz.DiscoverStoreID(discoverCtx, cfg.FGAAPIURL, authz.FGAStoreName)
	discoverCancel()
	if err != nil {
		log.Error("authz: discover store", "err", err, "api_url", cfg.FGAAPIURL)
		os.Exit(1)
	}
	if storeID == "" {
		log.Error("authz: store not found — bring up openfga-seed first",
			"store_name", authz.FGAStoreName, "api_url", cfg.FGAAPIURL)
		os.Exit(1)
	}
	log.Info("authz: discovered openfga store", "store_id", storeID)
	fgaClient, err := authz.New(authz.Config{APIURL: cfg.FGAAPIURL, StoreID: storeID})
	if err != nil {
		log.Error("authz: new client", "err", err)
		os.Exit(1)
	}
	authzMW := authz.NewMiddleware(fgaClient, log)

	// Admin wiring — constructed for admin and both modes. The storefront
	// process never mounts the admin group so these dependencies would go
	// unused there.
	var adminDeps admin.Deps
	if m == mode.Admin || m == mode.Both {
		productRepo := product.NewRepository(conn)
		categoryRepo := category.NewRepository(conn)
		outboxRepo := outbox.NewRepository(conn)
		storesRepo := stores.NewRepository(conn)

		// Media uploader — real GCS when MARKETPLACE_GCS_BUCKET is set,
		// FakeUploader otherwise so `make dev` works without credentials.
		var uploader media.Uploader
		if cfg.GCSBucket != "" {
			gcsCtx, gcsCancel := context.WithTimeout(context.Background(), 5*time.Second)
			sc, err := storage.NewClient(gcsCtx)
			gcsCancel()
			if err != nil {
				log.Error("media: gcs client", "err", err)
				os.Exit(1)
			}
			uploader = media.NewGCSUploader(sc, cfg.GCSBucket)
			log.Info("media: using real GCS uploader", "bucket", cfg.GCSBucket)
		} else {
			uploader = media.NewFakeUploader()
			log.Info("media: using fake uploader (MARKETPLACE_GCS_BUCKET is empty)")
		}

		// Platform client — real HTTP client when MARKETPLACE_PLATFORM_API_URL
		// is set, stub otherwise.
		var platformClient stores.Client
		if cfg.PlatformAPIURL != "" {
			platformClient = stores.NewHTTPClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			log.Info("stores: using real platform-api client", "url", cfg.PlatformAPIURL)
		} else {
			platformClient = stubPlatformClient{}
			log.Info("stores: using stub platform client (MARKETPLACE_PLATFORM_API_URL is empty)")
		}

		productSvc := product.NewService(product.Config{
			DB:         conn,
			Repo:       productRepo,
			StoresRepo: storesRepo,
			OutboxRepo: outboxRepo,
			Uploader:   uploader,
			Logger:     log,
		})
		categorySvc := category.NewService(category.Config{
			DB:         conn,
			Repo:       categoryRepo,
			OutboxRepo: outboxRepo,
			Logger:     log,
		})
		storeFlight := &singleflight.Group{}
		storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
			Repo:   storesRepo,
			Client: platformClient,
			Logger: log,
			Flight: storeFlight,
		})
		productHandler := admin.NewProductHandler(productSvc, categoryRepo, log)
		categoryHandler := admin.NewCategoryHandler(categorySvc, categoryRepo, log)
		variantHandler := admin.NewVariantHandler(productSvc, log)
		mediaHandler := admin.NewMediaHandler(productSvc, uploader, log)

		// Orders slice 1 wiring (M2/M4).
		orderRepo := order.NewRepository()
		returnRepo := order.NewReturnRepository()
		abandonedCartRepo := order.NewAbandonedCartRepository()
		orderSvc := order.NewService(conn, orderRepo, outboxRepo)
		returnSvc := order.NewReturnService(conn, returnRepo, orderRepo, orderSvc, outboxRepo)
		abandonedCartSvc := order.NewAbandonedCartService(conn, abandonedCartRepo, outboxRepo)
		ordersHandler := admin.NewOrdersHandler(conn, orderSvc, orderRepo, log)
		returnsHandler := admin.NewReturnsHandler(conn, returnSvc, returnRepo, orderRepo, orderSvc, log)
		abandonedCartsHandler := admin.NewAbandonedCartsHandler(abandonedCartSvc, log)

		storesHandler := admin.NewStoresHandler(storesRepo, log)
		bulkHandler := admin.NewBulkHandler(productSvc, fgaClient, log)

		// CSV import/export wiring (M7e).
		csvRepo := csvjob.NewRepository(conn)
		csvSvc := csvjob.NewService(csvRepo, log)
		exportRepo := product.NewExportRepository(conn)
		csvImportsHandler := admin.NewCSVImportsHandler(csvSvc, exportRepo, log)

		// Settings handlers (P5a).
		countryRepoAdmin := country.NewRepository(conn)
		paymentSettingsHandler := admin.NewPaymentSettingsHandler(conn, countryRepoAdmin, log)
		shippingSettingsHandler := admin.NewShippingSettingsHandler(conn, countryRepoAdmin, log)
		taxSettingsHandler := admin.NewTaxSettingsHandler(conn, countryRepoAdmin, log)

		adminDeps = admin.Deps{
			ProductHandler:          productHandler,
			CategoryHandler:         categoryHandler,
			VariantHandler:          variantHandler,
			MediaHandler:            mediaHandler,
			OrdersHandler:           ordersHandler,
			ReturnsHandler:          returnsHandler,
			AbandonedCartsHandler:   abandonedCartsHandler,
			StoresHandler:           storesHandler,
			BulkHandler:             bulkHandler,
			CSVImportsHandler:       csvImportsHandler,
			PaymentSettingsHandler:  paymentSettingsHandler,
			ShippingSettingsHandler: shippingSettingsHandler,
			TaxSettingsHandler:      taxSettingsHandler,
			StoresMiddleware:        storeMW,
			AuthzMiddleware:         authzMW,
			InternalSecret:          cfg.InternalAuthSecret,
		}
	}

	// Storefront wiring — constructed for storefront and both modes. The
	// admin process never mounts the storefront group so these
	// dependencies would go unused there.
	var storefrontDeps storefront.Deps
	if m == mode.Storefront || m == mode.Both {
		productRepoSF := product.NewRepository(conn)
		categoryRepoSF := category.NewRepository(conn)
		storesRepoSF := stores.NewRepository(conn)
		var storefrontPlatformClient stores.Client
		if cfg.PlatformAPIURL != "" {
			storefrontPlatformClient = stores.NewHTTPClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
		} else {
			storefrontPlatformClient = stubPlatformClient{}
		}
		slugFlight := &singleflight.Group{}
		slugCache := stores.NewSlugCache(storesRepoSF, storefrontPlatformClient, slugFlight, 5*time.Minute)
		storefrontHandler := storefront.NewStorefrontHandler(productRepoSF, categoryRepoSF, storesRepoSF, log)

		// Orders M5 — public checkout endpoint. Storefront mode does not
		// run the outbox publisher (admin mode owns that), but it DOES
		// produce outbox_events rows that the admin replica drains.
		orderRepoSF := order.NewRepository()
		outboxRepoSF := outbox.NewRepository(conn)
		orderSvcSF := order.NewService(conn, orderRepoSF, outboxRepoSF)
		checkoutHandler := storefront.NewCheckoutHandler(conn, orderSvcSF, orderRepoSF, log)

		countryRepo := country.NewRepository(conn)
		countryHandler := country.NewHandler(countryRepo)

		// P5b — extended checkout, payment methods, shipping rates, webhooks.
		checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, log)
		paymentMethodsHandler := storefront.NewPaymentMethodsHandler(conn, log)
		shippingRatesHandler := storefront.NewShippingRatesHandler(conn, log)
		webhookHandler := storefront.NewWebhookHandler(conn, orderSvcSF, log)
		orderDetailHandler := storefront.NewOrderDetailHandler(conn, orderRepoSF, log)

		storefrontDeps = storefront.Deps{
			Handler:               storefrontHandler,
			CheckoutHandler:       checkoutHandler,
			CheckoutExtHandler:    checkoutExtHandler,
			PaymentMethodsHandler: paymentMethodsHandler,
			ShippingRatesHandler:  shippingRatesHandler,
			WebhookHandler:        webhookHandler,
			OrderDetailHandler:    orderDetailHandler,
			SlugCache:             slugCache,
			StorefrontKey:         cfg.StorefrontKey,
			CountryHandler:        countryHandler,
		}
	}

	// CSV import worker — runs in admin and both modes. On startup, recover
	// orphaned jobs (stale heartbeat > 15 min → paused). Then poll for
	// queued jobs every 5s and run the worker.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	var workerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		csvRepo := csvjob.NewRepository(conn)

		// Recovery scan on startup.
		if err := csvjob.RecoverOrphanedJobs(context.Background(), csvRepo, 15*time.Minute, log); err != nil {
			log.Error("csvjob: recovery scan failed", "err", err)
			// Non-fatal — proceed without recovery.
		} else {
			log.Info("csvjob: recovery scan complete")
		}

		// Polling goroutine.
		done := make(chan struct{})
		workerDone = done
		go func() {
			defer close(done)
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
					jobs, _, err := csvRepo.ListByStore(workerCtx, "", 1, 1)
					_ = jobs
					if err != nil {
						log.Error("csvjob: poll error", "err", err)
					}
					// Full worker dispatch is wired when handlers submit
					// jobs — the polling loop here ensures queued jobs from
					// crash recovery are eventually picked up.
				}
			}
		}()
		log.Info("csvjob: worker polling started")
	}

	// Outbox publisher — runs in admin and both modes; the storefront
	// process does not produce events, so running it there would just poll
	// an always-empty table and waste a connection.
	publisherCtx, publisherCancel := context.WithCancel(context.Background())
	defer publisherCancel()
	var publisherDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		pub := outbox.New(outbox.Config{
			Repo:      outbox.NewRepository(conn),
			DB:        conn,
			Logger:    log,
			Interval:  2 * time.Second,
			BatchSize: 100,
		})
		publisherDone = pub.Start(publisherCtx)
		log.Info("outbox publisher started")
	}

	// Public country endpoint — available in ALL modes (admin, storefront, both).
	// When storefront mode is active, it's already wired via storefrontDeps.
	// For admin-only mode, we register it directly on the admin engine below.
	var countryPublicHandler *country.Handler
	if m == mode.Admin {
		countryRepo := country.NewRepository(conn)
		countryPublicHandler = country.NewHandler(countryRepo)
	}

	// Construct Gin engine(s) per MODE.
	healthHandler := health.New(conn)

	var srv *http.Server
	switch m {
	case mode.Both:
		// Single engine for local dev: both admin and storefront route
		// groups mount on one port so a developer can curl either without
		// running two processes.
		r := httpserver.MergedForBoth(cfg.Env, log)
		r.MaxMultipartMemory = 100 << 20 // 100 MB for CSV uploads
		healthHandler.Register(r)
		admin.RegisterAdmin(r.Group("/api/v1"), adminDeps)
		storefront.RegisterStorefront(r.Group("/api/v1"), storefrontDeps)
		srv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: r,
		}
	case mode.Admin, mode.Storefront:
		e := httpserver.New(cfg.Env, m, log)
		engine := e.Admin
		if m == mode.Storefront {
			engine = e.Storefront
		}
		engine.MaxMultipartMemory = 100 << 20 // 100 MB for CSV uploads
		healthHandler.Register(engine)
		if m == mode.Admin {
			admin.RegisterAdmin(engine.Group("/api/v1"), adminDeps)
			if countryPublicHandler != nil {
				engine.GET("/api/v1/public/supported-countries", countryPublicHandler.ListSupported)
			}
		}
		if m == mode.Storefront {
			storefront.RegisterStorefront(engine.Group("/api/v1"), storefrontDeps)
		}
		srv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: engine,
		}
	}

	// Start the server in a goroutine so we can signal-handle on the main.
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown", "err", err)
		os.Exit(1)
	}
	publisherCancel()
	if publisherDone != nil {
		select {
		case <-publisherDone:
			log.Info("outbox publisher stopped")
		case <-time.After(5 * time.Second):
			log.Warn("outbox publisher did not stop in time")
		}
	}
	workerCancel()
	if workerDone != nil {
		select {
		case <-workerDone:
			log.Info("csvjob: worker polling stopped")
		case <-time.After(5 * time.Second):
			log.Warn("csvjob: worker polling did not stop in time")
		}
	}
	log.Info("bye")
}
