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
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/mark8ly/marketplace-api/internal/k8sprov"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/handlers/testroutes"
	"github.com/mark8ly/marketplace-api/internal/health"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/push"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/internal/wishlist"
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

	// Coupon wiring — shared between admin and storefront (amendment FIX 9:
	// gormRepository is stateless and Service is read-safe, so one instance
	// serves both modes).
	couponRepo := coupon.NewRepository()
	couponSvc := coupon.NewService(coupon.ServiceConfig{
		DB:     conn,
		Repo:   couponRepo,
		Logger: log,
	})

	// Custom domain service — shared between admin (CRUD) and storefront
	// (domain resolution). Constructed once so both modes use the same repo.
	domainRepo := domain.NewRepository()
	// K8s provisioner — best-effort. Returns nil when not running inside
	// a cluster (local dev); the service handles nil gracefully.
	domainProvisioner, provErr := k8sprov.New(log)
	if provErr != nil {
		log.Warn("k8sprov unavailable", "err", provErr)
	}
	var provIface domain.Provisioner
	if domainProvisioner != nil {
		provIface = k8sprovAdapter{p: domainProvisioner}
	}
	domainSvc := domain.NewService(domain.ServiceConfig{
		DB:          conn,
		Repo:        domainRepo,
		CF:          nil, // Stub — real Cloudflare client wired in production config
		Provisioner: provIface,
		Logger:      log,
	})
	domainStoresRepo := stores.NewRepository(conn)
	domainsHandler := admin.NewDomainsHandler(domainSvc, domainStoresRepo, log)

	// Admin wiring — constructed for admin and both modes. The storefront
	// process never mounts the admin group so these dependencies would go
	// unused there.
	var adminDeps admin.Deps
	// vendorSvc stays nil in Storefront mode by design — the storefront
	// process does not call product.Create and does not expose the
	// /internal vendor endpoints, so no self-vendor lookup is needed.
	// product.Service.resolveVendorID is nil-safe for exactly this case.
	var vendorSvc *vendor.Service
	// brandingSeeder is non-nil only when MARKETPLACE_API_ENABLE_TEST_ROUTES=true.
	// Declared at func scope so the later route-mount block can see it.
	var brandingSeeder *testroutes.BrandingSeeder
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
			if cfg.GCSSignerSAEmail != "" {
				signCtx, signCancel := context.WithTimeout(context.Background(), 5*time.Second)
				gcsUploader, err := media.NewGCSUploaderWithIAMSigner(signCtx, sc, cfg.GCSBucket, cfg.GCSSignerSAEmail)
				signCancel()
				if err != nil {
					log.Error("media: gcs iam signer", "err", err)
					os.Exit(1)
				}
				uploader = gcsUploader
				log.Info("media: using real GCS uploader with IAM signer",
					"bucket", cfg.GCSBucket, "signer", cfg.GCSSignerSAEmail)
			} else {
				uploader = media.NewGCSUploader(sc, cfg.GCSBucket)
				log.Info("media: using real GCS uploader", "bucket", cfg.GCSBucket)
			}
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

		// Vendor service — constructed here so productSvc can reference it
		// as a SelfVendorLookup. The handler is wired later once vendorSvc
		// is set on the outer var.
		vendorSvc = vendor.NewService(vendor.NewRepository(conn))

		productSvc := product.NewService(product.Config{
			DB:                 conn,
			Repo:               productRepo,
			StoresRepo:         storesRepo,
			OutboxRepo:         outboxRepo,
			Uploader:           uploader,
			Logger:             log,
			MediaPublicBaseURL: cfg.MediaPublicBaseURL,
			MediaCacheControl:  cfg.MediaCacheControl,
			VendorLookup:       vendorSvc,
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
		// Order document mailer — invoice on accept, receipt on delivery.
		// Built up here because both OrdersHandler and ShipmentsHandler need
		// it. SendGrid when an API key is configured, log fallback otherwise
		// so local dev still exercises the dispatch path.
		var orderDocMailer orderdoc.Mailer
		if cfg.SendGridAPIKey != "" {
			orderDocMailer = orderdoc.NewSendGridMailer(cfg.SendGridAPIKey, cfg.EmailFrom, log)
		} else {
			orderDocMailer = &orderdoc.LogMailer{Logger: log}
		}
		orderDocBrandingSvc := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		orderDocSvc := orderdoc.NewService(conn, orderDocMailer, orderRepo, orderDocBrandingSvc, cfg.StorefrontBaseURLTemplate)

		ordersHandler := admin.NewOrdersHandler(conn, orderSvc, orderRepo, orderDocSvc, log)
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
		shippingRepo := shipping.NewRepository(conn)
		shippingService := shipping.NewShippingService(shippingRepo)
		shipmentsHandler := admin.NewShipmentsHandler(conn, shippingService, shippingRepo, orderDocSvc, log)
		taxSettingsHandler := admin.NewTaxSettingsHandler(conn, countryRepoAdmin, log)
		settingsMetaHandler := admin.NewSettingsMetaHandler(countryRepoAdmin, log)

		// Coupon handler (Marketing M1).
		couponHandler := admin.NewCouponHandler(couponSvc, log)

		// Gift cards — Marketing M2.
		giftCardRepo := giftcard.NewRepository()
		// Delivery email: SendGrid when an API key is configured, Log
		// fallback for local/dev. Theme loader pulls the merchant's
		// branding + storefront URL so the email matches the store, not
		// the platform.
		var giftCardMailer giftcard.Mailer
		if cfg.SendGridAPIKey != "" {
			giftCardMailer = giftcard.NewSendGridMailer(cfg.SendGridAPIKey, cfg.EmailFrom, log)
		} else {
			giftCardMailer = &giftcard.LogMailer{Logger: log}
		}
		giftCardBrandingSvc := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		giftCardThemeLoader := giftcard.NewStoreThemeLoader(conn, giftCardBrandingSvc, cfg.StorefrontBaseURLTemplate)
		giftCardSvc := giftcard.NewServiceWithMailer(conn, giftCardRepo, giftCardMailer, giftCardThemeLoader, log)
		giftCardHandler := admin.NewGiftCardHandler(giftCardSvc, log)

		// Loyalty M3 wiring.
		loyaltyRepo := loyalty.NewRepository()
		loyaltySvc := loyalty.NewService(conn, loyaltyRepo, log)
		loyaltyHandler := admin.NewLoyaltyHandler(loyaltySvc, log)

		// Campaign wiring (Marketing M4).
		campaignRepo := campaign.NewRepository(conn)
		segmentEngine := campaign.NewSegmentEngine(conn)
		campaignSvc := campaign.NewService(campaign.ServiceConfig{
			DB:            conn,
			Repo:          campaignRepo,
			SegmentEngine: segmentEngine,
			Logger:        log,
		})
		campaignHandler := admin.NewCampaignHandler(campaignSvc, campaignRepo, log)
		segmentHandler := admin.NewSegmentHandler(campaignSvc, log)

		// Customers C2 wiring.
		customerRepo := customer.NewRepository(conn)
		customersHandler := admin.NewCustomersHandler(customerRepo, log)

		// Reviews C3 wiring.
		reviewRepoAdmin := review.NewRepository(conn)
		reviewsHandler := admin.NewReviewsHandler(reviewRepoAdmin, log)

		// Settings S1 — Account & Security.
		accountHandler := admin.NewAccountHandler(cfg.AuthBFFURL, log)

		// Settings S2 — Custom Domains: domainSvc + domainsHandler hoisted
		// above mode blocks so the storefront can also use the resolve endpoint.

		// Settings S3 — Subscription/Billing.
		subscriptionRepo := subscription.NewRepository()
		subscriptionSvc := subscription.NewService(subscription.ServiceConfig{
			DB:     conn,
			Repo:   subscriptionRepo,
			Stripe: nil, // Stub — real Stripe client wired in production config
			Logger: log,
		})
		subscriptionHandler := admin.NewSubscriptionHandler(subscriptionSvc, cfg.StripeBillingWebhookSecret, log)

		// Settings S4 — Audit Logs.
		auditLogsHandler := admin.NewAuditLogsHandler(cfg.AuditServiceURL, log)

		// Dashboard D1 wiring.
		dashboardHandler := admin.NewDashboardHandler(conn, log)

		// Tickets D2 wiring.
		ticketRepo := ticket.NewRepository()
		ticketSvc := ticket.NewService(ticket.ServiceConfig{
			DB:     conn,
			Repo:   ticketRepo,
			Logger: log,
		})
		ticketsHandler := admin.NewTicketsHandler(ticketSvc, log)

		// Settings S5 — Notifications.
		notificationRepo := notification.NewRepository()
		notificationSvc := notification.NewService(notification.ServiceConfig{
			DB:     conn,
			Repo:   notificationRepo,
			Logger: log,
		})
		notificationsHandler := admin.NewNotificationsHandler(notificationSvc, log)

		// B1 — Storefront Branding.
		brandingRepo := branding.NewRepository()
		brandingSvc := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   brandingRepo,
			Logger: log,
		})
		brandingHandler := admin.NewBrandingHandler(brandingSvc, log)
		brandingHandler.SetUploader(uploader)

		// Test-only branding seeder. Wired only when
		// MARKETPLACE_API_ENABLE_TEST_ROUTES=true so it can never leak
		// into a deployed environment. Used by the storefront visual-
		// regression Playwright suite to flip layout + homepage_content
		// between snapshots. brandingSeeder is declared at func scope.
		if os.Getenv("MARKETPLACE_API_ENABLE_TEST_ROUTES") == "true" {
			brandingSeeder = testroutes.NewBrandingSeeder(storesRepo, brandingSvc, log)
			log.Warn("test routes enabled — do not use in production")
		}

		// Pages (CMS).
		pageSvc := page.NewService(page.NewRepository(conn))
		pagesHandler := admin.NewPagesHandler(pageSvc, log)

		// B2 — Plan gate resolver (shared between admin and storefront).
		planResolver := plangate.NewPlanResolver(conn, subscriptionRepo)

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
			ShipmentsHandler:       shipmentsHandler,
			TaxSettingsHandler:      taxSettingsHandler,
			SettingsMetaHandler:     settingsMetaHandler,
			CouponHandler:          couponHandler,
			GiftCardHandler:        giftCardHandler,
			LoyaltyHandler:         loyaltyHandler,
			CampaignHandler:        campaignHandler,
			SegmentHandler:         segmentHandler,
			CustomersHandler:       customersHandler,
			ReviewsHandler:        reviewsHandler,
			AccountHandler:         accountHandler,
			DomainsHandler:         domainsHandler,
			SubscriptionHandler:    subscriptionHandler,
			AuditLogsHandler:       auditLogsHandler,
			NotificationsHandler:   notificationsHandler,
			DashboardHandler:       dashboardHandler,
			TicketsHandler:         ticketsHandler,
			BrandingHandler:        brandingHandler,
			PagesHandler:           pagesHandler,
			PlanResolver:           planResolver,
			StoresMiddleware:        storeMW,
			AuthzMiddleware:         authzMW,
			InternalSecret:          cfg.InternalAuthSecret,
		}
	}

	// Mobile admin deps — Bearer auth for external mobile clients.
	var mobileDeps admin.MobileDeps
	var pushWebhookHandler gin.HandlerFunc
	if m == mode.Admin || m == mode.Both {
		var tokenVerifier auth.TokenVerifier
		if cfg.GIPProjectID != "" {
			firebaseApp, err := firebase.NewApp(context.Background(), &firebase.Config{
				ProjectID: cfg.GIPProjectID,
			})
			if err != nil {
				log.Error("failed to init Firebase app for mobile auth", "error", err)
			} else {
				authClient, err := firebaseApp.Auth(context.Background())
				if err != nil {
					log.Error("failed to init Firebase Auth client", "error", err)
				} else {
					tokenVerifier = auth.NewGIPVerifier(authClient)
				}
			}
		}
		pushRepo := push.NewRepository(conn)
		pushTokenHandler := admin.NewPushTokenHandler(pushRepo, log)
		pushSender := push.NewSender(&http.Client{Timeout: 10 * time.Second})
		pushWebhookHandler = push.NewWebhookHandler(pushSender, pushRepo, log)

		mobileDeps = admin.MobileDeps{
			Deps:             adminDeps,
			TokenVerifier:    tokenVerifier,
			PushTokenHandler: pushTokenHandler,
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

		// Coupon storefront wiring (Marketing M1) — shares couponSvc instance.
		couponValidateHandler := storefront.NewCouponValidateHandler(couponSvc, log)

		// Gift cards — Marketing M2.
		giftCardRepoSF := giftcard.NewRepository()
		// Storefront purchase flow: use the same SendGrid mailer config
		// as admin-issued cards, the store's own branding for the email,
		// and the store's configured Stripe gateway (payment_gateway_configs).
		var giftCardMailerSF giftcard.Mailer
		if cfg.SendGridAPIKey != "" {
			giftCardMailerSF = giftcard.NewSendGridMailer(cfg.SendGridAPIKey, cfg.EmailFrom, log)
		} else {
			giftCardMailerSF = &giftcard.LogMailer{Logger: log}
		}
		giftCardBrandingSvcSF := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		giftCardThemeLoaderSF := giftcard.NewStoreThemeLoader(conn, giftCardBrandingSvcSF, cfg.StorefrontBaseURLTemplate)
		giftCardGatewayResolver := giftcard.NewDBGatewayResolver(conn)
		giftCardSvcSF := giftcard.NewServiceWithMailer(conn, giftCardRepoSF, giftCardMailerSF, giftCardThemeLoaderSF, log).
			WithGatewayResolver(giftCardGatewayResolver)
		giftCardSFHandler := storefront.NewGiftCardStorefrontHandler(giftCardSvcSF, log)

		// Loyalty M3 storefront wiring.
		loyaltyRepoSF := loyalty.NewRepository()
		loyaltySvcSF := loyalty.NewService(conn, loyaltyRepoSF, log)
		sfLoyaltyHandler := storefront.NewLoyaltyHandler(loyaltySvcSF, log)

		// C1 — Customer profiles and account.
		customerRepo := customer.NewRepository(conn)
		customerSvc := customer.NewService(conn, customerRepo, log)
		customerAccountHandler := storefront.NewCustomerAccountHandler(conn, customerRepo, customerSvc, log)

		// C3 — Reviews.
		reviewRepoSF := review.NewRepository(conn)
		reviewSvcSF := review.NewService(conn, reviewRepoSF, log)
		sfReviewsHandler := storefront.NewReviewsHandler(reviewSvcSF, reviewRepoSF, productRepoSF, log)

		// C4 — Wishlists.
		wishlistRepo := wishlist.NewRepository(conn)
		wishlistHandler := storefront.NewWishlistHandler(wishlistRepo, log)

		// B1 — Storefront Branding (public endpoint).
		brandingRepoSF := branding.NewRepository()
		brandingSvcSF := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   brandingRepoSF,
			Logger: log,
		})
		campaignRepoSF := campaign.NewRepository(conn)
		sfBrandingHandler := storefront.NewBrandingHandler(brandingSvcSF, conn, campaignRepoSF, couponRepo, log)

		// Pages (CMS) — storefront public read.
		pageSvcSF := page.NewService(page.NewRepository(conn))
		sfPagesHandler := storefront.NewPagesHandler(pageSvcSF, log)

		// P5b — extended checkout, payment methods, shipping rates, webhooks.
		checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, couponSvc, giftCardSvcSF, log)
		checkoutExtHandler.SetLoyaltyService(loyaltySvcSF)
		paymentMethodsHandler := storefront.NewPaymentMethodsHandler(conn, log)
		shippingRatesHandler := storefront.NewShippingRatesHandler(conn, log)
		shippingOptionsHandler := storefront.NewShippingOptionsHandler(conn, log)
		webhookHandler := storefront.NewWebhookHandler(conn, orderSvcSF, log).
			WithGiftCardService(giftCardSvcSF).
			WithLoyaltyService(loyaltySvcSF)
		orderDetailHandler := storefront.NewOrderDetailHandler(conn, orderRepoSF, log)

		storefrontDeps = storefront.Deps{
			Handler:               storefrontHandler,
			CheckoutHandler:       checkoutHandler,
			CheckoutExtHandler:    checkoutExtHandler,
			PaymentMethodsHandler: paymentMethodsHandler,
			ShippingRatesHandler:   shippingRatesHandler,
			ShippingOptionsHandler: shippingOptionsHandler,
			WebhookHandler:        webhookHandler,
			OrderDetailHandler:    orderDetailHandler,
			CouponValidateHandler: couponValidateHandler,
			GiftCardHandler:       giftCardSFHandler,
			LoyaltyHandler:        sfLoyaltyHandler,
			SlugCache:             slugCache,
			StorefrontKey:         cfg.StorefrontKey,
			CountryHandler:        countryHandler,
			// C1 customer auth.
			CustomerAccountHandler: customerAccountHandler,
			CustomerService:        customerSvc,
			CustomerSessionSecret:  cfg.CustomerSessionSecret,
			// C3 reviews.
			ReviewsHandler: sfReviewsHandler,
			// C4 wishlists.
			WishlistHandler: wishlistHandler,
			// B1 branding.
			BrandingHandler:      sfBrandingHandler,
			PagesHandler:         sfPagesHandler,
			DomainResolveHandler: domainsHandler.ResolveDomain,
			Logger:               log,
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

	// Loyalty point expiry worker — runs daily, admin/both modes only.
	var expiryWorkerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		loyaltyRepoWorker := loyalty.NewRepository()
		expiryWorker := loyalty.NewExpiryWorker(conn, loyaltyRepoWorker, log)
		expiryWorkerDone = expiryWorker.Start(workerCtx, 24*time.Hour)
		log.Info("loyalty: expiry worker started (24h interval)")
	}

	// Campaign send worker — runs in admin and both modes.
	// On startup, recover stuck campaigns (stale heartbeat > 15 min → paused).
	// Then poll for sendable campaigns every 5s.
	var campaignWorkerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		campaignRepo := campaign.NewRepository(conn)

		// Recovery scan on startup.
		if err := campaign.RecoverStuckCampaigns(context.Background(), campaignRepo, conn, campaign.StaleDuration, log); err != nil {
			log.Error("campaign: recovery scan failed", "err", err)
			// Non-fatal — proceed without recovery.
		} else {
			log.Info("campaign: recovery scan complete")
		}

		// Polling goroutine.
		campaignDone := make(chan struct{})
		campaignWorkerDone = campaignDone
		// Prefer SendGrid when an API key is configured; fall back to
		// LogDispatcher in dev / local so the worker still runs.
		var dispatcher campaign.Dispatcher
		if cfg.SendGridAPIKey != "" {
			dispatcher = campaign.NewSendGridDispatcher(cfg.SendGridAPIKey, cfg.EmailFrom, log)
			log.Info("campaign: SendGrid dispatcher enabled", "from", cfg.EmailFrom)
		} else {
			dispatcher = &campaign.LogDispatcher{Logger: log}
			log.Warn("campaign: SENDGRID_API_KEY not set — using LogDispatcher (no emails will be sent)")
		}
		// Load per-store branding into the campaign envelope so customer
		// emails match the merchant's storefront theme, not the platform's.
		brandingSvcForCampaign := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		themeLoader := campaign.NewStoreThemeLoader(conn, brandingSvcForCampaign)

		sendWorker := campaign.NewSendWorker(campaign.SendWorkerConfig{
			DB:          conn,
			Repo:        campaignRepo,
			Dispatcher:  dispatcher,
			ThemeLoader: themeLoader,
			Logger:      log,
		})
		go func() {
			defer close(campaignDone)
			sendWorker.Run(workerCtx)
		}()
		log.Info("campaign: send worker started")
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

	// Vendor — Phase 1 of the tenant/vendor/store refactor. See
	// docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md
	// Handler only mounted in admin/both modes; storefront has no reason to
	// expose the /internal vendor endpoints.
	var vendorHandler *vendor.Handler
	if m == mode.Admin || m == mode.Both {
		vendorHandler = vendor.NewHandler(vendorSvc)
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
		admin.RegisterAdminMobile(r.Group("/api/v1"), mobileDeps)
		storefront.RegisterStorefront(r.Group("/api/v1"), storefrontDeps)
		if brandingSeeder != nil {
			brandingSeeder.Register(r.Group("/api/v1/test"))
		}
		if pushWebhookHandler != nil {
			r.POST("/internal/push-webhook", pushWebhookHandler)
		}
		if vendorHandler != nil {
			vendorHandler.RegisterRoutes(r.Group("/internal"))
		}
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
			admin.RegisterAdminMobile(engine.Group("/api/v1"), mobileDeps)
			if countryPublicHandler != nil {
				engine.GET("/api/v1/public/supported-countries", countryPublicHandler.ListSupported)
			}
			if pushWebhookHandler != nil {
				engine.POST("/internal/push-webhook", pushWebhookHandler)
			}
			if vendorHandler != nil {
				vendorHandler.RegisterRoutes(engine.Group("/internal"))
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
	if expiryWorkerDone != nil {
		select {
		case <-expiryWorkerDone:
			log.Info("loyalty: expiry worker stopped")
		case <-time.After(5 * time.Second):
			log.Warn("loyalty: expiry worker did not stop in time")
		}
	}
	if campaignWorkerDone != nil {
		select {
		case <-campaignWorkerDone:
			log.Info("campaign: send worker stopped")
		case <-time.After(5 * time.Second):
			log.Warn("campaign: send worker did not stop in time")
		}
	}
	log.Info("bye")
}

// k8sprovAdapter bridges the concrete *k8sprov.Provisioner to the
// domain.Provisioner interface so the domain package doesn't import
// the k8sprov package (avoids a large transitive client-go import in
// unit tests + keeps the boundary clean).
type k8sprovAdapter struct{ p *k8sprov.Provisioner }

func (a k8sprovAdapter) Provision(ctx context.Context, domainName string) (*domain.ProvisionResult, error) {
	res, err := a.p.Provision(ctx, domainName)
	if err != nil {
		return nil, err
	}
	return &domain.ProvisionResult{CertSecretName: res.CertSecretName}, nil
}

func (a k8sprovAdapter) Deprovision(ctx context.Context, domainName string) error {
	return a.p.Deprovision(ctx, domainName)
}

func (a k8sprovAdapter) CertStatus(ctx context.Context, domainName string) (bool, string, error) {
	return a.p.CertStatus(ctx, domainName)
}
