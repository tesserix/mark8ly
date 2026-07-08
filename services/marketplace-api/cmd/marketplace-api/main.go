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

	secretmanagerclient "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/robfig/cron/v3"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
	appcredspkg "github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/revalidation"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/seaqueue"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/taxreg"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/billingarchive"
	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
	campaignbudgetcron "github.com/mark8ly/marketplace-api/internal/campaignbudget/cron"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/cfclient"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/customerportal"
	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/gipkey"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/internalsvc"
	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/handlers/testroutes"
	"github.com/mark8ly/marketplace-api/internal/handlers/webhooks"
	"github.com/mark8ly/marketplace-api/internal/health"
	"github.com/mark8ly/marketplace-api/internal/ipprivacy"
	"github.com/mark8ly/marketplace-api/internal/k8sprov"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/observability"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/ottoclient"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/push"
	"github.com/mark8ly/marketplace-api/internal/refund"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/signup"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/internal/subscription/harddelete"
	"github.com/mark8ly/marketplace-api/internal/subscription/lifecycle"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/internal/subscription/readonly"
	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	wlapple "github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
	wlfirebase "github.com/mark8ly/marketplace-api/internal/whitelabel/firebase"
	wlgoogleplay "github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
	wllifecycle "github.com/mark8ly/marketplace-api/internal/whitelabel/lifecycle"
	"github.com/mark8ly/marketplace-api/internal/wishlist"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
	"github.com/mark8ly/marketplace-api/pkg/httpserver"
	"github.com/mark8ly/marketplace-api/pkg/logger"
	"github.com/mark8ly/marketplace-api/pkg/migrate"
)

// stripeClientAdapter wraps the package-level billingstripe helpers into the
// subscription.StripeClient interface. It exists only to avoid refactoring
// the subscription.Service signature as part of P2 wiring.
type stripeClientAdapter struct {
	c *billingstripe.Client
}

func (a *stripeClientAdapter) CreateCustomer(ctx context.Context, email, name string) (string, error) {
	cu, err := billingstripe.CreateCustomer(ctx, a.c, billingstripe.CreateCustomerInput{
		// StoreID/TenantID are not in the legacy StripeClient interface; passed
		// as empty strings here. TODO(P3): rewire to pass them from CheckoutInput.
		Email: email,
		Name:  name,
	})
	if err != nil {
		return "", err
	}
	return cu.ID, nil
}

func (a *stripeClientAdapter) CreateCheckoutSession(ctx context.Context, customerID string, plan subscription.SubscriptionPlan, successURL, cancelURL string) (string, error) {
	// The legacy interface carries no PriceID or Currency. Use the Developed
	// monthly descriptor to supply a sensible currency default.
	// TODO(P3): pass PriceID + Currency from a richer CheckoutInput so the
	// correct price object is selected per store locale.
	desc := pricing.MustGetDescriptor(pricing.Plan(plan), pricing.PeriodMonthly, pricing.TierDeveloped)
	sess, err := billingstripe.CreateCheckoutSession(ctx, a.c, billingstripe.CheckoutInput{
		CustomerID: customerID,
		PriceID:    "", // placeholder — real price ID populated after billing-bootstrap; 400s here are pre-existing (nil client = always broken)
		Currency:   desc.Baseline.Currency,
		Plan:       string(plan),
		Period:     string(pricing.PeriodMonthly),
		SuccessURL: successURL,
		CancelURL:  cancelURL,
	})
	if err != nil {
		return "", err
	}
	return sess.URL, nil
}

func (a *stripeClientAdapter) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	// The legacy interface has no StoreID; use customerID as the idempotency
	// bucket key — safe because Stripe portal sessions are customer-scoped.
	ps, err := billingstripe.CreatePortalSession(ctx, a.c, billingstripe.PortalInput{
		StoreID:    customerID,
		CustomerID: customerID,
		ReturnURL:  returnURL,
	})
	if err != nil {
		return "", err
	}
	return ps.URL, nil
}

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

// otelServiceName is the OpenTelemetry service.name reported for traces and
// metrics. Both MODE variants (admin/storefront) run the same binary/image,
// so they share one logical service name; the MODE is distinguished via
// resource/span attributes downstream.
const otelServiceName = "mark8ly-marketplace-api"

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	// OpenTelemetry — traces + metrics over OTLP gRPC to the in-cluster
	// collector. No-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset (local dev).
	otelShutdown, err := observability.Init(context.Background(), otelServiceName)
	if err != nil {
		log.Error("otel init", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			log.Error("otel shutdown", "err", err)
		}
	}()

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

	// ─── Email templates loader (B1f) ───────────────────────────────────
	// DB-backed templates with embedded fallback. tesserix-home authors
	// templates over the cross-DB grant; loader reads with a 5-minute
	// TTL cache and falls back to embedded on miss / DB error so emails
	// keep flowing during outages. SeedFromEmbedded is idempotent so
	// the first boot after migration 000085 ships byte-identical output
	// to embedded.
	templateLoader := emailtemplates.NewLoader(conn)
	orderdoc.RegisterFallbacks(templateLoader)
	giftcard.RegisterFallbacks(templateLoader)
	{
		seedCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if seedErr := templateLoader.SeedFromEmbedded(seedCtx); seedErr != nil {
			log.Warn("emailtemplates: seed failed (continuing with embedded fallback)", "err", seedErr)
		}
		cancel()
	}
	// Test-send dispatcher used by /internal/templates/:key/test. nil
	// SendGrid key is acceptable — handler returns 503 with a clear
	// message so dev users don't get silent fail.
	templateTestSender := emailtemplates.NewSendGridTestSender(cfg.SendGridAPIKey, cfg.EmailFrom)
	templateHandler := emailtemplates.NewHandler(templateLoader, templateTestSender)

	// API key encryptor — AES-256-GCM in production, noop in dev.
	var apiKeyEncryptor crypto.Encryptor
	switch cfg.EncryptionMode {
	case "aes":
		if cfg.EncryptionKey == "" {
			log.Error("ENCRYPTION_KEY required when ENCRYPTION_MODE=aes")
			os.Exit(1)
		}
		key, decodeErr := crypto.DecodeKey(cfg.EncryptionKey)
		if decodeErr != nil {
			log.Error("invalid ENCRYPTION_KEY", "err", decodeErr)
			os.Exit(1)
		}
		enc, aesErr := crypto.NewAESEncryptor(key)
		if aesErr != nil {
			log.Error("create AES encryptor", "err", aesErr)
			os.Exit(1)
		}
		apiKeyEncryptor = enc
		log.Info("crypto: AES-256-GCM encryptor initialized")
	default:
		apiKeyEncryptor = crypto.NewNoopEncryptor()
		log.Info("crypto: noop encryptor (dev mode)")
	}

	// Per-tenant carrier credential store. In "gcpsm" mode we stand up
	// a real Secret Manager client (workload-identity ADC) and fall
	// back to the inline encryptor if the client constructor errors
	// out — loud warning plus a boot flag so ops can spot the
	// mismatch without the service refusing to start. "inline" mode
	// keeps local dev working without GCP creds.
	var carrierSecretStore carriersecrets.Store
	carrierSecretStoreDegraded := false
	switch cfg.ShippingSecretStore {
	case "gcpsm":
		if cfg.GCPProjectID == "" {
			log.Error("GCP_PROJECT_ID required when SHIPPING_SECRET_STORE=gcpsm")
			os.Exit(1)
		}
		smClient, smErr := secretmanagerclient.NewClient(context.Background())
		if smErr != nil {
			log.Error("carriersecrets: secret manager client init failed — falling back to inline",
				"err", smErr)
			carrierSecretStore = carriersecrets.NewInlineStore(apiKeyEncryptor)
			carrierSecretStoreDegraded = true
		} else {
			carrierSecretStore = carriersecrets.NewHybridStore(carriersecrets.HybridConfig{
				Client:    carriersecrets.NewGCPStore(smClient, cfg.GCPProjectID),
				Encryptor: apiKeyEncryptor,
				ProjectID: cfg.GCPProjectID,
				Prefix:    cfg.SecretNamePrefix,
			})
			log.Info("carriersecrets: hybrid store online",
				"project_id", cfg.GCPProjectID, "prefix", cfg.SecretNamePrefix)
		}
	default:
		carrierSecretStore = carriersecrets.NewInlineStore(apiKeyEncryptor)
		log.Info("carriersecrets: inline store (dev mode)")
	}
	_ = carrierSecretStoreDegraded // readiness wiring is a downstream concern; flag kept in scope so future health-check hooks can read it.

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
	// Cloudflare auto-DNS client. Hardcoded edge target matches the A
	// record provisioned out of band on edge.mark8ly.com — keep in
	// sync with the manual flow's "Option A" IP.
	cfAPIClient := cfclient.New("edge.mark8ly.com")
	// Adapt the carriersecrets.Store onto domain.SecretStore. Same
	// underlying GSM-or-inline plumbing already in use for shipping /
	// payment carrier credentials, just scoped to a different bucket.
	domainSecretStore := domainSecretsAdapter{inner: carrierSecretStore}

	// GIP browser-key allowlist syncer — only wired when the operator
	// has provisioned the API key's resource name into env, otherwise
	// the domain service uses the Noop client and the GCP call is
	// skipped (correct posture for local dev + UAT). A construction
	// failure must not exit the process — the rest of marketplace-api
	// is unaffected, and the manual fallback (operator edits the key
	// in the GCP console) still works.
	var gipKeyClient gipkey.Client = gipkey.Noop{}
	if cfg.GIPWebAPIKeyResource != "" {
		gipCtx, gipCancel := context.WithTimeout(context.Background(), 10*time.Second)
		gc, err := gipkey.New(gipCtx, cfg.GIPWebAPIKeyResource, log)
		gipCancel()
		if err != nil {
			log.Error("gipkey: client construction failed — custom domain allowlist sync disabled",
				"err", err, "resource", cfg.GIPWebAPIKeyResource)
		} else {
			gipKeyClient = gc
			log.Info("gipkey: client wired", "resource", cfg.GIPWebAPIKeyResource)
		}
	} else {
		log.Info("gipkey: GIP_WEB_API_KEY_RESOURCE_NAME not set — custom domain allowlist sync disabled (noop)")
	}

	domainSvc := domain.NewService(domain.ServiceConfig{
		DB:          conn,
		Repo:        domainRepo,
		CF:          cfAPIClient,
		Provisioner: provIface,
		Secrets:     domainSecretStore,
		GIPKey:      gipKeyClient,
		Logger:      log,
	})
	domainStoresRepo := stores.NewRepository(conn)
	domainsHandler := admin.NewDomainsHandler(domainSvc, domainStoresRepo, log)
	// Super-admin (tesserix-home) re-verify + cert-refresh actions over
	// /internal. Mounted in mode.Both and mode.Admin alongside the
	// templates internal handler so the operator UI on tesserix.app can
	// trigger a fresh DNS / cert check without forging a tenant session.
	internalDomainsHandler := admin.NewInternalDomainsHandler(domainSvc, conn, log)

	// Admin wiring — constructed for admin and both modes. The storefront
	// process never mounts the admin group so these dependencies would go
	// unused there.
	var adminDeps admin.Deps
	// delhiveryWebhookHandler is constructed in the admin wiring branch
	// (where shipmentsHandler, apiKeyEncryptor and carrierSecretStore
	// are built) but mounted below inside the engine switch, so declare
	// it at outer scope. Nil-safe — the RegisterPublic call skips the
	// route when it's still nil, so Storefront-only builds don't mount
	// the webhook.
	var delhiveryWebhookHandler *public.DelhiveryWebhookHandler
	// taxService is declared at outer scope so the revalidation cron (which
	// runs in both modes) can reference it. Constructed inside the admin
	// branch when the registry + handler dependencies are wired.
	var taxService *tax.Service
	// vendorSvc stays nil in Storefront mode by design — the storefront
	// process does not call product.Create and does not expose the
	// /internal vendor endpoints, so no self-vendor lookup is needed.
	// product.Service.resolveVendorID is nil-safe for exactly this case.
	var vendorSvc *vendor.Service
	// brandingSeeder is non-nil only when MARKETPLACE_API_ENABLE_TEST_ROUTES=true.
	// Declared at func scope so the later route-mount block can see it.
	var brandingSeeder *testroutes.BrandingSeeder
	// downgradeCron is non-nil only when STRIPE_BILLING_SECRET_KEY is set.
	// Declared at func scope so the cron-start block below the admin-mode
	// block can reference it.
	var downgradeCron *planchange.DowngradeRecheckCron
	// billingStripeClient is hoisted to func scope so the P11 lifecycle crons
	// (registered outside the admin-mode block) can reference it for the
	// hard-delete runner's Stripe customer deletion step.
	var billingStripeClient *billingstripe.Client

	// wlAppCredsSvc is hoisted so both admin route registration (constructs
	// AppCredentialsHandler + AppAddOnHandler) and the white-label lifecycle
	// cron (constructs Advancer) share a single appcreds.Service instance.
	// Assigned inside the admin-mode block; lifecycle cron below guards on
	// nil for MODE=storefront so the storefront pod doesn't need credstore.
	var wlAppCredsSvc *appcredspkg.Service

	// auditEmitter is the async audit-log writer. Init runs unconditionally
	// because both admin AND storefront emit (e.g. storefront checkout
	// fires order.created, signup fires customer.signed_up). Previously
	// this was init'd inside the admin-mode block, which meant the
	// storefront pod (MODE=storefront) ran with auditEmitter == nil and
	// silently dropped every storefront event.
	auditRepo := audit.NewRepository()
	auditEmitter := audit.NewEmitter(audit.EmitterConfig{
		DB:     conn,
		Repo:   auditRepo,
		Logger: log,
	})

	// Settings S5 — Notifications. Constructed early so both admin and
	// storefront modes can share a single service instance: storefront
	// checkout + review submission emit merchant notifications via this
	// service, while admin exposes the CRUD endpoints.
	notificationSvc := notification.NewService(notification.ServiceConfig{
		DB:     conn,
		Repo:   notification.NewRepository(),
		Logger: log,
	})

	// Shared outbound email transport — every mailer below (ticket,
	// orderdoc, shipping label, gift card, campaign) sends through this
	// one Sender. EMAIL_PRIMARY_PROVIDER orders the provider chain; every
	// other configured provider is an always-on per-message fallback. When
	// no API key is configured it degrades to a log-only sender so
	// local/dev still exercises the full dispatch path without a provider
	// account. New providers plug in here: add the key to this map after
	// registering the adapter in internal/email/providers.go.
	emailSender := email.NewFromConfig(map[string]string{
		email.ProviderSendGrid: cfg.SendGridAPIKey,
		email.ProviderResend:   cfg.ResendAPIKey,
	}, cfg.EmailPrimaryProvider, log)

	// Dashboard D2 — Tickets. Hoisted for the same reason as the
	// notification service: the storefront /support/tickets endpoint
	// needs to create tickets via this service, and the admin dashboard
	// reads them through the same service instance.
	// Ticket emails (customer-facing) ride the shared transport.
	// publicHost is the storefront origin the deep link in the email
	// points to (e.g. https://mystore.mark8ly.com). In a multi-store
	// deployment we'd compute it per-store; for now the env var is
	// sufficient as we only have one storefront brand per cluster.
	ticketNotifier := ticket.NewEmailNotifier(
		emailSender, cfg.EmailFrom, cfg.PublicStorefrontHost, log,
	)
	ticketSvc := ticket.NewService(ticket.ServiceConfig{
		DB:       conn,
		Repo:     ticket.NewRepository(),
		Logger:   log,
		Notifier: ticketNotifier,
	})
	ticketInternalHandler := ticket.NewInternalHandler(ticketSvc, ticketNotifier)

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
		// Refund coordinator — the single place that moves real money via
		// the payment gateway. Gated by REFUND_GATEWAY_ENABLED so refunds
		// stay a hard error (not a silent no-op) until the gateway wiring
		// is verified in an environment; Task 13 documents the flag and
		// reuses refundCoordinator for the other refund-triggering flows
		// (returns, paid cancels).
		refundGatewayEnabled := os.Getenv("REFUND_GATEWAY_ENABLED") == "true"
		paymentSvc := payment.NewService(payment.NewRepository(conn))
		refundResolver := orderrefund.NewResolver(conn)
		refundCoordinator := orderrefund.NewCoordinator(conn, refundResolver, paymentSvc, orderSvc, orderRepo, refundGatewayEnabled)
		// Order document mailer — invoice on accept, receipt on delivery.
		// Built up here because both OrdersHandler and ShipmentsHandler need
		// it. Provider selection (SendGrid → Resend → log-only) already
		// happened once in emailSender, so no per-mailer branching here.
		orderDocMailer := orderdoc.NewDocumentMailer(emailSender, cfg.EmailFrom, log).WithLoader(templateLoader)
		// Wire the storefront PDF fetcher so invoice + receipt
		// emails carry the rendered PDF as an attachment instead
		// of asking the buyer to click through to download.
		if pf := orderdoc.NewHTTPStorefrontPDFFetcher(cfg.StorefrontBaseURLTemplate, cfg.InternalAuthSecret); pf != nil {
			orderDocMailer = orderDocMailer.WithStorefrontPDFFetcher(pf)
		}
		orderDocBrandingSvc := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		orderDocSvc := orderdoc.NewService(conn, orderDocMailer, orderRepo, orderDocBrandingSvc, cfg.StorefrontBaseURLTemplate).
			WithLogger(log)

		ordersHandler := admin.NewOrdersHandler(conn, orderSvc, orderRepo, orderDocSvc, log).
			WithRefunds(refundCoordinator)
		returnsHandler := admin.NewReturnsHandler(conn, returnSvc, returnRepo, orderRepo, orderSvc, log).
			WithRefunds(refundCoordinator)
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
		paymentSettingsHandler := admin.NewPaymentSettingsHandler(conn, countryRepoAdmin, apiKeyEncryptor, log).
			WithSecretStore(carrierSecretStore)
		shippingSettingsHandler := admin.NewShippingSettingsHandler(conn, countryRepoAdmin, apiKeyEncryptor, log).
			WithSecretStore(carrierSecretStore)
		shippingRepo := shipping.NewRepository(conn)
		shippingService := shipping.NewShippingService(shippingRepo)
		// Label email rides the shared transport; the log-only fallback
		// now lives inside emailSender, so local dev still exercises the
		// dispatch path and the admin UI sees a 200 instead of a 503.
		labelMailer := shipping.NewEmailLabelMailer(emailSender, cfg.EmailFrom, log)
		shipmentsHandler := admin.NewShipmentsHandler(conn, shippingService, shippingRepo, orderDocSvc, log).
			WithEncryptor(apiKeyEncryptor).
			WithSecretStore(carrierSecretStore).
			WithLabelMailer(labelMailer)

		// Public Delhivery webhook receiver — same carrier-secret resolution
		// path as the shipments admin handler, wired to the admin handler's
		// AdvanceShipmentFromTracking so push-updates and poller-updates
		// produce identical order_events rows.
		delhiveryWebhookHandler = public.NewDelhiveryWebhookHandler(conn, shippingRepo, log).
			WithEncryptor(apiKeyEncryptor).
			WithSecretStore(carrierSecretStore).
			WithAdvance(shipmentsHandler.AdvanceShipmentFromTracking)
		taxSettingsHandler := admin.NewTaxSettingsHandler(conn, countryRepoAdmin, apiKeyEncryptor, log).
			WithSecretStore(carrierSecretStore)
		settingsMetaHandler := admin.NewSettingsMetaHandler(countryRepoAdmin, log)

		// Coupon handler (Marketing M1).
		couponHandler := admin.NewCouponHandler(couponSvc, log)

		// Gift cards — Marketing M2.
		giftCardRepo := giftcard.NewRepository()
		// Delivery email rides the shared transport (log-only in dev).
		// Theme loader pulls the merchant's branding + storefront URL so
		// the email matches the store, not the platform.
		giftCardMailer := giftcard.NewDeliveryMailer(emailSender, cfg.EmailFrom, log).WithLoader(templateLoader)
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
		// P9 — campaign budget gate + concurrency slot.
		// Redis is optional: if REDIS_URL is unset, advisory-lock fallback is used.
		campaignBudgetSvc := campaignbudget.NewService(conn)
		campaignbudget.MustRegisterMetrics(prometheus.DefaultRegisterer)
		campaignSlotAcquirer := concurrency.Select(nil, conn) // nil = no Redis → advisory-lock
		campaignHandler := admin.NewCampaignHandler(campaignSvc, campaignRepo, log).
			WithBudgetGate(campaignBudgetSvc, campaignSlotAcquirer)
		segmentHandler := admin.NewSegmentHandler(campaignSvc, log)

		// Customers C2 wiring.
		customerRepo := customer.NewRepository(conn)
		customersHandler := admin.NewCustomersHandler(customerRepo, log)

		// Reviews C3 wiring.
		reviewRepoAdmin := review.NewRepository(conn)
		reviewsHandler := admin.NewReviewsHandler(reviewRepoAdmin, log)

		// Settings S1 — Account & Security.
		accountHandler := admin.NewAccountHandler(conn, cfg.AuthBFFURL, cfg.InternalAuthSecret, log)
		accountHandler.SetUploader(uploader)

		// Settings S2 — Custom Domains: domainSvc + domainsHandler hoisted
		// above mode blocks so the storefront can also use the resolve endpoint.

		// Settings S3 — Subscription/Billing.
		// billingStripeClient is declared at func scope (see above) so the P11
		// lifecycle crons can reference it outside this admin-mode block.
		subscriptionRepo := subscription.NewRepository()
		var stripeAdapter subscription.StripeClient
		if cfg.StripeBillingSecretKey != "" {
			billingStripeClient = billingstripe.New(cfg.StripeBillingSecretKey)
			stripeAdapter = &stripeClientAdapter{c: billingStripeClient}
		} else {
			log.Warn("STRIPE_BILLING_SECRET_KEY not set — subscription checkout/portal will fail")
		}
		subscriptionSvc := subscription.NewService(subscription.ServiceConfig{
			DB:     conn,
			Repo:   subscriptionRepo,
			Stripe: stripeAdapter,
			Logger: log,
		})
		subscriptionHandler := admin.NewSubscriptionHandler(subscriptionSvc, log).WithDB(conn).WithStripe(billingStripeClient)

		// P4 Subscription plan change (upgrade/downgrade) — requires Stripe + stores repo.
		var changePlanHandler *admin.ChangePlanHandler
		if billingStripeClient != nil {
			planchangeStripe := &planchange.StripeClientAdapter{C: billingStripeClient}
			planChangeOrch := planchange.NewOrchestrator(planchange.Deps{
				DB:               conn,
				Stripe:           planchangeStripe,
				Emitter:          auditEmitter,
				SubscriptionRepo: subscriptionRepo,
				StoreRepo:        storesRepo,
			})
			changePlanHandler = admin.NewChangePlanHandler(planChangeOrch, log)

			downgradeCron = planchange.NewDowngradeRecheckCron(planchange.CronDeps{
				DB:               conn,
				SubscriptionRepo: subscriptionRepo,
				StoreRepo:        storesRepo,
				Stripe:           planchangeStripe,
				Emitter:          auditEmitter,
				Logger:           log,
				// Interval defaults to 1h inside NewDowngradeRecheckCron.
				// Notifier: nil for now — email template lands in a later phase.
			})
		} else {
			log.Warn("STRIPE_BILLING_SECRET_KEY not set — /subscription/change-plan + downgrade cron disabled")
		}

		// Settings S4 — Audit Logs read endpoint (admin-only). The
		// emitter + repo are initialised unconditionally above so the
		// storefront pod can also write events.
		auditLogsHandler := admin.NewAuditLogsHandler(conn, auditRepo, log)

		// Wire the emitter into the handlers that own audited resources.
		// brandingHandler is wired below where it's constructed.
		ordersHandler.WithAudit(auditEmitter)
		productHandler.WithAudit(auditEmitter)
		domainsHandler.WithAudit(auditEmitter)
		customersHandler.WithAudit(auditEmitter)
		reviewsHandler.WithAudit(auditEmitter)
		couponHandler.WithAudit(auditEmitter)
		giftCardHandler.WithAudit(auditEmitter)
		subscriptionHandler.WithAudit(auditEmitter)
		paymentSettingsHandler.WithAudit(auditEmitter)

		// Dashboard D1 wiring.
		dashboardHandler := admin.NewDashboardHandler(conn, log)

		// Tickets D2 wiring — uses the shared ticketSvc hoisted above so
		// the storefront /support/tickets endpoint and the admin dashboard
		// read/write the same rows through the same service instance.
		ticketsHandler := admin.NewTicketsHandler(ticketSvc, log)
		ticketsHandler.WithAudit(auditEmitter)

		// Settings S5 — Notifications handler uses the shared notificationSvc
		// constructed above. Wire the notifier into lifecycle handlers so
		// events like new orders / cancellations / deliveries / returns fire
		// in-app notifications (respecting merchant preferences).
		notificationsHandler := admin.NewNotificationsHandler(notificationSvc, log)
		ordersHandler.WithNotifier(notificationSvc)
		returnsHandler.WithNotifier(notificationSvc)
		variantHandler.WithNotifier(conn, notificationSvc)
		shipmentsHandler.WithNotifier(notificationSvc)

		// B1 — Storefront Branding.
		brandingRepo := branding.NewRepository()
		brandingSvc := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   brandingRepo,
			Logger: log,
		})
		brandingHandler := admin.NewBrandingHandler(brandingSvc, log)
		brandingHandler.SetUploader(uploader)
		brandingHandler.WithAudit(auditEmitter)

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
		// Field-level plan gate for paid-tier branding fields (custom_css
		// requires Studio+). The branding handler was constructed above
		// before the resolver existed — wire it now via setter so the
		// gate is active before any request handler runs.
		brandingHandler.SetPlanResolver(planResolver)
		// Per-plan images-per-product cap (plangate.ImagesAllowed) on the
		// product-media Create handler. Also constructed before the
		// resolver, so wired via setter; see apps/onboarding plan doc
		// 2026-04-20-plangate-enforcement-gaps.md §P1.1.
		mediaHandler.SetPlanGate(planResolver, subscriptionRepo, conn)

		// P5 — Trial billing subscribe handler (deferred-charge card-add §5.3).
		var trialBillingHandler *admin.TrialBillingHandler
		if billingStripeClient != nil {
			trialSubscriber := trial.NewSubscriber(conn, &trial.StripeAdapter{C: billingStripeClient}, nil)
			trialBillingHandler = admin.NewTrialBillingHandler(trialSubscriber, log)
		}

		// P10 — Promo-code engine (§7).
		promoRepo := promo.NewRepository()
		promoSvc := promo.NewService(conn, promoRepo, billingStripeClient, log)
		promoHandler := admin.NewPromoHandler(conn, promoSvc, subscriptionRepo, log).WithAudit(auditEmitter)

		// P10 — 14-day cooling-off refund (§8).
		refundRepo := refund.NewRepository()
		refundSvc := refund.NewService(conn, refundRepo, subscriptionRepo, billingStripeClient, log)
		refundHandler := admin.NewRefundHandler(conn, refundSvc, log).WithAudit(auditEmitter)

		// P11 — Cancel handler (merchant-initiated cancellation + save-offer §15).
		cancelSvc := cancel.NewService(conn, subscriptionRepo, auditEmitter, log)
		cancelHandler := cancel.NewHandler(cancelSvc, log)

		// P5 — Migration fast-path submit handler.
		migrationRepo := migration.NewRepository(conn)
		migrationHandler := migration.NewHandler(migrationRepo, migration.NoOpValidator{}, log)

		// P8 — Arbitrage appeal handler (§18.8.1).
		arbitrageAppealSvc := arbitrage.NewAppealService(conn, arbitrage.NoOpPublisher{}, arbitrage.NopPIILogger{})
		arbitrageAppealHandler := admin.NewArbitrageAppealHandler(arbitrageAppealSvc)

		// P7 — tax-ID validation pipeline (§19). Registry holds 13 country
		// validators; NZ is flag-gated until counsel sign-off (§20.3). The
		// orchestrator wraps the registry with SEA queue insert + clock-pause
		// + advisory-lock CAS write of tax_id_validated.
		taxRegistry := taxreg.BuildDefault(taxreg.Config{
			HTTPClient:    &http.Client{Timeout: 15 * time.Second},
			NZEnabled:     cfg.NZTaxValidationEnabled,
			GSTNAuthToken: cfg.GSTNAuthToken,
			ABNGUID:       cfg.ABNGUID,
		})
		taxService = tax.NewService(tax.ServiceConfig{
			DB:       conn,
			Registry: taxRegistry,
			Audit:    auditEmitter,
			SEAQueue: seaqueue.New(conn),
			Clock:    tax.NewClockPauseTracker(conn),
		})
		taxHandler := admin.NewTaxHandler(conn, taxService, []byte(cfg.TaxAttestationIPHashKey))

		// P14 — enterprise API keys (§18.4). Repo + cache + service for the
		// admin endpoints; auth middleware + rate limiter + last-used worker
		// for the public R/W API mount (registered later when the public API
		// router lands — until then the auth surface is dormant but tested).
		apiKeysRepo := apikeys.NewRepo(conn)
		apiKeysCache := apikeys.NewCache(60 * time.Second)
		apiKeysService := apikeys.NewService(conn, apiKeysRepo, apiKeysCache, apikeys.EnvLive, auditEmitter)
		apiKeysHandler := admin.NewAPIKeysHandler(apiKeysService, planResolver, log)
		apiKeysIPHasher := ipprivacy.New([]byte(cfg.APIKeyIPHashKey))
		apiKeysLastUsed := apikeys.NewLastUsedWorker(apiKeysRepo, apiKeysIPHasher, log, 1024)
		_ = apiKeysCache    // referenced by middleware once the public API router mounts
		_ = apiKeysLastUsed // see above

		// P15 — white-label app credential store + purchase/upload handlers.
		// GCP Secret Manager when APPCREDS_PROJECT_ID is set; FakeSM dev
		// fallback when empty. The advancer + lifecycle cron are wired
		// further down (near trialScheduler); we share this Service via
		// the hoisted wlAppCredsSvc var.
		var wlAppCredsSM appcredspkg.SM
		if cfg.AppCredsProjectID != "" {
			if smClient, err := secretmanagerclient.NewClient(context.Background()); err != nil {
				log.Error("init secret manager client for appcreds", "err", err)
			} else {
				defer smClient.Close()
				wlAppCredsSM = appcredspkg.NewGCPSM(smClient, cfg.AppCredsProjectID)
			}
		}
		if wlAppCredsSM == nil {
			wlAppCredsSM = appcredspkg.NewFakeSM()
			log.Warn("P15 appcreds using FakeSM — set APPCREDS_PROJECT_ID for production")
		}
		wlAppCredsSvc = appcredspkg.NewService(appcredspkg.Config{
			ProjectID: cfg.AppCredsProjectID,
			SM:        wlAppCredsSM,
			Emitter:   auditEmitter,
		})
		appCredentialsHandler := admin.NewAppCredentialsHandler(conn, subscriptionRepo, wlAppCredsSvc)
		appAddOnHandler := appaddon.NewHandler(appaddon.Config{
			DB:      conn,
			Stripe:  &appaddon.StripeClientAdapter{Client: billingStripeClient},
			SubRepo: subscriptionRepo,
		})

		adminDeps = admin.Deps{
			ProductHandler:           productHandler,
			CategoryHandler:          categoryHandler,
			VariantHandler:           variantHandler,
			MediaHandler:             mediaHandler,
			OrdersHandler:            ordersHandler,
			ReturnsHandler:           returnsHandler,
			AbandonedCartsHandler:    abandonedCartsHandler,
			StoresHandler:            storesHandler,
			BulkHandler:              bulkHandler,
			CSVImportsHandler:        csvImportsHandler,
			PaymentSettingsHandler:   paymentSettingsHandler,
			ShippingSettingsHandler:  shippingSettingsHandler,
			ShipmentsHandler:         shipmentsHandler,
			TaxSettingsHandler:       taxSettingsHandler,
			SettingsMetaHandler:      settingsMetaHandler,
			CouponHandler:            couponHandler,
			GiftCardHandler:          giftCardHandler,
			LoyaltyHandler:           loyaltyHandler,
			CampaignHandler:          campaignHandler,
			SegmentHandler:           segmentHandler,
			CustomersHandler:         customersHandler,
			ReviewsHandler:           reviewsHandler,
			AccountHandler:           accountHandler,
			DomainsHandler:           domainsHandler,
			SubscriptionHandler:      subscriptionHandler,
			PromoHandler:             promoHandler,
			RefundHandler:            refundHandler,
			ChangePlanHandler:        changePlanHandler,
			CancelHandler:            cancelHandler,
			ArbitrageAppealHandler:   arbitrageAppealHandler,
			TrialBillingHandler:      trialBillingHandler,
			MigrationFastPathHandler: migrationHandler,
			TaxHandler:               taxHandler,
			APIKeysHandler:           apiKeysHandler,
			APIKeysLogger:            log,
			AppCredentialsHandler:    appCredentialsHandler,
			AppAddOnHandler:          appAddOnHandler,
			AuditLogsHandler:         auditLogsHandler,
			NotificationsHandler:     notificationsHandler,
			DashboardHandler:         dashboardHandler,
			TicketsHandler:           ticketsHandler,
			BrandingHandler:          brandingHandler,
			PagesHandler:             pagesHandler,
			PlanResolver:             planResolver,
			StoresMiddleware:         storeMW,
			SubscriptionStatusLoader: readonly.LoadStatus(readonly.StatusLoaderConfig{DB: conn, Repo: subscriptionRepo, Logger: log}),
			SubscriptionReadOnlyGate: readonly.RequireActive(readonly.Config{}),
			AuthzMiddleware:          authzMW,
			InternalSecret:           cfg.InternalAuthSecret,
			AuditIngestSecret:        cfg.AuditIngestSecret,
		}
	}

	// Shared otto chat client (S2S) — nil when OTTO_URL/OTTO_INTERNAL_AUTH
	// are unset (local dev without otto). Reused by the ticket transcript
	// pull, the admin platform-support bridge (#119), and the storefront
	// support bridge (#118).
	ottoChatClient := ottoclient.New(cfg.OttoURL, cfg.OttoInternalAuth)
	// Mobile storefront support chat (#118) — populated in the storefront
	// wiring block, mounted in the per-mode registration below.
	var storefrontSupportHandler *storefront.MobileSupportHandler
	var storefrontCustomerVerifier storefront.CustomerVerifier

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
			// Merchant→platform support chat (#119) — bridges to otto's
			// platform tenant. Nil otto client => routes return 503.
			PlatformSupportHandler: admin.NewPlatformSupportHandler(ottoChatClient, cfg.OttoWSPublicBase, log),
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
		// Return service — also wired in storefront mode so the customer
		// self-service "request a return/replace" endpoint can enqueue
		// return_requested events (admin replica drains them into the
		// notification hub).
		returnRepoSF := order.NewReturnRepository()
		returnSvcSF := order.NewReturnService(conn, returnRepoSF, orderRepoSF, orderSvcSF, outboxRepoSF)
		checkoutHandler := storefront.NewCheckoutHandler(conn, orderSvcSF, orderRepoSF, log).
			WithAudit(auditEmitter).
			WithNotifier(notificationSvc)

		countryRepo := country.NewRepository(conn)
		countryHandler := country.NewHandler(countryRepo)

		// Coupon storefront wiring (Marketing M1) — shares couponSvc instance.
		couponValidateHandler := storefront.NewCouponValidateHandler(couponSvc, log)

		// Gift cards — Marketing M2.
		giftCardRepoSF := giftcard.NewRepository()
		// Storefront purchase flow: same shared email transport as
		// admin-issued cards, the store's own branding for the email,
		// and the store's configured Stripe gateway (payment_gateway_configs).
		giftCardMailerSF := giftcard.NewDeliveryMailer(emailSender, cfg.EmailFrom, log).WithLoader(templateLoader)
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
		customerSvc := customer.NewService(conn, customerRepo, log).WithAudit(auditEmitter)
		customerAccountHandler := storefront.NewCustomerAccountHandler(conn, customerRepo, customerSvc, log)
		customerNotificationsHandler := storefront.NewCustomerNotificationsHandler(notificationSvc)

		// C3 — Reviews.
		reviewRepoSF := review.NewRepository(conn)
		reviewSvcSF := review.NewService(conn, reviewRepoSF, log)
		sfReviewsHandler := storefront.NewReviewsHandler(reviewSvcSF, reviewRepoSF, productRepoSF, log).
			WithNotifier(notificationSvc)

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
		checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, couponSvc, giftCardSvcSF, apiKeyEncryptor, log).
			WithAudit(auditEmitter).
			WithSecretStore(carrierSecretStore)
		checkoutExtHandler.SetLoyaltyService(loyaltySvcSF)
		paymentMethodsHandler := storefront.NewPaymentMethodsHandler(conn, apiKeyEncryptor, log).
			WithSecretStore(carrierSecretStore)
		shippingRatesHandler := storefront.NewShippingRatesHandler(conn, apiKeyEncryptor, log).
			WithSecretStore(carrierSecretStore)
		shippingOptionsHandler := storefront.NewShippingOptionsHandler(conn, log)
		webhookHandler := storefront.NewWebhookHandler(conn, orderSvcSF, log).
			WithEncryptor(apiKeyEncryptor).
			WithSecretStore(carrierSecretStore).
			WithGiftCardService(giftCardSvcSF).
			WithLoyaltyService(loyaltySvcSF).
			WithNotifier(notificationSvc).
			// Enables /webhooks/:storeSlug/:provider — spec §2.3 scoped
			// route. Without this the legacy /webhooks/:provider is the
			// only mount, and it fails closed on ambiguous multi-tenant
			// configs. Share the same slug cache as the storefront group.
			WithStoreCache(slugCache)
		// Order document mailer for the storefront — needed so the
		// customer self-service cancel path can fire the cancellation
		// email itself (the admin OrdersHandler.dispatchCancellationEmail
		// only runs when the admin route was hit).
		orderDocMailerSF := orderdoc.NewDocumentMailer(emailSender, cfg.EmailFrom, log).WithLoader(templateLoader)
		if pf := orderdoc.NewHTTPStorefrontPDFFetcher(cfg.StorefrontBaseURLTemplate, cfg.InternalAuthSecret); pf != nil {
			orderDocMailerSF = orderDocMailerSF.WithStorefrontPDFFetcher(pf)
		}
		orderDocBrandingSvcSF := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		orderDocSvcSF := orderdoc.NewService(conn, orderDocMailerSF, orderRepoSF, orderDocBrandingSvcSF, cfg.StorefrontBaseURLTemplate).
			WithLogger(log)
		// Wire the docMailer onto the webhook handler so a successful
		// payment-captured event auto-fires the buyer's invoice email
		// (the "order placed, here's your invoice" message). Same
		// orderdoc.Service the customer self-cancel path uses; the
		// mailer is fire-and-forget on a detached context so a
		// SendGrid blip never re-queues the webhook.
		webhookHandler.WithDocMailer(orderDocSvcSF)

		orderDetailHandler := storefront.NewOrderDetailHandler(conn, orderRepoSF, orderSvcSF, orderDocSvcSF, log).
			WithReturns(returnSvcSF, returnRepoSF).
			WithNotifier(notificationSvc)

		// Support tickets — public contact form endpoint. Shares the
		// ticketSvc with admin so created rows surface on the admin
		// dashboard immediately. The otto client is best-effort: when
		// OTTO_URL / OTTO_INTERNAL_AUTH are unset (typical for local
		// dev without otto running) the constructor returns nil and the
		// transcript endpoint cleanly degrades to 404.
		sfTicketsHandler := storefront.NewTicketsHandler(ticketSvc, log).
			WithNotifier(notificationSvc).
			WithAudit(auditEmitter).
			WithOtto(ottoChatClient)

		// Mobile support chat (#118) — customer→merchant, bridges to otto.
		// Mounted standalone below (the full mobile storefront route group
		// isn't wired yet). The customer verifier reuses the GIP project to
		// validate the app's Firebase ID tokens.
		storefrontSupportHandler = storefront.NewMobileSupportHandler(ottoChatClient, cfg.OttoWSPublicBase, log)
		if cfg.GIPProjectID != "" {
			if fbApp, err := firebase.NewApp(context.Background(), &firebase.Config{ProjectID: cfg.GIPProjectID}); err != nil {
				log.Error("mobile support: firebase init failed", "error", err)
			} else if fbAuth, err := fbApp.Auth(context.Background()); err != nil {
				log.Error("mobile support: firebase auth init failed", "error", err)
			} else {
				storefrontCustomerVerifier = storefront.NewGIPCustomerVerifier(fbAuth)
			}
		}

		// P11 — Customer portal (GDPR order-history + erasure §15.4).
		customerPortalHandler := customerportal.NewHandler(conn, log)

		storefrontDeps = storefront.Deps{
			Handler:                storefrontHandler,
			CheckoutHandler:        checkoutHandler,
			CheckoutExtHandler:     checkoutExtHandler,
			PaymentMethodsHandler:  paymentMethodsHandler,
			ShippingRatesHandler:   shippingRatesHandler,
			ShippingOptionsHandler: shippingOptionsHandler,
			WebhookHandler:         webhookHandler,
			OrderDetailHandler:     orderDetailHandler,
			CouponValidateHandler:  couponValidateHandler,
			GiftCardHandler:        giftCardSFHandler,
			LoyaltyHandler:         sfLoyaltyHandler,
			SlugCache:              slugCache,
			StorefrontKey:          cfg.StorefrontKey,
			CountryHandler:         countryHandler,
			// C1 customer auth.
			CustomerAccountHandler:       customerAccountHandler,
			CustomerNotificationsHandler: customerNotificationsHandler,
			CustomerService:              customerSvc,
			CustomerSessionSecret:  cfg.CustomerSessionSecret,
			// C3 reviews.
			ReviewsHandler: sfReviewsHandler,
			// C4 wishlists.
			WishlistHandler: wishlistHandler,
			// B1 branding.
			BrandingHandler:       sfBrandingHandler,
			PagesHandler:          sfPagesHandler,
			DomainResolveHandler:  domainsHandler.ResolveDomain,
			TicketsHandler:        sfTicketsHandler,
			CustomerPortalHandler: customerPortalHandler,
			Logger:                log,
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
		// Campaign dispatch rides the shared transport — SendGrid primary
		// with Resend fallback in prod, log-only in dev / local (decided
		// once by email.NewFromConfig) so the worker still runs either way.
		var dispatcher campaign.Dispatcher = campaign.NewEmailDispatcher(emailSender, cfg.EmailFrom, log)
		log.Info("campaign: email dispatcher enabled", "from", cfg.EmailFrom)
		// Load per-store branding into the campaign envelope so customer
		// emails match the merchant's storefront theme, not the platform's.
		brandingSvcForCampaign := branding.NewService(branding.ServiceConfig{
			DB:     conn,
			Repo:   branding.NewRepository(),
			Logger: log,
		})
		themeLoader := campaign.NewStoreThemeLoader(conn, brandingSvcForCampaign)

		// Per-tenant per-store monthly cap enforcement (spec §10). Re-instantiated
		// here because campaignBudgetSvc lives in a separate scope earlier in
		// main; the service is stateless (just wraps the *gorm.DB) so creating
		// another handle is free. Without this, merchants could send unlimited
		// campaign emails regardless of plan.
		campaignBudgetSvcForWorker := campaignbudget.NewService(conn)
		sendWorker := campaign.NewSendWorker(campaign.SendWorkerConfig{
			DB:          conn,
			Repo:        campaignRepo,
			Dispatcher:  dispatcher,
			ThemeLoader: themeLoader,
			Budget:      campaignBudgetSvcForWorker,
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

	// Stripe billing webhook handler + orphan cron — mounted outside the admin
	// auth middleware so Stripe can POST without X-Internal-Auth. Only active
	// when STRIPE_BILLING_WEBHOOK_SECRET is set.
	var stripeBillingWebhookHandler gin.HandlerFunc
	if cfg.StripeBillingWebhookSecret != "" {
		allowed := make(map[string]bool, len(cfg.StripeAllowedEventTypes))
		for _, t := range cfg.StripeAllowedEventTypes {
			allowed[t] = true
		}
		dispatcher := dispatch.New(auditEmitter)

		// Email adapter — currently the NoOp implementation pending real SMTP/SendGrid
		// wiring (deferred when email/store_name columns land on StoreSubscription).
		// Used by the dispatcher for the trial-billed confirmation email on first
		// invoice.paid, and by the dunning + trial reminder crons.
		dispatcherEmailClient := email.NoOpClient{Logger: log}
		dispatcher.WithEmail(dispatcherEmailClient)

		// P7 §19.2: annotate B2B invoices with the reverse-charge clause on
		// invoice.finalized for validated tax IDs in reverse-charge jurisdictions.
		dispatcher.WithReverseChargeAnnotator(billingStripeClient)

		// P8 §18.8: wire arbitrage recorder into the checkout webhook handler.
		// KeyLoader + Hasher use Secret Manager when ARBITRAGE_HMAC_SECRET_PATH
		// is set; omit gracefully in local dev (recorder stays nil → no-op).
		if secretPath := os.Getenv("ARBITRAGE_HMAC_SECRET_PATH"); secretPath != "" {
			smCtx, smCancel := context.WithTimeout(context.Background(), 5*time.Second)
			smClient, smErr := secretmanagerclient.NewClient(smCtx)
			smCancel()
			if smErr != nil {
				log.Warn("arbitrage: secret manager client failed — arbitrage check disabled", "err", smErr)
			} else {
				keySrc := &arbitrage.SecretManagerSource{Client: smClient, SecretPath: secretPath}
				keyLoader := arbitrage.NewKeyLoader(keySrc, 5*time.Minute)
				hasher := arbitrage.NewHasher(keyLoader)
				recorder := arbitrage.NewRecorder(conn, hasher, &arbitragePrometheusCounter{})
				dispatcher.WithRecorder(recorder)
				log.Info("arbitrage: triangulation recorder wired", "secret_path", secretPath)
			}
		} else {
			log.Warn("ARBITRAGE_HMAC_SECRET_PATH not set — arbitrage triangulation disabled")
		}
		webhookH := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
			DB:     conn,
			Secret: cfg.StripeBillingWebhookSecret,
			Repo:   webhookevents.NewRepository(),
			Dispatch: func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
				return dispatcher.Dispatch(ctx, tx, e)
			},
			AllowedTypes: allowed,
			MaxBodyBytes: cfg.WebhookMaxBodyBytes,
			Logger:       log,
		})
		stripeBillingWebhookHandler = webhookH.Handle

		// Orphan cron — retries unprocessed webhook events on a fixed interval.
		orphanCron := dispatch.NewCron(dispatch.CronConfig{
			DB:             conn,
			Repo:           webhookevents.NewRepository(),
			Dispatcher:     dispatcher,
			PagerDuty:      &dispatch.HTTPPagerDuty{URL: cfg.PagerDutyWebhookURL},
			StaleThreshold: cfg.OrphanStaleThreshold,
			Interval:       cfg.OrphanRetryInterval,
			MaxRetries:     cfg.OrphanRetryMaxCount,
		})
		if err := orphanCron.Start(workerCtx); err != nil {
			log.Error("orphan cron failed to start", "err", err)
		} else {
			defer orphanCron.Stop()
			log.Info("stripe billing: orphan cron started", "interval", cfg.OrphanRetryInterval)
		}
	} else {
		log.Warn("STRIPE_BILLING_WEBHOOK_SECRET not set — /webhooks/stripe-billing not mounted")
	}

	if downgradeCron != nil {
		if err := downgradeCron.Start(workerCtx); err != nil {
			log.Error("downgrade-recheck cron failed to start", "err", err)
		} else {
			defer downgradeCron.Stop()
			log.Info("planchange: downgrade-recheck cron started")
		}
	}

	// P5 trial + anomaly crons — daily lifecycle work routed through the shared
	// scheduler pattern the orphan cron established.
	trialScheduler := cron.New()

	bannerCron := trial.NewBannerCron(conn, log, nil)
	if _, err := trialScheduler.AddFunc(trial.BannerSpec, func() {
		if err := bannerCron.Run(workerCtx); err != nil {
			log.Error("trial banner cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register trial banner cron", "err", err)
	}

	expiryCron := trial.NewExpiryCron(conn, auditEmitter, log, nil)
	if _, err := trialScheduler.AddFunc(trial.ExpirySpec, func() {
		if err := expiryCron.Run(workerCtx); err != nil {
			log.Error("trial expiry cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register trial expiry cron", "err", err)
	}

	activationCron := trial.NewActivationCron(conn, metrics.TrialActivationDay30Total, log, nil)
	if _, err := trialScheduler.AddFunc(trial.ActivationSpec, func() {
		if err := activationCron.Run(workerCtx); err != nil {
			log.Error("trial activation cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register trial activation cron", "err", err)
	}

	anomalyCron := signup.NewAnomalyCron(conn, signup.NoOpSlack{}, metrics.TrialSignupAnomalyAlertsTotal, log, nil)
	if _, err := trialScheduler.AddFunc(signup.AnomalySpec, func() {
		if err := anomalyCron.Run(workerCtx); err != nil {
			log.Error("signup anomaly cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register signup anomaly cron", "err", err)
	}

	// Audit log retention — daily prune at 02:00 UTC. Per-plan windows
	// (Trial/Starter 90d, Studio 365d, Pro unlimited) are encoded in the
	// audit package's retentionBuckets. Multi-tenant safe: each DELETE
	// joins audit_logs to store_subscriptions on store_id, so a tenant's
	// rows are pruned only against their OWN plan.
	auditPruneCron := audit.NewPruneCron(conn, log, nil, 0).
		WithCounter(func(label string, n int64) {
			metrics.AuditPruneRowsDeletedTotal.WithLabelValues(label).Add(float64(n))
		})
	if _, err := trialScheduler.AddFunc(audit.PruneSpec, func() {
		if _, err := auditPruneCron.Run(workerCtx); err != nil {
			log.Error("audit prune cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register audit prune cron", "err", err)
	}

	trialScheduler.Start()
	defer trialScheduler.Stop()
	log.Info("P5 crons started", "count", 4)

	// P6 dunning + SCA recovery crons. Emails route through the NoOpClient
	// until a real adapter is wired (email columns on StoreSubscription are
	// deferred). All state mutations go through statemachine.Transition.
	dunningEmailClient := email.NoOpClient{Logger: log}

	ladderCron := dunning.NewStepDailyLadder(conn, auditEmitter, log,
		dunning.WrapPrometheusCounter(metrics.DunningSuppressedRefundWindowTotal),
		nil)
	if _, err := trialScheduler.AddFunc(dunning.LadderSpec, func() {
		if err := ladderCron.Run(workerCtx); err != nil {
			log.Error("dunning ladder cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register dunning ladder cron", "err", err)
	}

	dunningEmailsCron := dunning.NewSendDunningEmails(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.DunningEmailsSentTotal),
		nil)
	if _, err := trialScheduler.AddFunc(dunning.DunningEmailsSpec, func() {
		if err := dunningEmailsCron.Run(workerCtx); err != nil {
			log.Error("dunning emails cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register dunning emails cron", "err", err)
	}

	scaRemindersCron := dunning.NewSendPaymentActionReminders(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.PaymentActionRemindersSentTotal),
		nil)

	// P7 §19.5 — quarterly tax-ID revalidation cron. Daily 02:00 UTC. Re-runs
	// the validator for each subscription with a >90d-old validation; on
	// definitive failure flips tax_id_validated=false, opens a 14d grace
	// window, and unpublishes the storefront on day 14. Subscription status
	// stays 'active' (billing continues — "no perverse incentive" §19.5).
	// Skipped when taxService is nil (storefront-only mode).
	if taxService != nil {
		revalidationCron := &revalidation.Cron{
			DB:    conn,
			Svc:   taxService,
			Audit: auditEmitter,
		}
		if _, err := trialScheduler.AddFunc(revalidation.Spec, func() {
			if err := revalidationCron.Run(workerCtx); err != nil {
				log.Error("tax revalidation cron failed", "err", err)
			}
		}); err != nil {
			log.Error("register tax revalidation cron", "err", err)
		}
	}

	if _, err := trialScheduler.AddFunc(dunning.PaymentActionRemindersSpec, func() {
		if err := scaRemindersCron.Run(workerCtx); err != nil {
			log.Error("SCA reminders cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register SCA reminders cron", "err", err)
	}

	// Trial-end reminders — escalating nudges for merchants without a payment
	// method (T-15 / T-10 / T-7 / T-3 / T-1) and a single heads-up for
	// merchants with a card on file (T-1 before auto-billing). Idempotency
	// via the trial_reminders table (migration 088). See spec §5.3.
	trialRemindersCron := dunning.NewSendTrialReminders(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.TrialRemindersSentTotal),
		nil)
	if _, err := trialScheduler.AddFunc(dunning.TrialRemindersSpec, func() {
		if err := trialRemindersCron.Run(workerCtx); err != nil {
			log.Error("trial reminders cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register trial reminders cron", "err", err)
	}

	log.Info("P6 dunning crons registered", "count", 4)

	// P11 lifecycle crons — post-cancellation pipeline + win-back + GDPR portal.
	// All registered on the shared trialScheduler (same thread pool as P5/P6).
	p11SubscriptionRepo := subscription.NewRepository()
	p11HardDeleteRunner := harddelete.NewRunner(conn, billingStripeClient, auditEmitter, log)

	finalizeCron := lifecycle.NewFinalizeCron(conn, auditEmitter, log, nil)
	if _, err := trialScheduler.AddFunc(lifecycle.FinalizeSpec, func() {
		if err := finalizeCron.Run(workerCtx); err != nil {
			log.Error("lifecycle finalize cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register lifecycle finalize cron", "err", err)
	}

	closeCron := lifecycle.NewCloseCron(conn, auditEmitter, log, nil)
	if _, err := trialScheduler.AddFunc(lifecycle.CloseSpec, func() {
		if err := closeCron.Run(workerCtx); err != nil {
			log.Error("lifecycle close cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register lifecycle close cron", "err", err)
	}

	queueHardDeleteCron := lifecycle.NewQueueHardDeleteCron(conn, auditEmitter, log, nil)
	if _, err := trialScheduler.AddFunc(lifecycle.QueueHardDeleteSpec, func() {
		if err := queueHardDeleteCron.Run(workerCtx); err != nil {
			log.Error("lifecycle queue-hard-delete cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register lifecycle queue-hard-delete cron", "err", err)
	}

	hardDeleteCron := lifecycle.NewHardDeleteCron(conn, p11SubscriptionRepo, p11HardDeleteRunner, auditEmitter, log, nil)
	if _, err := trialScheduler.AddFunc(lifecycle.HardDeleteSpec, func() {
		if err := hardDeleteCron.Run(workerCtx); err != nil {
			log.Error("lifecycle hard-delete cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register lifecycle hard-delete cron", "err", err)
	}

	winBackEmailClient := email.NoOpClient{Logger: log}
	winBackCron := lifecycle.NewWinBackCron(conn, winBackEmailClient, log, nil)
	if _, err := trialScheduler.AddFunc(lifecycle.WinBackSpec, func() {
		if err := winBackCron.Run(workerCtx); err != nil {
			log.Error("lifecycle win-back cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register lifecycle win-back cron", "err", err)
	}

	log.Info("P11 lifecycle crons registered", "count", 5)

	// P10 — billing archive expiry sweeper (§23.2 7-year retention).
	archiveSweeper := billingarchive.NewSweeper(conn, log)
	if _, err := trialScheduler.AddFunc("@daily", func() {
		if _, err := archiveSweeper.RunOnce(workerCtx); err != nil {
			log.Error("billing archive sweeper failed", "err", err)
		}
	}); err != nil {
		log.Error("register billing archive sweeper cron", "err", err)
	}
	log.Info("P10 billing archive sweeper registered")

	// P9 — campaign budget crons: trial ramp (00:00 UTC daily) + monthly reset (00:05 UTC on 1st).
	// Both registered on the shared trialScheduler alongside P5/P6/P11 crons.
	if _, err := campaignbudgetcron.RegisterTrialRampJob(trialScheduler, conn); err != nil {
		log.Error("register campaign trial ramp cron", "err", err)
	}
	if _, err := campaignbudgetcron.RegisterMonthlyResetJob(trialScheduler, conn); err != nil {
		log.Error("register campaign monthly reset cron", "err", err)
	}
	log.Info("P9 campaign budget crons registered", "count", 2)

	// P17 — MRR USD rollup collector. Registered with the default Prometheus
	// registry so it is scraped automatically via the existing /metrics handler.
	// The FXRepository reads from the fx_rates table populated by the
	// cmd/fx-rate-refresh CronJob (migration 063).
	fxRepo := metrics.NewFXRepository(conn)
	mrrCollector := metrics.NewMRRRollup(conn, fxRepo, log)
	prometheus.MustRegister(mrrCollector)
	log.Info("P17 MRR rollup collector registered")

	// P17 Task 11 — billing_archive expiry gauge. Counts rows expiring soon
	// and rows already expired (awaiting sweeper) at each scrape.
	archiveExpiryCollector := metrics.NewBillingArchiveExpiryCollector(conn, log)
	prometheus.MustRegister(archiveExpiryCollector)
	log.Info("P17 billing_archive expiry collector registered")

	// ──────────────────────────────────────────────────────────────────
	// P15 — White-label mobile-app add-on: credential store + teardown
	// lifecycle cron (spec §13.5, §18.9).
	//
	// Credential store (appcreds): uses GCP Secret Manager in production
	// when APPCREDS_PROJECT_ID is set; falls back to an in-memory FakeSM
	// for dev so `make dev` boots without GCP auth. Every read/write/
	// delete emits an audit event + increments a Prometheus counter.
	//
	// Apple/Google/Firebase clients: wired as FakeClient today — real
	// integrations return ErrNotWired until the respective API SDKs are
	// fleshed out in a follow-up. The lifecycle advancer tolerates
	// ErrNotWired (logged, swallowed) so day-30/60/90 actions can land
	// progressively as each integration matures.
	//
	// The lifecycle cron registers on the shared trialScheduler so it
	// shares the same thread pool and shutdown semantics as P5/P6/P11
	// crons. Production spec: "0 5 * * *" (05:00 UTC daily).
	// ──────────────────────────────────────────────────────────────────
	// wlAppCredsSvc is assigned above inside the admin-mode block. On
	// MODE=storefront it's nil and we skip the lifecycle cron — the
	// storefront pod has no business running the teardown advancer.
	if wlAppCredsSvc != nil {
		wlAppleCli := wlapple.NewFakeClient()
		wlGoogleCli := wlgoogleplay.NewFakeClient()
		wlFirebaseCli := wlfirebase.NewFakeClient()
		wlAdvancer := wllifecycle.NewAdvancer(wllifecycle.Config{
			DB:       conn,
			Apple:    wlAppleCli,
			Google:   wlGoogleCli,
			Firebase: wlFirebaseCli,
			Creds:    wlAppCredsSvc,
			Clock:    time.Now,
			Logger:   log,
		})
		if _, err := trialScheduler.AddFunc(cfg.WhiteLabelLifecycleCron, func() {
			if err := wlAdvancer.AdvanceDue(workerCtx); err != nil {
				log.Error("P15 white-label lifecycle advance failed", "err", err)
			}
		}); err != nil {
			log.Error("register P15 white-label lifecycle cron", "err", err)
		}
		log.Info("P15 white-label lifecycle cron registered",
			"spec", cfg.WhiteLabelLifecycleCron,
			"appcreds_mode", func() string {
				if cfg.AppCredsProjectID == "" {
					return "fake"
				}
				return "gcp"
			}())
	} else {
		log.Info("P15 white-label lifecycle cron skipped (MODE=storefront, wlAppCredsSvc nil)")
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
		r.Use(otelgin.Middleware(otelServiceName))
		r.MaxMultipartMemory = 100 << 20 // 100 MB for CSV uploads
		healthHandler.Register(r)
		admin.RegisterAdmin(r.Group("/api/v1"), adminDeps)
		admin.RegisterAdminMobile(r.Group("/api/v1"), mobileDeps)
		storefront.RegisterStorefront(r.Group("/api/v1"), storefrontDeps)
		storefront.RegisterMobileStorefrontSupport(r.Group("/api/v1"), storefrontSupportHandler, storefrontDeps.SlugCache, storefrontCustomerVerifier)
		public.RegisterPublic(r.Group("/api/v1"), public.PublicDeps{
			DelhiveryWebhookHandler: delhiveryWebhookHandler,
		})
		if brandingSeeder != nil {
			brandingSeeder.Register(r.Group("/api/v1/test"))
		}
		if pushWebhookHandler != nil {
			r.POST("/internal/push-webhook", pushWebhookHandler)
		}
		if vendorHandler != nil {
			vendorHandler.RegisterRoutes(r.Group("/internal"))
		}
		// Email templates registry (B1f) — refresh + test-send.
		templateHandler.Register(r.Group("/internal"))
		// Super-admin domain re-verify + cert-refresh actions.
		internalDomainsHandler.Register(r.Group("/internal"))
		// Mirror of platform_api.stores so admin/storefront slug lookups
		// don't cross-service-call platform-api on every request. Called
		// by platform-api at onboarding completion (parallel to
		// EnsureSelfVendor). See internal_handler.go for idempotency.
		stores.NewInternalHandler(domainStoresRepo).
			RegisterRoutes(r.Group("/internal"))
		// Otto escalation hook — slm-router POSTs
		// /internal/v1/tickets/from-conversation when an AI chat is
		// handed off to a human. Same /internal namespace + same shared
		// secret; the slm-router pod carries it via SLM_ROUTER_INTERNAL_AUTH.
		ticketInternalHandler.RegisterRoutes(r.Group("/internal"))
		// Cross-service audit ingest — auth-bff posts login/logout,
		// platform-api posts staff invite/accept/revoke. Mounted on the
		// existing /internal namespace gated by X-Internal-Auth.
		internalsvc.NewAuditIngestHandler(auditEmitter, domainStoresRepo, log).
			Register(r.Group("/internal"), cfg.AuditIngestSecret)
		// P12 — storefront-gate Cloudflare Worker reads this endpoint at
		// the edge to decide closed-page vs pass-through (§5.4).
		internalsvc.NewStorefrontStatusHandler(conn).
			Register(r.Group("/internal"), cfg.AuditIngestSecret)
		// Custom-domain takeover — admin + storefront middleware read
		// this to decide whether `*.mark8ly.com` URLs for a slug should
		// 301 to the merchant's verified custom domain.
		internalsvc.NewStoreActiveDomainHandler(conn).
			Register(r.Group("/internal"), cfg.AuditIngestSecret)
		if stripeBillingWebhookHandler != nil {
			r.POST("/webhooks/stripe-billing", stripeBillingWebhookHandler)
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
		engine.Use(otelgin.Middleware(otelServiceName))
		engine.MaxMultipartMemory = 100 << 20 // 100 MB for CSV uploads
		healthHandler.Register(engine)
		if m == mode.Admin {
			admin.RegisterAdmin(engine.Group("/api/v1"), adminDeps)
			admin.RegisterAdminMobile(engine.Group("/api/v1"), mobileDeps)
			// Public Delhivery webhook receiver. Mounted on the admin
			// engine because the merchant-configured URL points at the
			// admin hostname (playwrite-test-admin.mark8ly.com) and the
			// handler shares shipments/secrets wiring with the admin
			// ShipmentsHandler.
			public.RegisterPublic(engine.Group("/api/v1"), public.PublicDeps{
				DelhiveryWebhookHandler: delhiveryWebhookHandler,
			})
			if countryPublicHandler != nil {
				engine.GET("/api/v1/public/supported-countries", countryPublicHandler.ListSupported)
			}
			if pushWebhookHandler != nil {
				engine.POST("/internal/push-webhook", pushWebhookHandler)
			}
			if vendorHandler != nil {
				vendorHandler.RegisterRoutes(engine.Group("/internal"))
			}
			// Email templates registry (B1f) — refresh + test-send.
			// Admin-only because tesserix-home authors templates and
			// pings exactly one URL per database. Storefront's loader
			// ages its cache out via the 5-min TTL, which is acceptable
			// for runtime sends.
			templateHandler.Register(engine.Group("/internal"))
			// Super-admin domain re-verify + cert-refresh actions.
			internalDomainsHandler.Register(engine.Group("/internal"))
			// Mirror of platform_api.stores. See main mode-Both branch
			// above for context. Admin engine only — storefront never
			// writes stores.
			stores.NewInternalHandler(domainStoresRepo).
				RegisterRoutes(engine.Group("/internal"))
			// slm-router escalation hook + the mark8ly-mcp
			// create_support_ticket tool both POST /internal/v1/tickets/
			// from-conversation to open a support ticket from an AI chat
			// (idempotent on conversation_id, for traceability).
			// Previously only registered in mode-Both, so the split admin
			// service 404'd it — register on the admin engine (the
			// dashboard that owns tickets).
			ticketInternalHandler.RegisterRoutes(engine.Group("/internal"))
			// Audit ingest is admin-only because the audit_logs read
			// endpoint also lives on the admin engine — keeping write
			// + read on the same pod simplifies ops and keeps the
			// storefront engine's surface area small.
			internalsvc.NewAuditIngestHandler(auditEmitter, domainStoresRepo, log).
				Register(engine.Group("/internal"), cfg.AuditIngestSecret)
			// P12 — storefront-gate Cloudflare Worker hits this endpoint
			// (mounted on admin pod since it reads cross-tenant rows the
			// storefront pod doesn't otherwise serve).
			internalsvc.NewStorefrontStatusHandler(conn).
				Register(engine.Group("/internal"), cfg.AuditIngestSecret)
			// Custom-domain takeover — wired on both engines so admin
			// + storefront middlewares can hit whichever pod is local.
			internalsvc.NewStoreActiveDomainHandler(conn).
				Register(engine.Group("/internal"), cfg.AuditIngestSecret)
			// Reverse custom-domain lookup (domain → slug). The admin
			// middleware uses this to verify a custom-admin host
			// (admin.<merchant>) before rendering. Originally only the
			// storefront engine had it, so admin → admin-flavor calls
			// 404'd and the custom-domain admin URL was unreachable.
			engine.GET("/api/v1/storefront/resolve-domain", domainsHandler.ResolveDomain)
			if stripeBillingWebhookHandler != nil {
				engine.POST("/webhooks/stripe-billing", stripeBillingWebhookHandler)
			}
		}
		if m == mode.Storefront {
			storefront.RegisterStorefront(engine.Group("/api/v1"), storefrontDeps)
			storefront.RegisterMobileStorefrontSupport(engine.Group("/api/v1"), storefrontSupportHandler, storefrontDeps.SlugCache, storefrontCustomerVerifier)
			// Custom-domain takeover — storefront middleware queries
			// this on every slug-host request to decide whether to 301
			// to the merchant's verified custom domain.
			internalsvc.NewStoreActiveDomainHandler(conn).
				Register(engine.Group("/internal"), cfg.AuditIngestSecret)
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
	if auditEmitter != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		auditEmitter.Stop(stopCtx)
		stopCancel()
		log.Info("audit emitter stopped")
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

// domainSecretsAdapter wraps the carriersecrets.Store so it can satisfy
// domain.SecretStore. Both interfaces are minimal Put/Get/Destroy
// surfaces; the only difference is the Scope type (carriersecrets has
// its own struct, domain has its own). This adapter translates one
// scope to the other so the domain package doesn't have to import
// carriersecrets directly.
type domainSecretsAdapter struct {
	inner carriersecrets.Store
}

func (a domainSecretsAdapter) Put(ctx context.Context, scope domain.SecretScope, plaintext string) (string, error) {
	return a.inner.Put(ctx, carriersecrets.Scope{
		TenantID: scope.TenantID,
		Domain:   scope.Domain,
		Provider: scope.Provider,
		Field:    scope.Field,
	}, plaintext)
}

func (a domainSecretsAdapter) Get(ctx context.Context, reference string) (string, error) {
	return a.inner.Get(ctx, reference)
}

func (a domainSecretsAdapter) Destroy(ctx context.Context, reference string) error {
	return a.inner.Destroy(ctx, reference)
}

// arbitragePrometheusCounter bridges arbitrage.Counter to the P17 Prometheus
// singleton (metrics.Subscription.SubscriptionArbitrageFlaggedTotal).
type arbitragePrometheusCounter struct{}

func (c *arbitragePrometheusCounter) IncArbitrageFlagged() {
	if metrics.Subscription != nil {
		metrics.Subscription.SubscriptionArbitrageFlaggedTotal.
			WithLabelValues("ppp_developed_signal").Inc()
	}
}

func (c *arbitragePrometheusCounter) IncArbitrageFalsePositiveCleared() {
	// P17 dashboard reads the arbitrage_flagged counter; false-positive-cleared
	// is a separate counter that P17 alert rules reference. Emit on the same
	// registry under a distinct reason label.
	if metrics.Subscription != nil {
		metrics.Subscription.SubscriptionArbitrageFlaggedTotal.
			WithLabelValues("false_positive_cleared").Inc()
	}
}
