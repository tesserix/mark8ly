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
	"github.com/google/uuid"
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
	"github.com/mark8ly/marketplace-api/internal/authbffclient"
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
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/billingarchive"
	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
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
	"github.com/mark8ly/marketplace-api/internal/displayname"
	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailevents"
	"github.com/mark8ly/marketplace-api/internal/emaillog"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/estatecounts"
	"github.com/mark8ly/marketplace-api/internal/estateuserdir"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/gipkey"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/internalsvc"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/handlers/testroutes"
	"github.com/mark8ly/marketplace-api/internal/handlers/webhooks"
	"github.com/mark8ly/marketplace-api/internal/health"
	"github.com/mark8ly/marketplace-api/internal/ipprivacy"
	"github.com/mark8ly/marketplace-api/internal/journal"
	"github.com/mark8ly/marketplace-api/internal/k8sprov"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/observability"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/ottoclient"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/payment/stripewebhook"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/push"
	"github.com/mark8ly/marketplace-api/internal/pushevents"
	"github.com/mark8ly/marketplace-api/internal/refund"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/shipmentcancel"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/signup"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/storeidentity"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/internal/subscription/harddelete"
	"github.com/mark8ly/marketplace-api/internal/subscription/lifecycle"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/internal/subscription/readonly"
	"github.com/mark8ly/marketplace-api/internal/teamproxy"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/internal/userprofile"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/internal/webhookprune"
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
		PriceID:    "", // placeholder — real price ID populated once the catalog's Prices exist in Stripe (published by the console, #303); 400s here are pre-existing (nil client = always broken)
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

// trialStripeAdapter adapts the billing Stripe client to
// trial.StripeTrialUpdater. It exists so the trial package depends on a
// two-method interface it declares itself rather than on the whole client.
type trialStripeAdapter struct{ c *billingstripe.Client }

func (a *trialStripeAdapter) GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error) {
	return billingstripe.GetSubscription(ctx, a.c, id)
}

func (a *trialStripeAdapter) UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	return billingstripe.UpdateTrialEnd(ctx, a.c, in)
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

// lifecycleSkipCounter adapts the shared skipped-emails CounterVec to
// lifecycle.SkipCounter. Separate from dunning's adapter because the two
// packages declare their own consumer-side interfaces.
type lifecycleSkipCounter struct{ cv *prometheus.CounterVec }

func (l lifecycleSkipCounter) WithTemplateReason(template, reason string) lifecycle.CounterIncrementer {
	return l.cv.WithLabelValues(template, reason)
}

// lifecycleSentCounter adapts the shared delivered-emails CounterVec to
// lifecycle.SentCounter — win_back_day30 has no per-feature sent counter of
// its own, so without this the sent+skipped identity would be false for it.
type lifecycleSentCounter struct{ cv *prometheus.CounterVec }

func (l lifecycleSentCounter) WithTemplate(template string) lifecycle.CounterIncrementer {
	return l.cv.WithLabelValues(template)
}

// dispatchSkipCounter / dispatchSentCounter do the same for the
// trial_started_billed confirmation emitted from the invoice.paid webhook.
type dispatchSkipCounter struct{ cv *prometheus.CounterVec }

func (d dispatchSkipCounter) WithTemplateReason(template, reason string) dispatch.CounterIncrementer {
	return d.cv.WithLabelValues(template, reason)
}

type dispatchSentCounter struct{ cv *prometheus.CounterVec }

func (d dispatchSentCounter) WithTemplate(template string) dispatch.CounterIncrementer {
	return d.cv.WithLabelValues(template)
}

// trialSkipCounter / trialSentCounter do the same for the trial_expired
// notice emitted by the trial expiry cron.
type trialSkipCounter struct{ cv *prometheus.CounterVec }

func (t trialSkipCounter) WithTemplateReason(template, reason string) trial.CounterIncrementer {
	return t.cv.WithLabelValues(template, reason)
}

type trialSentCounter struct{ cv *prometheus.CounterVec }

func (t trialSentCounter) WithTemplate(template string) trial.CounterIncrementer {
	return t.cv.WithLabelValues(template)
}

// migrationSkipCounter / migrationSentCounter do the same again for the
// migration fast-path decision notices (#703).
//
// A third near-identical pair rather than reusing the trial ones: each
// package declares its own CounterIncrementer, and Go requires the return
// type to match exactly, so trialSentCounter cannot satisfy
// migration.SentCounter however alike they look.
type migrationSkipCounter struct{ cv *prometheus.CounterVec }

func (m migrationSkipCounter) WithTemplateReason(template, reason string) migration.CounterIncrementer {
	return m.cv.WithLabelValues(template, reason)
}

type migrationSentCounter struct{ cv *prometheus.CounterVec }

func (m migrationSentCounter) WithTemplate(template string) migration.CounterIncrementer {
	return m.cv.WithLabelValues(template)
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

	// Journal "coming soon" page email capture (#153). Built here,
	// unconditionally, rather than inside the admin-only wiring block
	// below: unlike delhiveryWebhookHandler it has no admin-specific
	// dependency (shipments, carrier secrets) — it only needs the DB
	// connection every mode already has. Mounted via RegisterPublic on
	// the same two engines (mode.Both, mode.Admin) as the rest of the
	// public group; see internal/handlers/public/routes.go for why a
	// tenant-free record is not on mode.Storefront.
	journalSubscribeHandler := public.NewJournalSubscribeHandler(
		journal.NewRepository(conn), journal.NewRateLimiter(), log)
	// Erasure counterpart to journalSubscribeHandler (migration 000125)
	// — a separate journal.RateLimiter instance rather than sharing
	// journalSubscribeHandler's, so a burst against one endpoint can't
	// lock a subscriber out of the other.
	journalUnsubscribeHandler := public.NewJournalUnsubscribeHandler(
		journal.NewRepository(conn), journal.NewRateLimiter(), log)

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
	// #381 — billing mail (dunning, trial cadence, payment-action, win-back,
	// trial-billed). Registered AFTER SeedFromEmbedded, deliberately: spec §4
	// says "no seed migration ... a key with no row simply renders from its
	// embedded default". Registration makes a key overridable from the
	// operator console; seeding it as a published row would mean the first
	// boot wins forever, because SeedFromEmbedded is ON CONFLICT DO NOTHING
	// and Loader.Render prefers a published row — so a later edit to
	// templates_content.go would deploy and silently never reach a merchant.
	// orderdoc/giftcard keep their existing seed-then-render behaviour: they
	// are registered above and so still seed, unchanged.
	email.RegisterFallbacks(templateLoader)
	// Test-send dispatcher used by /internal/templates/:key/test. nil
	// SendGrid key is acceptable — handler returns 503 with a clear
	// message so dev users don't get silent fail.
	templateTestSender := emailtemplates.NewSendGridTestSender(cfg.SendGridAPIKey, cfg.EmailFrom)
	templateHandler := emailtemplates.NewHandler(templateLoader, templateTestSender)
	// The same loader instance backs the platform admin authoring surface
	// (tesserix-home#588), which is why a save there can evict the send
	// path's cache in-process instead of pinging /internal/templates/refresh
	// over HTTP the way tesserix-home had to after its cross-DB write.
	templateStore := emailtemplates.NewStore(conn)

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

	// Per-tenant carrier credential store. SHIPPING_SECRET_STORE selects
	// which backend is PRIMARY (where Put writes, and which reference
	// format Get treats as "not a fallback"); it never adds a second,
	// independent selector — see internal/carriersecrets/chain.go.
	//
	//   - "inline" (default, unchanged): NewInlineStore, no GCP/OpenBao
	//     creds needed — keeps local dev working exactly as before.
	//   - "gcpsm" (unchanged behaviour): a ChainStore with
	//     Primary: BackendGCP, UNCACHED — reads gsm:// and inline
	//     references exactly as the old HybridStore did, with no
	//     wrapper of any kind. This is deliberate: merging this task
	//     changes nothing on the default/existing path. A CachingStore
	//     is not merely a performance detail — it would introduce
	//     up-to-60s plaintext credential residency where there is none
	//     today, up-to-60s staleness after a rotation from another
	//     process, and (most significantly) stale-on-error, which would
	//     turn a GCP Secret Manager outage that today surfaces
	//     immediately as an error into one silently masked by a stale
	//     cached value. None of that is acceptable to introduce on a
	//     path nobody asked to change.
	//   - "bao" (new, opt-in): a ChainStore with Primary: BackendBao,
	//     wrapped in a CachingStore. Writes mint bao://; gsm:// and
	//     inline references keep reading exactly as before, so rows
	//     already in GCP Secret Manager keep resolving while new writes
	//     land in OpenBao. The cache is intentionally bao-only: it
	//     arrives bundled with the same deliberate, monitored config
	//     change that switches the backend, rather than riding in
	//     unannounced on "gcpsm" (see above).
	//
	// A real GCP Secret Manager client is required in both "gcpsm" and
	// "bao" modes — a bao-primary chain still has to serve pre-cutover
	// gsm:// rows — so the workload-identity-ADC-and-fallback-to-inline
	// dance below is shared by both. config.Validate already refused to
	// boot with SHIPPING_SECRET_STORE=bao and no OPENBAO_ROLE, no
	// GCP_PROJECT_ID, or a non-"kv" OPENBAO_KV_MOUNT
	// (carriersecrets.BaoPath hardcodes the "kv/" path prefix
	// independently of whatever mount the OpenBao client is configured
	// with, so a mismatch there would otherwise fail every read/write
	// at runtime instead of at boot).
	// The construction itself — the mode switch this comment block
	// describes — lives in internal/carriersecrets.Build, shared with
	// cmd/refund-sweep-cron so the two callers cannot drift the way that
	// produced mark8ly#166 (the cron never got the "bao" mode the API
	// did, because the switch was duplicated instead of shared). Build
	// never calls os.Exit; every failure it can't degrade past comes
	// back as an error, and this is where that decision (exit vs.
	// continue) is made for the API process specifically.
	carrierSecretStore, carrierSecretStoreDegraded, buildErr := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         cfg.ShippingSecretStore,
		OpenBaoAddr:  cfg.OpenBaoAddr,
		OpenBaoMount: cfg.OpenBaoKVMount,
		OpenBaoRole:  cfg.OpenBaoRole,
		Encryptor:    apiKeyEncryptor,
		Logger:       log,
		Counter:      metrics.CarrierSecretCounter,
	})
	if buildErr != nil {
		log.Error("carriersecrets: build failed", "err", buildErr, "shipping_secret_store", cfg.ShippingSecretStore)
		os.Exit(1)
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
		// Derived from the encryption key rather than a new secret so
		// there is nothing extra to deploy; rotating it re-issues every
		// outstanding domain token.
		ChallengeSecret: cfg.EncryptionKey,
		GIPKey:          gipKeyClient,
		Logger:          log,
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
	// tenantGate (#287) is declared at outer scope so the platformadmin.Register
	// call sites in the mode switch below (which construct the tenant
	// lifecycle handler's invalidator) can see the same instance the admin
	// group's RequireActiveTenant middleware reads from — one process, one
	// cache. Constructed inside the admin wiring branch below; stays nil in
	// Storefront-only builds.
	var tenantGate *tenantgate.Gate
	// brandingSeeder is non-nil only when MARKETPLACE_API_ENABLE_TEST_ROUTES=true.
	// Declared at func scope so the later route-mount block can see it.
	var brandingSeeder *testroutes.BrandingSeeder
	// migrationHandler is constructed in the admin wiring branch but its
	// CSM-review route is mounted on the /internal group inside both
	// engine-switch cases below, so declare it at outer scope (same
	// reason as brandingSeeder above).
	var migrationHandler *migration.Handler
	// migrationRepo is hoisted for the same reason as migrationHandler: the
	// inbox action executor (#281a) is built in both engine-switch cases
	// below, outside the admin wiring branch that constructs it.
	var migrationRepo *migration.Repository
	// customerEraser is hoisted for the same reason: the erasure inbox action
	// (#259) is built in both engine-switch cases below. It is an INTERFACE,
	// not a *customererasure.Executor, so the "no database" case stays an
	// untyped nil that inboxActionExecutors can actually test.
	var customerEraser platformadmin.CustomerEraser
	// downgradeCron is non-nil only when STRIPE_BILLING_SECRET_KEY is set.
	// Declared at func scope so the cron-start block below the admin-mode
	// block can reference it.
	var downgradeCron *planchange.DowngradeRecheckCron
	// billingStripeClient is hoisted to func scope so the P11 lifecycle crons
	// (registered outside the admin-mode block) can reference it for the
	// hard-delete runner's Stripe customer deletion step.
	var billingStripeClient *billingstripe.Client

	// tenantDiscountSvc is hoisted for the same reason and one more: it is
	// used in THREE places at two different depths — the two
	// subscription-creation paths inside the admin-mode block (#660 T6) and
	// the platform-admin route wiring far below it — and constructing it
	// twice would give the fan-out and the creation hook different services.
	var tenantDiscountSvc *tenantdiscount.Service

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
	auditEmitter, err := audit.NewEmitter(audit.EmitterConfig{
		DB:     conn,
		Repo:   auditRepo,
		Logger: log,
	})
	if err != nil {
		log.Error("audit: new emitter", "err", err)
		os.Exit(1)
	}

	// Settings S5 — Notifications. Constructed early so both admin and
	// storefront modes can share a single service instance: storefront
	// checkout + review submission emit merchant notifications via this
	// service, while admin exposes the CRUD endpoints.
	// Merchant device push (mobile-admin): every in-app notification also
	// fans out to the store's admin devices. Publishing to Pub/Sub keeps the
	// notification write decoupled from push delivery. Disabled (in-app only)
	// when project/topic are unset. Typed-nil trap avoided by keeping a real
	// nil interface until a publisher is actually built.
	var pushPublisher notification.PushPublisher
	if cfg.GCPProjectID != "" && cfg.PushEventsTopic != "" {
		if pub, err := pushevents.NewPublisher(context.Background(), cfg.GCPProjectID, cfg.PushEventsTopic, log); err != nil {
			log.Error("push events publisher init failed; merchant push disabled", "error", err)
		} else {
			pushPublisher = pub
			defer pub.Close()
		}
	}

	notificationRepo := notification.NewRepository()
	notificationSvc := notification.NewService(notification.ServiceConfig{
		DB:     conn,
		Repo:   notificationRepo,
		Logger: log,
		Pusher: pushPublisher,
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

	// Wrap the transport so every outbound email is recorded (#348A). One
	// wrap, here, is what makes coverage complete: a mailer cannot opt out of
	// being logged without opting out of sending. Everything below — the
	// template client, ticket, giftcard, orderdoc, shipping-label and campaign
	// mailers — receives the wrapped sender without knowing it exists.
	//
	// A nil conn leaves the sender unwrapped rather than nil-panicking on the
	// first send: mail without a log beats no mail.
	if conn != nil {
		emailSender = emaillog.NewSender(emailSender, conn, log)
	}

	// billingEmailClient is the production email.Client. Before #381 the only
	// implementation was the no-op logger, wired at three sites below, so no
	// merchant had ever received a dunning notice, trial reminder,
	// payment-action reminder, win-back promo or trial-billed confirmation.
	// One instance shared by all three, so failover and attribution are
	// identical wherever billing mail originates.
	billingEmailClient := email.NewTemplateClient(templateLoader, emailSender, cfg.EmailFrom, log)

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
	).WithStoreIdentity(storeidentity.NewDBLoader(conn))
	// ticketRepo is hoisted so the platformadmin.Register call sites below
	// (mode.Both and mode.Admin) can both wire the same #329 cross-store
	// ticket read without constructing a second, redundant repository.
	ticketRepo := ticket.NewRepository()
	ticketSvc := ticket.NewService(ticket.ServiceConfig{
		DB:       conn,
		Repo:     ticketRepo,
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
		refundResolver := orderrefund.NewResolver(conn).WithSecretStore(carrierSecretStore).WithEncryptor(apiKeyEncryptor)
		refundCoordinator := orderrefund.NewCoordinator(conn, refundResolver, paymentSvc, orderSvc, orderRepo, refundGatewayEnabled).WithLogger(log)
		log.Info("refund coordinator wired (admin)", "gateway_enabled", refundGatewayEnabled)
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
			WithSecretStore(carrierSecretStore).
			// Registers the store's Stripe webhook on save so a merchant
			// never has to touch the Stripe dashboard. Disabled when the
			// public API base is unset, which leaves the manual flow.
			WithStripeWebhookProvisioner(stripewebhook.New(), cfg.PublicAPIBaseURL)
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
			WithLabelMailer(labelMailer).
			WithOrderService(orderSvc)

		// Shipment-cancel executor — resolves + executes the carrier action
		// when an order is fully refunded or cancelled. Reuses the shipments
		// handler's carrier-resolution path so credential decryption is not
		// duplicated. Fired best-effort from the refund coordinator (full
		// refunds), the orders Cancel handler (non-paid cancels), and the
		// manual per-shipment endpoint.
		// REVERSE_PICKUP_ENABLED gates the Phase 3 delivered-shipment reverse
		// pickup (creates a live courier dispatch; payload not yet live-verified).
		// Default off, mirroring REFUND_GATEWAY_ENABLED — delivered shipments
		// record `unsupported` until this is flipped on after verification.
		reversePickupEnabled := os.Getenv("REVERSE_PICKUP_ENABLED") == "true"
		shipmentCanceller := shipmentcancel.NewExecutor(shippingRepo, shipmentsHandler.CarrierForStore, log).
			WithReversePickup(reversePickupEnabled)
		log.Info("shipment-cancel executor wired", "reverse_pickup_enabled", reversePickupEnabled)
		shipmentsHandler = shipmentsHandler.WithCanceller(shipmentCanceller)

		// Production hook wraps the executor call in a detached goroutine so a
		// slow carrier never blocks the refund/cancel response; the executor is
		// already best-effort and never errors back.
		cancelShipmentsAsync := func(_ context.Context, orderID uuid.UUID) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shipmentCanceller.CancelForOrder(ctx, orderID)
			}()
		}
		refundCoordinator = refundCoordinator.WithShipmentCanceller(cancelShipmentsAsync)
		ordersHandler = ordersHandler.WithShipmentCanceller(cancelShipmentsAsync)

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

		// Outbound webhooks admin API (#562 task 7). Its own SubscriptionRepo/
		// DeliveryRepo/Sender instances, separate from the dispatcher and
		// delivery worker wired further below — both sets are stateless
		// wrappers over the same conn/ssrfguard, so nothing needs sharing
		// between the HTTP surface and the background loops.
		webhooksGuard := ssrfguard.New(nil)
		// The shared planResolver (B2) is constructed further down, after
		// subscriptionRepo. PlanResolver is stateless — just a *gorm.DB and
		// a subscription.Repository — so building a second one here costs
		// nothing and keeps the per-store webhook subscription cap (#586)
		// an explicit, non-nil constructor argument rather than a setter
		// that could be forgotten.
		webhooksPlanResolver := plangate.NewPlanResolver(conn, subscription.NewRepository())
		webhooksHandler := admin.NewWebhooksHandler(
			webhook.NewSubscriptionRepo(conn),
			webhook.NewDeliveryRepo(conn),
			webhooksGuard,
			webhook.NewSender(webhooksGuard, nil),
			webhooksPlanResolver,
			log,
		)

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
		campaignSlotAcquirer := concurrency.NewAdvisoryLockAcquirer(conn)
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
		userProfileRepo := userprofile.NewRepository(conn)
		accountHandler := admin.NewAccountHandler(userProfileRepo, cfg.AuthBFFURL, cfg.InternalAuthSecret, log)
		accountHandler.SetUploader(uploader)
		// First-seed of the merchant's display name (#790). Wired here,
		// unconditionally, rather than inside a verifier-specific closure:
		// /admin/account runs behind header-trust auth, so it is reached
		// on every deployment regardless of which bearer verifier the
		// mobile routes selected. Nil-safe by construction — an unset
		// AUTH_BFF_URL or internal secret just means blank names, exactly
		// as before this was wired at all.
		accountHandler.SetDisplayNames(
			displayname.NewAuthBFFLookup(cfg.AuthBFFURL, cfg.InternalAuthSecret, nil),
		)

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

			// #660 — the tenant-wide platform discount. Constructed HERE,
			// before the plan-change orchestrator and the trial subscriber
			// below, because both take it as their T6 creation hook; the
			// platform-admin routes far below reuse this same instance.
			//
			// A construction failure is logged and leaves the service nil,
			// which is a supported state everywhere it is used: the two
			// creation paths skip the hook and the two operator routes stay
			// unmounted. It cannot fail on the audit writer here — the
			// emitter is unconditional — so in practice this only fires if a
			// future dependency is added and not wired.
			if svc, tdErr := tenantdiscount.NewService(tenantdiscount.Config{
				DB:     conn,
				Stripe: &tenantdiscount.StripeAdapter{C: billingStripeClient},
				Audit:  auditEmitter,
				Logger: log,
			}); tdErr != nil {
				log.Warn("tenant discount service not available", "err", tdErr)
			} else {
				tenantDiscountSvc = svc
			}
		} else {
			log.Warn("STRIPE_BILLING_SECRET_KEY not set — subscription checkout/portal will fail")
		}
		// #304 / #392 — the plan-catalog parallel run.
		//
		// Reads the console's published catalog and compares it against the one
		// compiled into internal/billing/pricing, logging differences. It
		// DECIDES NOTHING: prices still come from the compiled catalog. The
		// cutover is a separate change, gated on this reporting durably zero.
		//
		// Runs on its own ticker rather than on any request path, so a slow or
		// unreachable console cannot touch a customer payment — BACKLOG §P's
		// invariant. Unconfigured, nothing is started at all.
		startCatalogParityRun(cfg, log)

		// tesserix-home#328 phase B — exercise the Cache's READ PATH.
		//
		// Sits beside the parity run because both belong on the pod that
		// reads the plan catalog, and does a deliberately different job: the
		// monitor above fetches directly to evidence reachability and data
		// agreement, this one resolves through the cache to exercise what
		// phase C will serve from (platformadmin/money.go, planchange,
		// reconciliation — all admin-side). It logs and discards; prices
		// still come from the compiled catalog. It carries its own
		// m.RunsAdmin() gate rather than relying on this block, so the mode
		// contract is stated and tested in one place.
		// See catalog_admin_resolve.go.
		startAdminCatalogResolve(m, cfg, log)

		// #726 step 2 — ingest the console's promo-code definitions.
		//
		// Unlike the plan catalog above, this one WRITES: a redemption's
		// promo_code_id is a foreign key into promo_codes, so the
		// definitions have to exist as rows. Boot plus a ticker, entirely on
		// its own goroutine — unconfigured or unreachable, the service
		// starts identically and promo_codes is left alone. It carries its
		// own m.RunsAdmin() gate for the same reason the resolve above does.
		// See promo_ingest.go.
		startPromoCatalogIngest(m, cfg, conn, log)

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
				TenantDiscount:   tenantDiscountApplier(tenantDiscountSvc),
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

		// Setup-progress realtime wiring (snapshot + SSE + WS) — shares
		// the dashboard's checklist computation.
		setupProgressHandler := admin.NewSetupProgressHandler(conn, log)

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
			if tenantDiscountSvc != nil {
				trialSubscriber = trialSubscriber.WithTenantDiscount(tenantDiscountSvc)
			}
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
		cancelSvc := cancel.NewService(conn, subscriptionRepo, auditEmitter, log).WithPromo(promoSvc)
		cancelHandler := cancel.NewHandler(cancelSvc, log)

		// P5 — Migration fast-path submit handler.
		migrationRepo = migration.NewRepository(conn)

		// #259 — the erasure inbox action. A construction failure is a wiring
		// bug and is fatal: degrading to a 501 here would present a broken
		// deployment as a deliberate "not implemented", which is exactly the
		// misreading this action was added to remove.
		customerEraser, err = newCustomerEraser(conn, log)
		if err != nil {
			log.Error("marketplace-api: customer erasure executor could not be built", "err", err)
			os.Exit(1)
		}
		// #706: the fast path's 90-day domain-age gate is NOT enforced. No WHOIS
		// or RDAP lookup exists, so UnenforcedDomainAge accepts every domain and
		// eligibility rests entirely on the CSM review at
		// POST /internal/csm/migration-fast-path/:id/review. Logged rather than
		// left implicit so an operator reading boot logs learns the gate is inert
		// instead of inferring it from a passing check.
		log.Warn("migration fast-path: domain-age check NOT enforced — every domain is accepted; eligibility rests on CSM review (#706)")

		// The recipient lookup is a closure over conn rather than a handle
		// passed into the handler — see migration.RecipientLookup. Both
		// halves are total: a missing subscription row yields "", which
		// ValidateRecipient classifies as no_address, and a missing store
		// projection yields "your store".
		migrationRecipient := func(ctx context.Context, storeID uuid.UUID) (string, string) {
			return subscription.BillingEmailFor(ctx, conn, storeID),
				subscription.StoreNameFor(ctx, conn, storeID)
		}
		migrationHandler = migration.NewHandler(migrationRepo, migration.UnenforcedDomainAge{}, log).
			WithAudit(auditEmitter).
			WithEmail(billingEmailClient, migrationRecipient,
				migrationSentCounter{metrics.BillingEmailsSentTotal},
				migrationSkipCounter{metrics.BillingEmailsSkippedTotal})

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

		// Tenant gate (#287) — refuses ALL admin traffic for a suspended
		// tenant, across every admin group (StoreMiddleware only covers
		// /admin/stores/:storeId). Degrades to a nil Gate — a no-op
		// middleware — when MARKETPLACE_PLATFORM_API_URL is unset,
		// matching how the other platform-api-backed features degrade.
		if cfg.PlatformAPIURL != "" {
			tenantGate = tenantgate.New(
				tenantdirectory.NewClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil),
				log, 5*time.Minute)
			log.Info("admin: tenant suspension gate enabled", "url", cfg.PlatformAPIURL)
		} else {
			log.Info("admin: tenant suspension gate disabled (MARKETPLACE_PLATFORM_API_URL is empty)")
		}

		// TenantGate is assigned nil, not a method value on a nil *Gate,
		// when the gate itself is unwired: a method value formed on a nil
		// receiver is a non-nil func (RequireActiveTenant does its own
		// g == nil check internally), which would make every `!= nil`
		// guard downstream (admin/routes.go, admin/mobile_routes.go) dead
		// code — this is the third instance of that pattern on this
		// branch (#287 review, F3). Keeping the explicit nil here makes
		// those guards real instead of decorative.
		var adminTenantGateHandler gin.HandlerFunc
		if tenantGate != nil {
			adminTenantGateHandler = tenantGate.RequireActiveTenant()
		}

		adminDeps = admin.Deps{
			TenantGate:               adminTenantGateHandler,
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
			WarehousesHandler:        admin.NewWarehousesHandler(conn, log),
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
			SetupProgressHandler:     setupProgressHandler,
			TicketsHandler:           ticketsHandler,
			BrandingHandler:          brandingHandler,
			PagesHandler:             pagesHandler,
			WebhooksHandler:          webhooksHandler,
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
	var pubsubPushHandler gin.HandlerFunc
	if m == mode.Admin || m == mode.Both {
		// Bearer verifier for mobile admin routes (#524 phase 4):
		// Zitadel when ZITADEL_ENABLED=true, otherwise the incumbent
		// GIP/Firebase verifier — selectMobileTokenVerifier never falls
		// back from one to the other, so a misconfigured or unreachable
		// Zitadel disables mobile admin routes rather than quietly
		// running them on GIP. cfg.ZitadelEnabled=true here always
		// carries a non-empty issuer + audience: config.Load's
		// Config.ValidateZitadel is checked unconditionally, so a
		// missing value already panicked main() at boot, before this
		// code could ever run misconfigured.
		tokenVerifier := selectMobileTokenVerifier(context.Background(), cfg, log,
			func(ctx context.Context, issuer, audience string) (auth.TokenVerifier, error) {
				return auth.NewZitadelVerifier(ctx, issuer, audience)
			},
			func() (auth.TokenVerifier, error) {
				if cfg.GIPProjectID == "" {
					return nil, nil
				}
				firebaseApp, err := firebase.NewApp(context.Background(), &firebase.Config{
					ProjectID: cfg.GIPProjectID,
				})
				if err != nil {
					return nil, fmt.Errorf("init firebase app: %w", err)
				}
				authClient, err := firebaseApp.Auth(context.Background())
				if err != nil {
					return nil, fmt.Errorf("init firebase auth client: %w", err)
				}
				return auth.NewGIPVerifier(authClient), nil
			},
		)
		pushRepo := push.NewRepository(conn)
		pushTokenHandler := admin.NewPushTokenHandler(pushRepo, log)
		pushSender := push.NewSender(&http.Client{Timeout: 10 * time.Second})
		// Public, OIDC-authenticated Pub/Sub push delivery. Mounted at
		// /pubsub/merchant-push (see route registration). Safe to expose:
		// it verifies the caller is our push subscription before any work.
		pubsubPushHandler = push.NewPubsubPushHandler(push.PubsubPushConfig{
			Sender:         pushSender,
			Repo:           pushRepo,
			Logger:         log,
			Audience:       cfg.PushOIDCAudience,
			ServiceAccount: cfg.PushOIDCServiceAccount,
		})

		// Team management + account deletion both proxy platform-api's
		// internal endpoints, so they share one teamproxy client — reusing
		// the SAME platform client config as the storefront store lookups
		// (already wired in prod), so no new chart env is needed. Nil when
		// the platform URL is unset — both handlers, and their routes,
		// then stay unmounted.
		var teamHandler *admin.TeamHandler
		var mobileAccountHandler *admin.MobileAccountHandler
		var myTenantsHandler *admin.MobileMyTenantsHandler
		var mobileLoginHandler *admin.MobileLoginHandler
		var mobileIDPHandler *admin.MobileIDPHandler
		if cfg.PlatformAPIURL != "" {
			teamClient := teamproxy.NewClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			teamHandler = admin.NewTeamHandler(teamClient, log)
			mobileAccountHandler = admin.NewMobileAccountHandler(teamClient, log)
			// Mobile tenant discovery (#686). Shares the platform client
			// above — no new chart env. Unset platform URL leaves it nil
			// and the route unmounted, which is the pre-existing shape for
			// every platform-backed mobile route.
			myTenantsHandler = admin.NewMobileMyTenantsHandler(teamClient, log)
			// Public mobile sign-in (#686). Needs auth-bff as well as the
			// platform client; both env vars already exist on this
			// deployment, so no chart change. Unset AUTH_BFF_URL leaves it
			// nil and the route unmounted.
			if cfg.AuthBFFURL != "" {
				authBFF := authbffclient.NewMobileLoginClient(cfg.AuthBFFURL, cfg.InternalAuthSecret, nil)
				mobileLoginHandler = admin.NewMobileLoginHandler(teamClient, authBFF, log)
				// "Continue with Google" (#686 item 1). Shares the same
				// auth-bff client and the same tenant lookup as password
				// login; the only extra input is the bridge-page return
				// URL, which is built from config here and never from
				// anything the device sends.
				mobileIDPHandler = admin.NewMobileIDPHandler(teamClient, authBFF, cfg.MobileIDPReturnURL, log)
			} else {
				log.Info("mobile login: AUTH_BFF_URL empty; mobile sign-in route disabled")
			}
		} else {
			log.Info("team: platform client not configured (MARKETPLACE_PLATFORM_API_URL empty); team + account-deletion routes disabled")
		}

		mobileDeps = admin.MobileDeps{
			Deps:             adminDeps,
			TokenVerifier:    tokenVerifier,
			PushTokenHandler: pushTokenHandler,
			// Merchant→platform support chat (#119) — bridges to otto's
			// platform tenant. Nil otto client => routes return 503.
			PlatformSupportHandler: admin.NewPlatformSupportHandler(ottoChatClient, cfg.OttoWSPublicBase, log),
			TeamHandler:            teamHandler,
			MobileAccountHandler:   mobileAccountHandler,
			// #524 phase 4 (blocking-fix round) — MUST be the exact same
			// flag that selected tokenVerifier above. RegisterAdminMobile
			// uses it to decide the single source of tenancy: FGA-validated
			// X-Acting-Tenant-Id when true, the GIP custom claim when
			// false. Passing anything other than cfg.ZitadelEnabled here
			// would let a claim-based and an FGA-based tenant write
			// compete (if both true) or would 404 every mobile-admin
			// request on GIP (if TenantFromRequest ran with no client-side
			// header support to feed it).
			ZitadelEnabled: cfg.ZitadelEnabled,
			// Dual-issuer relaxes the "exactly one tenancy writer" rule
			// above into an ORDERING rule instead: both writers run, and
			// the FGA-validated one runs second so it can only ever
			// overwrite an unvalidated claim, never the reverse. See
			// MobileDeps.DualIssuer.
			DualIssuer:       cfg.ZitadelEnabled && cfg.ZitadelDualIssuer,
			MyTenantsHandler: myTenantsHandler,
			LoginHandler:     mobileLoginHandler,
			IDPHandler:       mobileIDPHandler,
			// Resolves X-Acting-Tenant-Id via the same FGA client the rest
			// of the admin surface already checks permissions against, for
			// bearer tokens (Zitadel) that carry no tenant_id claim at
			// all. Only consulted when ZitadelEnabled is true.
			TenantMembershipChecker: fgaClient,
			TenantMembershipLogger:  log,
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
			WithNotifier(notificationSvc).
			// #230 — without this a storefront sale does not touch
			// inventory and oversells without limit.
			WithStockHolds(stockhold.NewRepository())

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
		giftCardGatewayResolver := giftcard.NewDBGatewayResolver(conn).WithSecretStore(carrierSecretStore).WithEncryptor(apiKeyEncryptor)
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
			// #230 — this is the handler routes.go actually mounts when
			// wired, so enforcement here is what stops the oversell.
			WithStockHolds(stockhold.NewRepository()).
			WithAudit(auditEmitter).
			WithSecretStore(carrierSecretStore).
			WithNotifier(notificationSvc)
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

		// Refund coordinator for the storefront's own self-cancel auto-refund
		// (spec §4). Built independently from the admin block's
		// refundCoordinator — that one is scoped inside the mode.Admin
		// block and out of reach here in a mode.Storefront-only process.
		refundGatewayEnabledSF := os.Getenv("REFUND_GATEWAY_ENABLED") == "true"
		paymentSvcSF := payment.NewService(payment.NewRepository(conn))
		refundResolverSF := orderrefund.NewResolver(conn).WithSecretStore(carrierSecretStore).WithEncryptor(apiKeyEncryptor)
		refundCoordinatorSF := orderrefund.NewCoordinator(conn, refundResolverSF, paymentSvcSF, orderSvcSF, orderRepoSF, refundGatewayEnabledSF).WithLogger(log)
		log.Info("refund coordinator wired (storefront)", "gateway_enabled", refundGatewayEnabledSF)

		orderDetailHandler := storefront.NewOrderDetailHandler(conn, orderRepoSF, orderSvcSF, orderDocSvcSF, log).
			WithReturns(returnSvcSF, returnRepoSF).
			WithNotifier(notificationSvc).
			WithRefunds(refundCoordinatorSF)

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
			CustomerSessionSecret:        cfg.CustomerSessionSecret,
			// C3 reviews.
			ReviewsHandler: sfReviewsHandler,
			// C4 wishlists.
			WishlistHandler: wishlistHandler,
			// #232 — server-side stock holds. Placed at cart-add for
			// HoldTTL, committed inside the order transaction at checkout.
			CartHoldsHandler: storefront.NewCartHoldsHandler(conn, stockhold.NewRepository(), log),
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
		if err := csvjob.RecoverOrphanedJobs(context.Background(), csvRepo, csvjob.OrphanWindow, log); err != nil {
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
			// Only log when the pending count or the error state changes —
			// a 5s tick would otherwise flood the log with identical lines.
			lastPending := -1
			var lastErr string
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
					jobs, err := csvRepo.FindQueuedJobs(workerCtx, 10)
					if err != nil {
						if workerCtx.Err() != nil {
							return
						}
						if err.Error() != lastErr {
							lastErr = err.Error()
							log.Error("csvjob: poll error", "err", err)
						}
						continue
					}
					lastErr = ""
					if len(jobs) != lastPending {
						lastPending = len(jobs)
						if lastPending > 0 {
							log.Info("csvjob: queued jobs awaiting dispatch", "count", lastPending)
						}
					}
					// Dispatch is driven by the submit handler once the CSV
					// upload writes to GCS; this loop reports the backlog of
					// jobs recovered from a crash so it is visible in logs.
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

	// Outbox publisher — runs in admin and both modes because admin owns
	// draining, not because the storefront produces nothing. It does: public
	// checkout in storefront mode writes outbox_events rows through
	// orderSvcSF (see the Orders M5 wiring above), and this replica drains
	// them. Running a second publisher there would duplicate the poll, not
	// find an empty table.
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

	// Outbound webhooks (#562). Two loops beside the outbox publisher, on the
	// same engines: a merchant subscription is admin-domain state, and the
	// storefront replica must not run a second copy of either poll.
	//
	// In-process rather than a separate Deployment, deliberately. The
	// alternative costs another chart, another ArgoCD Application and another
	// pod's memory on a cluster that has already had rollouts deadlock under
	// memory pressure. FOR UPDATE SKIP LOCKED makes both loops safe across
	// KEDA replicas. The bounded worker batch and the 5s per-request timeout
	// in internal/webhook cap what a slow merchant endpoint can tie up.
	//
	// Because dispatch and delivery are decoupled by webhook_deliveries,
	// moving the delivery loop to its own workload later is a deployment
	// change, not a redesign.
	webhookCtx, webhookCancel := context.WithCancel(context.Background())
	defer webhookCancel()
	var webhookDispatcherDone, webhookWorkerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		whSubs := webhook.NewSubscriptionRepo(conn)
		whDeliveries := webhook.NewDeliveryRepo(conn)
		whSender := webhook.NewSender(ssrfguard.New(nil), nil)

		dispatcher := webhook.NewDispatcher(conn, whSubs, whDeliveries, log, 100)
		webhookDispatcherDone = dispatcher.Start(webhookCtx, 5*time.Second)

		// notify is nil: merchant notification on auto-disable is NOT wired
		// yet. Design decision 3 describes emailing the merchant, and that
		// email is still outstanding — see the design doc, which records
		// what actually ships. What a merchant gets today is the disabled
		// subscription surfaced in admin settings with its
		// disabled_reason/disabled_at, plus a server-side warning log; a
		// merchant who is not looking at admin learns nothing until they do.
		// Known gap, deliberately not papered over here.
		worker := webhook.NewWorker(whDeliveries, whSubs, whSender, log, 4, nil)
		webhookWorkerDone = worker.Start(webhookCtx, 5*time.Second)

		log.Info("webhook dispatcher and delivery worker started")
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

		// Email adapter — the real template client (render → SendGrid → Resend),
		// shared with the dunning, trial reminder and win-back crons so failover
		// and attribution are identical wherever billing mail originates.
		// WithDB hands the dispatcher a NON-transactional handle: the
		// trial-billed confirmation claims a billing_email_sends row before
		// sending, and that claim has to survive a rollback of the webhook
		// transaction it is sent from (see dispatch.sendTrialBilled).
		dispatcherEmailClient := billingEmailClient
		dispatcher.WithEmail(dispatcherEmailClient).
			WithDB(conn).
			WithSkipCounter(dispatchSkipCounter{metrics.BillingEmailsSkippedTotal}).
			WithSentCounter(dispatchSentCounter{metrics.BillingEmailsSentTotal})

		// P7 §19.2: annotate B2B invoices with the reverse-charge clause on
		// invoice.finalized for validated tax IDs in reverse-charge jurisdictions.
		dispatcher.WithReverseChargeAnnotator(billingStripeClient)

		// #704: radar.early_fraud_warning carries a charge id and no customer,
		// so attributing a Radar fraud warning to a store needs a live Stripe
		// charge lookup. A nil client (no billing key) leaves the getter unset
		// and every warning is recorded as unattributed rather than dropped.
		dispatcher.WithChargeGetter(billingStripeClient)

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

	// billingEmailClient, not dunningEmailClient: the latter is declared
	// further down, after this cron is built. Same underlying template
	// client either way.
	expiryCron := trial.NewExpiryCron(conn, auditEmitter, log, nil).
		WithEmail(billingEmailClient,
			trialSentCounter{metrics.BillingEmailsSentTotal},
			trialSkipCounter{metrics.BillingEmailsSkippedTotal})
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
	// rows are pruned only against their OWN plan. Additionally, the same
	// pass also prunes actor_type='operator' rows at seven years via a
	// join-less path, because those rows carry no store_id and are
	// unreachable by the plan-based DELETE (#365).
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

	// webhook_events retention — daily prune at 03:30 UTC (#440). Two
	// windows, in the table's own vocabulary: status='processed' rows go at
	// 30 days, everything else (in practice 'received' — the unprocessed,
	// stuck case someone may still want to inspect or replay) at 90 days.
	// This table has no tenant_id, store_id or customer link, so neither
	// GDPR erasure nor tenant purge can reach the raw provider payloads it
	// stores; age is the only axis available. 03:30 is clear of every other
	// daily cron in the service.
	webhookPruneCron := webhookprune.NewPruneCron(conn, log, nil, 0).
		WithCounter(func(label string, n int64) {
			metrics.WebhookPruneRowsDeletedTotal.WithLabelValues(label).Add(float64(n))
		})
	if _, err := trialScheduler.AddFunc(webhookprune.PruneSpec, func() {
		if _, err := webhookPruneCron.Run(workerCtx); err != nil {
			log.Error("webhook prune cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register webhook prune cron", "err", err)
	}

	// Platform admin nonce sweep — daily at 09:45 UTC, deletes
	// platform_request_nonces rows past expires_at. Only registered when the
	// platform admin secret is configured: an unconfigured deploy is
	// refusing all /admin/* traffic (see platformadmin.Register), so there
	// is nothing accumulating in the table yet to sweep.
	if cfg.PlatformAdminSecret != "" {
		if _, err := trialScheduler.AddFunc(platformadmin.SweepSpec, func() {
			deleted, err := platformadmin.SweepExpiredNonces(workerCtx, conn)
			if err != nil {
				log.Error("platform admin nonce sweep failed", "err", err)
				return
			}
			log.Info("platform admin nonce sweep complete", "rows_deleted", deleted)
		}); err != nil {
			log.Error("register platform admin nonce sweep cron", "err", err)
		}

		if _, err := trialScheduler.AddFunc(platformadmin.SweepSpec, func() {
			deleted, err := platformadmin.SweepExpiredIdempotencyKeys(workerCtx, conn)
			if err != nil {
				log.Error("platform admin idempotency sweep failed", "err", err)
				return
			}
			log.Info("platform admin idempotency sweep complete", "rows_deleted", deleted)
		}); err != nil {
			log.Error("register platform admin idempotency sweep cron", "err", err)
		}
	}

	trialScheduler.Start()
	defer trialScheduler.Stop()
	log.Info("P5 crons started", "count", 5)

	// P6 dunning + SCA recovery crons. Emails route through the real
	// template client as of #381 — recipients come from
	// store_subscriptions.email, and an unknown or placeholder address is
	// counted as skipped rather than reported as delivered.
	dunningEmailClient := billingEmailClient

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
		nil).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))
	if _, err := trialScheduler.AddFunc(dunning.DunningEmailsSpec, func() {
		if err := dunningEmailsCron.Run(workerCtx); err != nil {
			log.Error("dunning emails cron failed", "err", err)
		}
	}); err != nil {
		log.Error("register dunning emails cron", "err", err)
	}

	scaRemindersCron := dunning.NewSendPaymentActionReminders(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.PaymentActionRemindersSentTotal),
		nil).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))

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
		// Register (not a bare AddFunc) so the pass runs under revalidation's
		// own 30-minute timeout. Without it the cron inherits workerCtx, which
		// has no deadline — so a stalled pass would hang forever and every
		// later fire would queue behind it, leaking a goroutine and a
		// connection per day (#396).
		if _, err := revalidation.Register(trialScheduler, *revalidationCron); err != nil {
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
		nil).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))
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

	winBackEmailClient := billingEmailClient

	// The win-back only ever VALIDATES a promo code — it never redeems one —
	// so this service is constructed with a nil Stripe client on purpose.
	// promo.Service.ValidateCode makes no Stripe call, and handing the cron a
	// client it must not use would be an invitation to start using it.
	winBackPromo := promo.NewService(conn, promo.NewRepository(), nil, log)

	winBackCron := lifecycle.NewWinBackCron(conn, winBackEmailClient, log, nil).
		WithSkipCounter(lifecycleSkipCounter{metrics.BillingEmailsSkippedTotal}).
		WithSentCounter(lifecycleSentCounter{metrics.BillingEmailsSentTotal}).
		WithPromo(winBackPromo)
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

	// trialStripe stays a TRUE nil interface when Stripe is not configured.
	// Assigning &trialStripeAdapter{c: nil} unconditionally would make
	// Extender.Stripe != nil TRUE and panic on the first card-backed
	// extension, after the row lock is taken — the same shape as #288's
	// typed-nil gipDeleter. The nil interface is a supported configuration:
	// card-backed trials get ErrStripeManaged, exactly as before #358.
	var trialStripe trial.StripeTrialUpdater
	if billingStripeClient != nil {
		trialStripe = &trialStripeAdapter{c: billingStripeClient}
	} else {
		log.Warn("STRIPE_BILLING_SECRET_KEY not set — card-backed trials cannot be extended (409 stripe_managed)")
	}

	// tenantDiscounter stays a TRUE nil interface unless the service was
	// really constructed, for the same reason trialStripe above does: a
	// typed nil assigned into platformadmin.Deps.TenantDiscount is a
	// non-nil interface value and would defeat Register's mount guard.
	//
	// The service itself is built up in the admin-mode block, beside the
	// Stripe client and the two subscription-creation paths that share it
	// (#660 T6). It is nil here when Stripe billing is unconfigured, when
	// MODE=storefront so that block never ran, or when construction failed —
	// and unlike trials there is no degraded mode to fall back to. Every
	// operation this service performs IS a Stripe operation
	// (tenantdiscount.ErrNoStripeClient), and it refuses construction
	// without an audit writer as well (ErrNoAuditWriter), so a missing
	// dependency leaves the two discount routes unmounted rather than
	// mounted and answering an error.
	var tenantDiscounter platformadmin.TenantDiscounter
	if tenantDiscountSvc != nil {
		tenantDiscounter = tenantDiscountSvc
	} else {
		log.Warn("tenant discount service not available — tenant discounts cannot be applied or removed (routes unmounted)")
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
		var tenantDirectoryClient platformadmin.TenantDirectory
		var onboardingFunnelClient platformadmin.OnboardingFunnel
		var estateCountsClient platformadmin.EstateCounts
		var tenantLifecycleClient platformadmin.TenantLifecycle
		var tenantTeardownClient platformadmin.TenantTeardown
		var estateUsersClient platformadmin.EstateUserDirectory
		if cfg.PlatformAPIURL != "" {
			tenantDirectoryClient = tenantdirectory.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			estateUsersClient = estateuserdir.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			onboardingFunnelClient = onboardingfunnel.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			estateCountsClient = estatecounts.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			// One construction, assigned to both interfaces: the
			// TenantLifecycle interface Register uses for suspend/unsuspend
			// doesn't include Teardown, so purge needs the concrete client
			// too. Assigning only inside this block (rather than declaring
			// a *tenantlifecycle.Client at outer scope and handing it to
			// both fields unconditionally) keeps tenantTeardownClient a
			// TRUE nil interface when PlatformAPIURL is unset — a nil
			// *tenantlifecycle.Client assigned into an interface field is a
			// non-nil interface value, which would defeat Register's
			// TenantTeardown != nil mount guard (#323).
			lifecycleClient := tenantlifecycle.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
			tenantLifecycleClient = lifecycleClient
			tenantTeardownClient = lifecycleClient
		}
		platformSubscriptionRepo := subscription.NewRepository()
		platformadmin.Register(r.Group(platformadmin.MountPrefix), platformadmin.Deps{
			DB:                      conn,
			Repo:                    auditRepo,
			Logger:                  log,
			Secret:                  cfg.PlatformAdminSecret,
			TenantDirectory:         tenantDirectoryClient,
			OnboardingFunnel:        onboardingFunnelClient,
			EstateCounts:            estateCountsClient,
			Subscriptions:           platformadmin.SubscriptionsFunc(trial.CountExpiring),
			Trials:                  platformadmin.TrialListerFunc(trial.ListExpiring),
			AllSubscriptions:        platformadmin.SubscriptionListerFunc(platformSubscriptionRepo.ListAllSubscriptions),
			TenantLifecycle:         tenantLifecycleClient,
			Emitter:                 auditEmitter,
			TenantGateInvalidator:   tenantGateInvalidator(tenantGate),
			Tickets:                 ticketRepo,
			Notifications:           notificationRepo,
			Outbox:                  platformadmin.OutboxListerFunc(outbox.ListPlatform),
			OutboxWriter:            outbox.WriterFuncs{},
			TrialExtender:           trial.NewExtender(trialStripe),
			TenantDiscount:          tenantDiscounter,
			TenantTeardown:          tenantTeardownClient,
			Purger:                  tenantpurge.NewGormPurger(conn),
			Inbox:                   inboxDep(newInboxAggregator(conn, onboardingFunnelClient, 0)),
			InboxItems:              inboxItemSource(newInboxAggregator(conn, onboardingFunnelClient, 0)),
			InboxActionExecutors:    inboxActionExecutors(migrationRepo, customerEraser),
			EstateUsers:             estateUsersClient,
			EmailSends:              platformadmin.EmailSendListerFunc(emaillog.ListPlatform),
			BreakGlass:              platformadmin.BreakGlassListerFunc(breakglass.ListPlatform),
			EmailTemplates:          templateStore,
			EmailTemplateRegistry:   templateLoader,
			EmailTemplateTestSender: templateTestSender,
			PriceCatalog:            newServingCatalogResolver(cfg, log),
		})
		storefront.RegisterStorefront(r.Group("/api/v1"), storefrontDeps)
		storefront.RegisterMobileStorefrontSupport(r.Group("/api/v1"), storefrontSupportHandler, storefrontDeps.SlugCache, storefrontCustomerVerifier)
		public.RegisterPublic(r.Group("/api/v1"), public.PublicDeps{
			DelhiveryWebhookHandler:   delhiveryWebhookHandler,
			JournalSubscribeHandler:   journalSubscribeHandler,
			JournalUnsubscribeHandler: journalUnsubscribeHandler,
		})
		if brandingSeeder != nil {
			brandingSeeder.Register(r.Group("/api/v1/test"))
		}
		if pubsubPushHandler != nil {
			// Public path (OIDC-verified in-handler) — Pub/Sub push delivery.
			r.POST("/pubsub/merchant-push", pubsubPushHandler)
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
		// CSM fast-path review action — POST /internal/csm/migration-fast-path/
		// :id/review. Was implemented and tested but never mounted (#281);
		// mounted here and on the mode.Admin engine below via the same
		// RegisterInternalRoutes method so the two engines can't drift (#323).
		if migrationHandler != nil {
			migrationHandler.RegisterInternalRoutes(r.Group("/internal"), cfg.InternalAuthSecret)
		}
		// Provider delivery events for outbound mail (#348B). NOT under
		// /internal: the caller is Resend and holds no internal secret. It
		// authenticates with the signature in svix-signature, verified over
		// the RAW body. Mounted unconditionally — with no secret configured
		// it answers 503 not_configured, which is diagnosable, rather than
		// 404, which reads as a wrong URL (the failure #280 shipped).
		if conn != nil {
			emailevents.NewHandler(
				emailevents.NewApplier(conn, log), cfg.ResendWebhookSecret, log,
			).Register(r.Group(""))
		}
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
		// OpenPanel CORS reconciler reads this — no wildcard covers a
		// merchant's own domain, so the list has to be enumerated.
		internalsvc.NewActiveDomainsHandler(conn).
			Register(r.Group("/internal"), cfg.AuditIngestSecret)
		// Tenant hard-delete — platform-api's outbox drainer POSTs here to
		// run the destructive purge of a tenant's marketplace-api domain
		// data (see internal/tenantpurge). Purge is idempotent, so replay
		// on drainer retry is safe.
		// Gated by InternalAuthSecret (MARKETPLACE_INTERNAL_AUTH_SECRET) —
		// must match what VendorClient.PurgeTenant signs with, NOT
		// AuditIngestSecret (different caller, different secret).
		internalsvc.NewTenantPurgeHandler(func(ctx context.Context, tenantID string, storeIDs []string) error {
			_, err := tenantpurge.Purge(ctx, conn, tenantID, storeIDs)
			return err
		}).Register(r.Group("/internal"), cfg.InternalAuthSecret)
		if stripeBillingWebhookHandler != nil {
			r.POST("/webhooks/stripe-billing", stripeBillingWebhookHandler)
		}
		srv = newHTTPServer(cfg.HTTPPort, r)
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
			var tenantDirectoryClient platformadmin.TenantDirectory
			var onboardingFunnelClient platformadmin.OnboardingFunnel
			var estateCountsClient platformadmin.EstateCounts
			var tenantLifecycleClient platformadmin.TenantLifecycle
			var tenantTeardownClient platformadmin.TenantTeardown
			var estateUsersClient platformadmin.EstateUserDirectory
			if cfg.PlatformAPIURL != "" {
				tenantDirectoryClient = tenantdirectory.NewClient(
					cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
				estateUsersClient = estateuserdir.NewClient(
					cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
				onboardingFunnelClient = onboardingfunnel.NewClient(
					cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
				estateCountsClient = estatecounts.NewClient(
					cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
				// One construction, assigned to both interfaces — see the
				// matching comment on the mode.Both branch above for why a
				// nil *tenantlifecycle.Client must never reach
				// tenantTeardownClient unconditionally (#323).
				lifecycleClient := tenantlifecycle.NewClient(
					cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
				tenantLifecycleClient = lifecycleClient
				tenantTeardownClient = lifecycleClient
			}
			platformSubscriptionRepo := subscription.NewRepository()
			platformadmin.Register(engine.Group(platformadmin.MountPrefix), platformadmin.Deps{
				DB:                      conn,
				Repo:                    auditRepo,
				Logger:                  log,
				Secret:                  cfg.PlatformAdminSecret,
				TenantDirectory:         tenantDirectoryClient,
				OnboardingFunnel:        onboardingFunnelClient,
				EstateCounts:            estateCountsClient,
				Subscriptions:           platformadmin.SubscriptionsFunc(trial.CountExpiring),
				Trials:                  platformadmin.TrialListerFunc(trial.ListExpiring),
				AllSubscriptions:        platformadmin.SubscriptionListerFunc(platformSubscriptionRepo.ListAllSubscriptions),
				TenantLifecycle:         tenantLifecycleClient,
				Emitter:                 auditEmitter,
				TenantGateInvalidator:   tenantGateInvalidator(tenantGate),
				Tickets:                 ticketRepo,
				Notifications:           notificationRepo,
				Outbox:                  platformadmin.OutboxListerFunc(outbox.ListPlatform),
				OutboxWriter:            outbox.WriterFuncs{},
				TrialExtender:           trial.NewExtender(trialStripe),
				TenantDiscount:          tenantDiscounter,
				TenantTeardown:          tenantTeardownClient,
				Purger:                  tenantpurge.NewGormPurger(conn),
				Inbox:                   inboxDep(newInboxAggregator(conn, onboardingFunnelClient, 0)),
				InboxItems:              inboxItemSource(newInboxAggregator(conn, onboardingFunnelClient, 0)),
				InboxActionExecutors:    inboxActionExecutors(migrationRepo, customerEraser),
				EstateUsers:             estateUsersClient,
				EmailSends:              platformadmin.EmailSendListerFunc(emaillog.ListPlatform),
				BreakGlass:              platformadmin.BreakGlassListerFunc(breakglass.ListPlatform),
				EmailTemplates:          templateStore,
				EmailTemplateRegistry:   templateLoader,
				EmailTemplateTestSender: templateTestSender,
				PriceCatalog:            newServingCatalogResolver(cfg, log),
			})
			// Public Delhivery webhook receiver. Mounted on the admin
			// engine because the merchant-configured URL points at the
			// admin hostname (playwrite-test-admin.mark8ly.com) and the
			// handler shares shipments/secrets wiring with the admin
			// ShipmentsHandler.
			public.RegisterPublic(engine.Group("/api/v1"), public.PublicDeps{
				DelhiveryWebhookHandler:   delhiveryWebhookHandler,
				JournalSubscribeHandler:   journalSubscribeHandler,
				JournalUnsubscribeHandler: journalUnsubscribeHandler,
			})
			if countryPublicHandler != nil {
				engine.GET("/api/v1/public/supported-countries", countryPublicHandler.ListSupported)
			}
			if pubsubPushHandler != nil {
				// Public path (OIDC-verified in-handler) — Pub/Sub push delivery.
				engine.POST("/pubsub/merchant-push", pubsubPushHandler)
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
			// CSM fast-path review action — mirrors the mode.Both mount
			// above via the same RegisterInternalRoutes method, so
			// MODE=admin (production) and local mode.Both dev never drift
			// on this route (#281, #323).
			if migrationHandler != nil {
				migrationHandler.RegisterInternalRoutes(engine.Group("/internal"), cfg.InternalAuthSecret)
			}
			// Provider delivery events (#348B) — mirrors the mount above so
			// the two engines cannot drift (#323).
			if conn != nil {
				emailevents.NewHandler(
					emailevents.NewApplier(conn, log), cfg.ResendWebhookSecret, log,
				).Register(engine.Group(""))
			}
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
			// OpenPanel CORS reconciler reads this — admin-only, matching
			// the other cross-tenant enumerations above.
			internalsvc.NewActiveDomainsHandler(conn).
				Register(engine.Group("/internal"), cfg.AuditIngestSecret)
			// Tenant hard-delete — platform-api's outbox drainer POSTs here
			// to run the destructive purge of a tenant's marketplace-api
			// domain data (see internal/tenantpurge). Admin-only, matching
			// the audit-ingest placement above. Purge is idempotent, so
			// replay on drainer retry is safe.
			// Gated by InternalAuthSecret (MARKETPLACE_INTERNAL_AUTH_SECRET) —
			// must match what VendorClient.PurgeTenant signs with, NOT
			// AuditIngestSecret (different caller, different secret).
			internalsvc.NewTenantPurgeHandler(func(ctx context.Context, tenantID string, storeIDs []string) error {
				_, err := tenantpurge.Purge(ctx, conn, tenantID, storeIDs)
				return err
			}).Register(engine.Group("/internal"), cfg.InternalAuthSecret)
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
		srv = newHTTPServer(cfg.HTTPPort, engine)
	}

	// Start the server in a goroutine so we can signal-handle on the main.
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Prometheus scrape endpoint, on its own listener so the registry is not
	// reachable through the public ingress. A failure here must never take
	// the API down — an unscrapable service is a monitoring outage, not a
	// customer-facing one — so this logs and gives up rather than exiting.
	var metricsSrv *http.Server
	if cfg.MetricsPort > 0 && cfg.MetricsPort != cfg.HTTPPort {
		metricsSrv = newMetricsServer(cfg.MetricsPort)
		go func() {
			log.Info("metrics listening", slog.String("addr", metricsSrv.Addr))
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("metrics listen", "err", err)
			}
		}()
	} else {
		log.Warn("metrics endpoint disabled",
			slog.Int("metrics_port", cfg.MetricsPort),
			slog.Int("http_port", cfg.HTTPPort))
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			log.Warn("metrics shutdown", "err", err)
		}
	}
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
	webhookCancel()
	if webhookDispatcherDone != nil {
		select {
		case <-webhookDispatcherDone:
			log.Info("webhook dispatcher stopped")
		case <-time.After(5 * time.Second):
			log.Warn("webhook dispatcher did not stop in time")
		}
	}
	if webhookWorkerDone != nil {
		select {
		case <-webhookWorkerDone:
			log.Info("webhook delivery worker stopped")
		case <-time.After(5 * time.Second):
			log.Warn("webhook delivery worker did not stop in time")
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

// IncArbitrageTenantMismatch records a refused audit write where the caller's
// tenant does not own the subscription (#423). Distinct reason label so P17
// can alert on it independently — it is a bug or a probe, never routine.
func (c *arbitragePrometheusCounter) IncArbitrageTenantMismatch() {
	if metrics.Subscription != nil {
		metrics.Subscription.SubscriptionArbitrageFlaggedTotal.
			WithLabelValues("tenant_mismatch").Inc()
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

// tenantDiscountApplier converts a possibly-nil *tenantdiscount.Service into a
// planchange.TenantDiscountApplier that is TRULY nil when no service was
// constructed.
//
// Assigning the typed nil straight into the struct field would make
// Deps.TenantDiscount != nil true, and every initial subscription would call a
// method on a nil receiver. That is survivable — ApplyToNewSubscription
// returns ErrNilService and the hook swallows it — but it would log an error
// on every subscription created without Stripe billing configured, which is
// noise that reads like a fault.
func tenantDiscountApplier(s *tenantdiscount.Service) planchange.TenantDiscountApplier {
	if s == nil {
		return nil
	}
	return s
}
