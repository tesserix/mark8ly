// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for marketplace-api.
//
// MODE selects which Gin engine(s) the binary constructs — see
// internal/mode. Default is "both" for local dev. Knative Services in
// the infra repo set this explicitly per service.
type Config struct {
	Env         string `envconfig:"ENV" default:"dev"`
	Mode        string `envconfig:"MODE" default:"both"`
	HTTPPort    int    `envconfig:"HTTP_PORT" default:"8087"`
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	FGAAPIURL   string `envconfig:"MARKETPLACE_FGA_API_URL" required:"true"`
	// InternalAuthSecret guards the internal-trust header auth used by the
	// admin route group. Empty disables the shared-secret check — fine for
	// local dev and test; production wiring sets this via ExternalSecret.
	InternalAuthSecret string `envconfig:"MARKETPLACE_INTERNAL_AUTH_SECRET" default:""`
	// GCSBucket, when non-empty, switches the media uploader from the
	// dev FakeUploader to a real GCS-backed implementation. Empty keeps
	// `make dev` working without GCS credentials.
	GCSBucket string `envconfig:"MARKETPLACE_GCS_BUCKET" default:""`
	// GCSSignerSAEmail is the service-account email used to sign V4 GCS
	// upload URLs via the IAM Credentials API. Required on GKE Workload
	// Identity, where Application Default Credentials do not include a
	// private key. When empty, the storage client falls back to ADC and
	// signing will fail with "signedURL: cannot sign…" if no key is
	// embedded in credentials. Set to the SA the workload runs as.
	GCSSignerSAEmail string `envconfig:"MARKETPLACE_GCS_SIGNER_SA_EMAIL" default:""`
	// MediaPublicBaseURL is the externally-reachable URL prefix that the
	// service prepends to a storage_key when persisting product_media.url.
	// Examples:
	//   - https://cdn.mark8ly.com                                        (Cloudflare-fronted)
	//   - https://storage.googleapis.com/tesseracthub-480811-mark8ly-media (direct GCS)
	// When empty, the URL persisted on product_media is left as the raw
	// storage_key (legacy behavior).
	MediaPublicBaseURL string `envconfig:"MARKETPLACE_MEDIA_PUBLIC_BASE_URL" default:""`
	// MediaCacheControl is the Cache-Control header stamped on uploaded
	// product-media objects after finalize. The default is safe because
	// keys are content-addressed (hash + filename) and therefore
	// effectively immutable.
	MediaCacheControl string `envconfig:"MARKETPLACE_MEDIA_CACHE_CONTROL" default:"public, max-age=31536000, immutable"`
	// PlatformAPIURL, when non-empty, switches StoreMiddleware from the
	// dev stub client to a real HTTP client against platform-api's
	// /internal/stores endpoint. Empty keeps the stub wired.
	PlatformAPIURL string `envconfig:"MARKETPLACE_PLATFORM_API_URL" default:""`
	// PlatformAPISecret is sent as X-Internal-Auth on calls to
	// platform-api. Empty disables the header (fine when Istio network
	// policy is the only gate).
	PlatformAPISecret string `envconfig:"MARKETPLACE_PLATFORM_API_SECRET" default:""`
	// StorefrontKey, when non-empty, gates the storefront route group with
	// a shared-secret X-Storefront-Key header. Empty disables the check —
	// fine for local dev and tests; production wiring sets this via
	// ExternalSecret.
	StorefrontKey string `envconfig:"MARKETPLACE_STOREFRONT_KEY" default:""`
	// CustomerSessionSecret is the HMAC key used to validate auth-bff
	// session cookies. When empty, OptionalCustomerAuth always yields
	// guest context — fine for local dev without auth-bff.
	CustomerSessionSecret string `envconfig:"CUSTOMER_SESSION_SECRET" default:""`
	// GIPProjectID is the Google Identity Platform project ID used to verify
	// mobile Bearer tokens. When empty, GIPBearerAuth rejects all requests —
	// fine for dev environments that don't use mobile auth.
	GIPProjectID string `envconfig:"GIP_PROJECT_ID" default:""`
	// GIPMerchantTenantID is the Google Identity Platform *tenant* the
	// merchant-staff pool lives in (e.g. "MP-Internal-e986p"). Note this
	// is a GIP tenant id and has nothing to do with a mark8ly tenant id.
	//
	// Used to look an account record up when seeding a new
	// user_profiles row, so a merchant who signed up with Google gets
	// the name GIP already holds instead of a blank field. A
	// project-level lookup cannot see tenant users, so when this is
	// empty the seed simply leaves the name blank — the pre-existing
	// behaviour.
	GIPMerchantTenantID string `envconfig:"GIP_MERCHANT_TENANT_ID" default:""`
	// GIPWebAPIKeyResource is the full Google API Keys v2 resource name
	// of the browser API key used by storefront sign-in (e.g.
	// "projects/849928263410/locations/global/keys/2457e3a0-..."). When
	// set, the custom-domain service patches this key's HTTP-referrer
	// allowlist on verify and Remove, so a new merchant domain is
	// self-served end-to-end. Empty disables the integration — gipkey
	// falls back to a Noop client and no GCP API call is made.
	GIPWebAPIKeyResource string `envconfig:"GIP_WEB_API_KEY_RESOURCE_NAME" default:""`

	// S1 — auth-bff URL for MFA/session proxying.
	AuthBFFURL string `envconfig:"AUTH_BFF_URL" default:""`
	// S3 — Stripe Billing keys + webhook / orphan cron config.
	StripeBillingSecretKey     string `envconfig:"STRIPE_BILLING_SECRET_KEY" default:""`
	StripeBillingWebhookSecret string `envconfig:"STRIPE_BILLING_WEBHOOK_SECRET" default:""`

	// Console plan-catalog read (#304). The console is becoming the single
	// place a price is maintained; these configure the parallel run that
	// compares its catalog against the one compiled into
	// internal/billing/pricing, ahead of the cutover.
	//
	// ALL OF THESE ARE OPTIONAL BY DESIGN. Unset, no console read is
	// attempted and prices come from the compiled catalog exactly as they do
	// today — BACKLOG §P requires that nothing on a customer payment path
	// depend on the console being reachable, and an unconfigured deploy is
	// the strongest form of that guarantee.
	ConsoleCatalogURL          string `envconfig:"CONSOLE_CATALOG_URL" default:""`
	ConsoleCatalogTokenURL     string `envconfig:"CONSOLE_CATALOG_TOKEN_URL" default:""`
	ConsoleCatalogClientID     string `envconfig:"CONSOLE_CATALOG_CLIENT_ID" default:""`
	ConsoleCatalogClientSecret string `envconfig:"CONSOLE_CATALOG_CLIENT_SECRET" default:""`
	// Scope must carry BOTH the project-audience scope and the roles scope.
	// The first puts the project in the token's `aud`, which is what proves
	// the token was minted for this route; the second makes it carry the
	// roles claim, without which it verifies but holds no capability. Kept in
	// config rather than compiled in so the project id is not hardcoded here.
	ConsoleCatalogScope    string        `envconfig:"CONSOLE_CATALOG_SCOPE" default:""`
	ConsoleCatalogMode     string        `envconfig:"CONSOLE_CATALOG_MODE" default:"test"`
	ConsoleCatalogInterval time.Duration `envconfig:"CONSOLE_CATALOG_INTERVAL" default:"15m"`

	// RESEND_WEBHOOK_SECRET verifies inbound provider delivery events
	// (#348B). Empty leaves the endpoint mounted but inert: it answers 503
	// not_configured rather than 404, so a missing secret is diagnosable
	// instead of looking like a wrong URL.
	ResendWebhookSecret     string        `envconfig:"RESEND_WEBHOOK_SECRET" default:""`
	StripeAllowedEventTypes []string      `envconfig:"STRIPE_ALLOWED_EVENT_TYPES" default:"checkout.session.completed,customer.subscription.updated,customer.subscription.deleted,invoice.paid,invoice.payment_failed,invoice.payment_action_required,customer.updated,charge.refunded,payment_method.attached,payment_method.detached,radar.early_fraud_warning"`
	WebhookMaxBodyBytes     int64         `envconfig:"WEBHOOK_MAX_BODY_BYTES" default:"524288"`
	OrphanRetryMaxCount     int           `envconfig:"ORPHAN_RETRY_MAX_COUNT" default:"6"`
	OrphanRetryInterval     time.Duration `envconfig:"ORPHAN_RETRY_INTERVAL" default:"5m"`
	OrphanStaleThreshold    time.Duration `envconfig:"ORPHAN_STALE_THRESHOLD" default:"1h"`
	PagerDutyWebhookURL     string        `envconfig:"PAGERDUTY_WEBHOOK_URL" default:""`

	// AUDIT — shared secret gating /internal/audit-events. When empty,
	// the endpoint is permissive (dev convenience). Set to the SAME
	// non-empty value on auth-bff and platform-api to enforce
	// service-to-service auth on the audit ingest path. Deliberately
	// separate from InternalAuthSecret (which gates HeaderTrustAuth
	// across the entire admin surface and would require admin BFF
	// changes to enable safely).
	AuditIngestSecret string `envconfig:"AUDIT_INGEST_SECRET" default:""`

	// PlatformAdminSecret is the HMAC key for the Tesserix platform console's
	// signed /admin/* calls (#275). Separate from InternalAuthSecret and
	// AuditIngestSecret: different caller, different blast radius.
	//
	// Unlike those, an empty value does NOT no-op the check — the platform
	// admin surface fails closed and answers 503 until this is populated.
	PlatformAdminSecret string `envconfig:"MARKETPLACE_PLATFORM_ADMIN_SECRET" default:""`

	// P0 — CORS allowed origins (comma-separated, storefront engine only).
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:"https://*.mark8ly.com"`
	// P0 — Encryption mode: "aes" for AES-256-GCM, "noop" for base64 dev stub.
	EncryptionMode string `envconfig:"ENCRYPTION_MODE" default:"noop"`
	// P0 — DEK for API key encryption (32-byte hex or base64). Required when
	// EncryptionMode=aes. In production, loaded from GCP Secret Manager.
	EncryptionKey string `envconfig:"ENCRYPTION_KEY" default:""`
	// P0 — Sentry DSN for error tracking. Empty disables Sentry.
	SentryDSN string `envconfig:"SENTRY_DSN" default:""`
	// P1 — Prometheus metrics port. 0 disables the metrics server.
	MetricsPort int `envconfig:"METRICS_PORT" default:"9090"`

	// Marketing M4 — outbound email dispatch (campaigns + transactional).
	// EMAIL_PRIMARY_PROVIDER orders the provider chain ("resend" or
	// "sendgrid", or a comma-separated preference list); every other
	// configured provider becomes a per-message fallback. When all keys
	// are empty, the shared sender (email.NewFromConfig) degrades to a
	// log-only transport so local/dev doesn't need a provider account.
	SendGridAPIKey       string `envconfig:"SENDGRID_API_KEY" default:""`
	ResendAPIKey         string `envconfig:"RESEND_API_KEY" default:""`
	EmailPrimaryProvider string `envconfig:"EMAIL_PRIMARY_PROVIDER" default:"sendgrid"`
	EmailFrom            string `envconfig:"EMAIL_FROM" default:"noreply@mark8ly.com"`

	// Otto support — deep link in customer ticket emails points back
	// to the storefront's tickets page so the customer can resume the
	// case (`{host}/support/tickets/{ticket_number}`). Empty disables
	// the CTA but keeps the email body intact. For multi-store
	// deployments where each store has its own subdomain, use
	// StorefrontBaseURLTemplate instead and per-store render — for
	// the single-brand v1 setup this static host is sufficient.
	PublicStorefrontHost string `envconfig:"PUBLIC_STOREFRONT_HOST" default:""`

	// PublicAPIBaseURL is the externally reachable origin for this
	// service — the host Stripe must call back on. It is NOT the
	// storefront host: the Istio wildcard routes api.mark8ly.com to
	// marketplace-api and every other *.mark8ly.com to the Next.js
	// apps, so a webhook registered against a store's own domain 404s.
	//
	// Used to auto-provision each store's Stripe webhook endpoint.
	// Empty disables provisioning (and is logged) rather than
	// registering an endpoint at a URL that cannot receive events.
	PublicAPIBaseURL string `envconfig:"MARKETPLACE_PUBLIC_API_BASE_URL" default:"https://api.mark8ly.com"`

	// Marketing M2 — Storefront URL template for gift card delivery
	// emails. {slug} is substituted with the store slug. Empty disables
	// the "Shop storefront" CTA in gift card emails.
	StorefrontBaseURLTemplate string `envconfig:"STOREFRONT_BASE_URL_TEMPLATE" default:"https://{slug}.mark8ly.com"`

	// Otto chat service — server-to-server URL and shared secret. The
	// customer's ticket detail view uses this to lazily pull the chat
	// transcript that spawned the ticket (when conversation_id is set).
	// Empty URL disables the transcript pull but leaves the rest of the
	// ticket flow untouched. The shared secret is sent as the
	// X-Internal-Auth header on every call.
	OttoURL          string `envconfig:"OTTO_URL" default:""`
	OttoInternalAuth string `envconfig:"OTTO_INTERNAL_AUTH" default:""`
	// OttoWSPublicBase is the public WebSocket origin mobile clients dial
	// for the real-time support chat, e.g. "wss://api.mark8ly.com". The
	// mobile support BFF returns "<base>/api/v1/storefront/otto/.../ws" so
	// the app connects straight to otto via the gateway (bypassing the
	// BFF). When empty the BFF derives it from the inbound request host.
	OttoWSPublicBase string `envconfig:"OTTO_WS_PUBLIC_BASE" default:""`

	// P7 — tax-ID validation (§19). NZTaxValidationEnabled gates the IRD
	// validator until counsel sign-off (§20.3); leave false in production
	// until legal approves. The remaining keys are per-registry secrets;
	// empty values mean "anonymous" for HMRC/VIES/ABN/ACRA (which permit
	// anonymous lookups) and "skip" for GSTN (token required in prod).
	NZTaxValidationEnabled  bool   `envconfig:"NZ_TAX_VALIDATION_ENABLED" default:"false"`
	GSTNAuthToken           string `envconfig:"GSTN_AUTH_TOKEN" default:""`
	ABNGUID                 string `envconfig:"AU_ABN_LOOKUP_GUID" default:""`
	TaxAttestationIPHashKey string `envconfig:"TAX_ATTESTATION_IP_HASH_KEY" default:""`

	// P14 — enterprise API key IP-hash key (§18.4). Same shape as the tax
	// attestation key but rotated independently; loaded from Secret Manager
	// in production. Empty disables IP hashing on last_used_ip_hash.
	APIKeyIPHashKey string `envconfig:"APIKEY_IP_HASH_KEY" default:""`

	// P15 — white-label mobile-app add-on lifecycle cron (spec §13.5).
	// Daily at 05:00 UTC advances sunset_scheduled rows through the
	// day 7/30/60/90 teardown.
	WhiteLabelLifecycleCron string `envconfig:"WHITE_LABEL_LIFECYCLE_CRON" default:"0 5 * * *"`

	// P15 — GCP project hosting the merchant_* Secret Manager secrets
	// referenced by internal/billing/appcreds. Empty = use the same
	// project as ADC's default (GKE Workload Identity).
	AppCredsProjectID string `envconfig:"APPCREDS_PROJECT_ID" default:""`

	// P18 — shipping carrier credentials for v2 country rollout (IE + NZ via
	// ShipEngine, VN via NinjaVan). Secrets are provisioned in GCP Secret
	// Manager; these fields wire the env-var reads.
	ShipEngineCarrierAccountIE string `envconfig:"SHIPENGINE_CARRIER_ACCOUNT_IE" default:""`
	ShipEngineCarrierAccountNZ string `envconfig:"SHIPENGINE_CARRIER_ACCOUNT_NZ" default:""`
	NinjaVanVNAPIKey           string `envconfig:"NINJAVAN_VN_API_KEY" default:""`
	NinjaVanVNClientID         string `envconfig:"NINJAVAN_VN_CLIENT_ID" default:""`
	NinjaVanVNClientSecret     string `envconfig:"NINJAVAN_VN_CLIENT_SECRET" default:""`

	// Per-tenant carrier credential store (Delhivery, Razorpay, TaxJar,
	// future providers). "gcpsm" → one GCP Secret Manager secret per
	// (tenant, domain, provider, field); "inline" → stores envelope-
	// encrypted ciphertext in the DB column (legacy behaviour). Default
	// is "inline" so local dev without GCP creds still boots.
	// Valid values: "inline", "gcpsm", "bao". Anything else is rejected
	// at startup by Validate — a typo here must not silently leave the
	// wrong backend primary (see ErrShippingSecretStoreUnknown).
	ShippingSecretStore string `envconfig:"SHIPPING_SECRET_STORE" default:"inline"`
	// GCPProjectID is the project that hosts the per-tenant carrier
	// secrets. Required when ShippingSecretStore is "gcpsm" or "bao" —
	// even bao-primary chains still route legacy gsm:// reads through
	// GCP Secret Manager. Unused when "inline".
	GCPProjectID string `envconfig:"GCP_PROJECT_ID" default:""`
	// SecretNamePrefix goes at the head of every per-tenant secret ID,
	// e.g. "mark8ly-prod", "mark8ly-test". Isolates dev and prod
	// clusters that share the same GCP project by scoping IAM
	// bindings via resource.name.startsWith.
	SecretNamePrefix string `envconfig:"SECRET_NAME_PREFIX" default:"mark8ly-dev"`

	// OpenBaoAddr is the OpenBao API address. Required whenever
	// ShippingSecretStore != "inline" — ChainStore routes any bao://
	// reference to OpenBao BY PREFIX regardless of which mode is
	// configured, so a "gcpsm" deployment with already-migrated rows
	// still needs a working address to resolve them. Validate rejects an
	// empty value outside inline mode for exactly this reason (see
	// ErrOpenBaoAddrRequired).
	OpenBaoAddr string `envconfig:"OPENBAO_ADDR" default:"http://openbao-active.openbao.svc.cluster.local:8200"`
	// OpenBaoRole is the Kubernetes auth role the OpenBao client logs in
	// as. Required whenever ShippingSecretStore != "inline", not only in
	// "bao" mode — a bao:// reference is routed to OpenBao by prefix
	// regardless of the configured mode, so rolling a deployment back
	// from "bao" to "gcpsm" still needs a working login role for any row
	// that already migrated, or that rollback silently breaks every
	// migrated tenant's checkout/shipping/webhook reads. Kubernetes auth
	// cannot proceed without it, so Validate rejects an empty value
	// outside inline mode.
	OpenBaoRole string `envconfig:"OPENBAO_ROLE" default:""`
	// OpenBaoKVMount is the KV v2 mount name. carriersecrets.BaoPath
	// currently hardcodes the "kv" mount prefix independently of this
	// setting, so Validate rejects any other value when
	// ShippingSecretStore=bao — a mismatch here must fail at boot, not
	// on the first merchant credential save. Unused otherwise.
	OpenBaoKVMount string `envconfig:"OPENBAO_KV_MOUNT" default:"kv"`

	// --- Merchant device push (mobile-admin) ---
	// PushEventsTopic is the Pub/Sub topic merchant notifications are
	// published to; a push subscription delivers them to the OIDC-gated
	// /pubsub/merchant-push endpoint, which fans out to the store's admin
	// devices via the Expo Push API. Empty disables publishing (the
	// notification stays in-app only).
	PushEventsTopic string `envconfig:"MARKETPLACE_PUSH_TOPIC" default:""`
	// PushOIDCAudience is the audience claim the push subscription stamps
	// on its OIDC token (typically the public endpoint URL). The handler
	// rejects any token whose aud differs. Empty disables verification —
	// only acceptable for local/dev, never prod.
	PushOIDCAudience string `envconfig:"PUSH_OIDC_AUDIENCE" default:""`
	// PushOIDCServiceAccount is the service-account email the push
	// subscription authenticates as. The handler rejects tokens whose
	// email claim differs, so only our subscription can invoke the
	// endpoint. Empty disables the email check.
	PushOIDCServiceAccount string `envconfig:"PUSH_OIDC_SERVICE_ACCOUNT" default:""`
}

// Fail-closed boot errors. Each names a setting whose empty value disables a
// control rather than breaking a feature, so an unset var must stop the boot.
var (
	ErrInternalAuthSecretRequired = errors.New(
		"marketplace config: MARKETPLACE_INTERNAL_AUTH_SECRET must be set when ENV != \"dev\" (HeaderTrustAuth gates the whole admin surface)")
	ErrCustomerSessionSecretRequired = errors.New(
		"marketplace config: CUSTOMER_SESSION_SECRET must be set when ENV != \"dev\" (HMAC key for storefront customer sessions)")
	ErrEncryptionModeRequired = errors.New(
		"marketplace config: ENCRYPTION_MODE must be \"aes\" when ENV != \"dev\" (noop stores merchant provider secrets as base64)")
	ErrEncryptionKeyRequired = errors.New(
		"marketplace config: ENCRYPTION_KEY must be set when ENCRYPTION_MODE=aes")

	// ErrShippingSecretStoreUnknown guards against a typo silently
	// leaving the wrong carrier-secret backend primary — checked in
	// every environment, not just non-dev, since a bad value is a
	// mistake regardless of ENV.
	ErrShippingSecretStoreUnknown = errors.New(
		"marketplace config: SHIPPING_SECRET_STORE must be \"inline\", \"gcpsm\", or \"bao\"")
	// ErrOpenBaoRoleRequired fires when ShippingSecretStore is anything
	// other than "inline" but OPENBAO_ROLE is unset. This is required in
	// "gcpsm" mode too, not only "bao" — ChainStore routes a bao://
	// reference to OpenBao by prefix regardless of which mode is
	// configured, so a deployment rolled back from "bao" to "gcpsm" still
	// needs a working role for any row that already migrated. Without
	// it, the Kubernetes auth login has no role to present and cannot
	// succeed.
	ErrOpenBaoRoleRequired = errors.New(
		"marketplace config: OPENBAO_ROLE must be set when SHIPPING_SECRET_STORE is not \"inline\" (ChainStore routes bao:// references to OpenBao by prefix regardless of mode, so a gcpsm rollback still needs a working role)")
	// ErrOpenBaoAddrRequired fires when ShippingSecretStore is anything
	// other than "inline" but OPENBAO_ADDR is empty, for the same
	// rollback-safety reason as ErrOpenBaoRoleRequired.
	ErrOpenBaoAddrRequired = errors.New(
		"marketplace config: OPENBAO_ADDR must be set when SHIPPING_SECRET_STORE is not \"inline\" (ChainStore routes bao:// references to OpenBao by prefix regardless of mode, so a gcpsm rollback still needs a working address)")
	// ErrOpenBaoKVMountUnsupported fires when ShippingSecretStore=bao
	// and OPENBAO_KV_MOUNT is anything other than "kv". This is a
	// carry-forward from an earlier task: carriersecrets.BaoPath
	// hardcodes the "kv/" logical path prefix independently of whatever
	// mount the OpenBao client is configured with, so a mismatch here
	// can no longer be expressed inside carriersecrets — every read and
	// write would fail at runtime with a "does not start with mount"
	// error on the first merchant credential save instead of at boot.
	ErrOpenBaoKVMountUnsupported = errors.New(
		"marketplace config: OPENBAO_KV_MOUNT must be \"kv\" when SHIPPING_SECRET_STORE=bao (carriersecrets.BaoPath currently assumes the \"kv\" mount)")
	// ErrGCPProjectIDRequiredForBao fires when ShippingSecretStore=bao
	// and GCP_PROJECT_ID is unset. A bao-primary ChainStore still routes
	// legacy gsm:// reads (and destroys) through GCP Secret Manager, so
	// GCP_PROJECT_ID is a genuine prerequisite for "bao" mode, not just
	// "gcpsm" mode.
	ErrGCPProjectIDRequiredForBao = errors.New(
		"marketplace config: GCP_PROJECT_ID must be set when SHIPPING_SECRET_STORE=bao (legacy gsm:// rows must still resolve through GCP Secret Manager)")
)

// Load reads .env (if present) and binds environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}

	// Trim whitespace on internal-auth shared secrets. GCP Secret Manager
	// values pushed via `echo "..." | gcloud secrets ...` carry a trailing
	// LF; ESO mounts the raw bytes verbatim, and the SHA-256 comparison in
	// the X-Internal-Auth middleware fails against trimmed values on the
	// receiving side (platform-api fixed this in commit e43b57a). The
	// 2026-05-05 bondi storefront incident (~25h of /api/v1/storefront/
	// stores/:slug/* 404s) was caused by exactly this trailing LF on
	// MARKETPLACE_PLATFORM_API_SECRET. Trimming here makes the binary
	// robust to future GCP-SM trailing-LF cases without requiring SM-side
	// cleanups.
	cfg.InternalAuthSecret = strings.TrimSpace(cfg.InternalAuthSecret)
	cfg.PlatformAPISecret = strings.TrimSpace(cfg.PlatformAPISecret)
	cfg.AuditIngestSecret = strings.TrimSpace(cfg.AuditIngestSecret)
	cfg.PlatformAdminSecret = strings.TrimSpace(cfg.PlatformAdminSecret)
	// Provider API keys go straight into Authorization headers — a
	// trailing LF from GCP SM would make net/http reject every request
	// with "invalid header field value".
	cfg.SendGridAPIKey = strings.TrimSpace(cfg.SendGridAPIKey)
	cfg.ResendWebhookSecret = strings.TrimSpace(cfg.ResendWebhookSecret)
	cfg.ResendAPIKey = strings.TrimSpace(cfg.ResendAPIKey)
	cfg.CustomerSessionSecret = strings.TrimSpace(cfg.CustomerSessionSecret)
	cfg.EncryptionKey = strings.TrimSpace(cfg.EncryptionKey)
	cfg.EncryptionMode = strings.ToLower(strings.TrimSpace(cfg.EncryptionMode))

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate refuses to boot outside dev when a security-critical setting is
// missing. Every one of these silently degrades to "no protection" when
// empty, so an unset env var would ship an open service rather than a
// broken one — the failure mode we can least afford to discover in prod.
func (c *Config) Validate() error {
	// Checked in every environment, including dev — a SHIPPING_SECRET_STORE
	// typo (or selecting bao without the settings it needs) is a
	// misconfiguration, not a security posture that dev gets to relax.
	if err := c.validateShippingSecretStore(); err != nil {
		return err
	}
	if c.Env == "dev" {
		return nil
	}
	if c.InternalAuthSecret == "" {
		return ErrInternalAuthSecretRequired
	}
	if c.CustomerSessionSecret == "" {
		return ErrCustomerSessionSecretRequired
	}
	if c.EncryptionMode != "aes" {
		return ErrEncryptionModeRequired
	}
	if c.EncryptionKey == "" {
		return ErrEncryptionKeyRequired
	}
	return nil
}

// validateShippingSecretStore rejects an unrecognised SHIPPING_SECRET_STORE
// value outright (rather than silently coercing it to the "inline"
// default) and checks the settings that a non-"inline" mode depends on but
// does not itself have a working fallback for.
//
// An explicitly-empty SHIPPING_SECRET_STORE is treated as "inline" rather
// than an unknown value: envconfig's `default` tag only applies when the
// variable is completely UNSET, so a var that is SET to "" would otherwise
// fail this switch and crash-loop the service, even though every chart
// today renders `| default "inline"` and would never produce that shape
// live. It is a startup trap, not a live risk, and it costs nothing to
// close.
func (c *Config) validateShippingSecretStore() error {
	if c.ShippingSecretStore == "" {
		c.ShippingSecretStore = "inline"
	}
	switch c.ShippingSecretStore {
	case "inline", "gcpsm", "bao":
	default:
		return fmt.Errorf("%w (got %q)", ErrShippingSecretStoreUnknown, c.ShippingSecretStore)
	}
	if c.ShippingSecretStore == "inline" {
		return nil
	}
	// OPENBAO_ROLE and OPENBAO_ADDR are required in "gcpsm" mode too, not
	// only "bao" — ChainStore.Get/Destroy route a bao:// reference to
	// OpenBao BY PREFIX unconditionally, regardless of which backend is
	// configured as primary. A deployment rolled back from "bao" to
	// "gcpsm" after any row has migrated still needs a working OpenBao
	// login to read it; without these settings that rollback breaks
	// checkout, shipping rates and payment webhooks for every migrated
	// tenant instead of restoring service.
	if c.OpenBaoRole == "" {
		return ErrOpenBaoRoleRequired
	}
	if c.OpenBaoAddr == "" {
		return ErrOpenBaoAddrRequired
	}
	if c.ShippingSecretStore != "bao" {
		return nil
	}
	if c.OpenBaoKVMount != "kv" {
		return fmt.Errorf("%w (got OPENBAO_KV_MOUNT=%q)", ErrOpenBaoKVMountUnsupported, c.OpenBaoKVMount)
	}
	if c.GCPProjectID == "" {
		return ErrGCPProjectIDRequiredForBao
	}
	return nil
}

// CarrierSecretJobConfig is the narrow config surface for a background
// job that only needs to build a per-tenant carrier secret Store (see
// internal/carriersecrets.Build) and talk to Postgres — e.g.
// cmd/refund-sweep-cron — as opposed to the full Config a Gin-serving
// process like cmd/marketplace-api needs.
type CarrierSecretJobConfig struct {
	DatabaseURL         string
	ShippingSecretStore string
	GCPProjectID        string
	SecretNamePrefix    string
	OpenBaoAddr         string
	OpenBaoRole         string
	OpenBaoKVMount      string
	// EncryptionMode/EncryptionKey stay in scope even though this is a
	// "secret store" loader, not a full auth surface: the store needs an
	// Encryptor for "inline" mode and for decoding legacy inline
	// (noop:/aes:) references under any mode.
	EncryptionMode string
	EncryptionKey  string
}

// carrierSecretJobEnv is the envconfig-tagged struct LoadCarrierSecretJob
// binds env vars into. Kept separate from Config so envconfig's
// required-field enforcement only applies to DATABASE_URL, not to
// Config's MARKETPLACE_FGA_API_URL (required unconditionally) or its
// prod-only auth secrets (see Validate) — none of which a carrier-secret
// job has any use for.
type carrierSecretJobEnv struct {
	DatabaseURL         string `envconfig:"DATABASE_URL" required:"true"`
	ShippingSecretStore string `envconfig:"SHIPPING_SECRET_STORE" default:"inline"`
	GCPProjectID        string `envconfig:"GCP_PROJECT_ID" default:""`
	SecretNamePrefix    string `envconfig:"SECRET_NAME_PREFIX" default:"mark8ly-dev"`
	OpenBaoAddr         string `envconfig:"OPENBAO_ADDR" default:"http://openbao-active.openbao.svc.cluster.local:8200"`
	OpenBaoRole         string `envconfig:"OPENBAO_ROLE" default:""`
	OpenBaoKVMount      string `envconfig:"OPENBAO_KV_MOUNT" default:"kv"`
	EncryptionMode      string `envconfig:"ENCRYPTION_MODE" default:""`
	EncryptionKey       string `envconfig:"ENCRYPTION_KEY" default:""`
}

// LoadCarrierSecretJob reads .env (if present) and binds only the env
// vars a carrier-secret-consuming background job needs: DATABASE_URL,
// SHIPPING_SECRET_STORE, GCP_PROJECT_ID, SECRET_NAME_PREFIX, OPENBAO_ADDR,
// OPENBAO_ROLE, OPENBAO_KV_MOUNT, ENCRYPTION_MODE, ENCRYPTION_KEY.
//
// It deliberately does NOT call Load(): Load's envconfig.Process(Config{})
// requires MARKETPLACE_FGA_API_URL unconditionally and, outside
// ENV=dev, MARKETPLACE_INTERNAL_AUTH_SECRET / CUSTOMER_SESSION_SECRET /
// ENCRYPTION_MODE=aes+ENCRYPTION_KEY (see Validate) — none of which a job
// that only builds a carrier secret Store and talks to Postgres has any
// use for. Loading the full Config here would force such a job to boot
// with settings it never reads, and would widen the blast radius of
// secrets (the customer session secret, the internal auth secret) it
// never touches.
//
// It DOES reuse Config.validateShippingSecretStore() — the one piece of
// validation that must never drift between the API and any other caller
// of internal/carriersecrets.Build — by copying the relevant fields onto
// a throwaway *Config and invoking that same method, rather than
// duplicating its logic here.
func LoadCarrierSecretJob() (*CarrierSecretJobConfig, error) {
	_ = godotenv.Load() // .env is optional

	var env carrierSecretJobEnv
	if err := envconfig.Process("", &env); err != nil {
		return nil, err
	}

	// validateShippingSecretStore is a method on *Config and also
	// normalises ShippingSecretStore ("" -> "inline") in place — run it
	// through a throwaway Config carrying only the fields it reads, then
	// copy the (possibly normalised) value back out.
	validation := &Config{
		ShippingSecretStore: env.ShippingSecretStore,
		GCPProjectID:        env.GCPProjectID,
		OpenBaoAddr:         env.OpenBaoAddr,
		OpenBaoRole:         env.OpenBaoRole,
		OpenBaoKVMount:      env.OpenBaoKVMount,
	}
	if err := validation.validateShippingSecretStore(); err != nil {
		return nil, err
	}

	return &CarrierSecretJobConfig{
		DatabaseURL:         env.DatabaseURL,
		ShippingSecretStore: validation.ShippingSecretStore,
		GCPProjectID:        env.GCPProjectID,
		SecretNamePrefix:    env.SecretNamePrefix,
		OpenBaoAddr:         env.OpenBaoAddr,
		OpenBaoRole:         env.OpenBaoRole,
		OpenBaoKVMount:      env.OpenBaoKVMount,
		EncryptionMode:      strings.ToLower(strings.TrimSpace(env.EncryptionMode)),
		EncryptionKey:       strings.TrimSpace(env.EncryptionKey),
	}, nil
}
