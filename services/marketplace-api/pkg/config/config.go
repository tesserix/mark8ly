// Package config loads runtime configuration from environment variables.
package config

import (
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

	// S1 — auth-bff URL for MFA/session proxying.
	AuthBFFURL string `envconfig:"AUTH_BFF_URL" default:""`
	// S3 — Stripe Billing keys + webhook / orphan cron config.
	StripeBillingSecretKey      string        `envconfig:"STRIPE_BILLING_SECRET_KEY" default:""`
	StripeBillingWebhookSecret  string        `envconfig:"STRIPE_BILLING_WEBHOOK_SECRET" default:""`
	StripeAllowedEventTypes     []string      `envconfig:"STRIPE_ALLOWED_EVENT_TYPES" default:"checkout.session.completed,customer.subscription.updated,customer.subscription.deleted,invoice.paid,invoice.payment_failed,invoice.payment_action_required,customer.updated,charge.refunded,payment_method.attached,payment_method.detached,radar.early_fraud_warning"`
	WebhookMaxBodyBytes         int64         `envconfig:"WEBHOOK_MAX_BODY_BYTES" default:"524288"`
	OrphanRetryMaxCount         int           `envconfig:"ORPHAN_RETRY_MAX_COUNT" default:"6"`
	OrphanRetryInterval         time.Duration `envconfig:"ORPHAN_RETRY_INTERVAL" default:"5m"`
	OrphanStaleThreshold        time.Duration `envconfig:"ORPHAN_STALE_THRESHOLD" default:"1h"`
	PagerDutyWebhookURL         string        `envconfig:"PAGERDUTY_WEBHOOK_URL" default:""`

	// AUDIT — shared secret gating /internal/audit-events. When empty,
	// the endpoint is permissive (dev convenience). Set to the SAME
	// non-empty value on auth-bff and platform-api to enforce
	// service-to-service auth on the audit ingest path. Deliberately
	// separate from InternalAuthSecret (which gates HeaderTrustAuth
	// across the entire admin surface and would require admin BFF
	// changes to enable safely).
	AuditIngestSecret string `envconfig:"AUDIT_INGEST_SECRET" default:""`

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

	// Marketing M4 — Email dispatch for campaigns. When SendGridAPIKey is
	// empty, the campaign worker falls back to LogDispatcher (no emails
	// sent) so local/dev doesn't need a SendGrid account.
	SendGridAPIKey string `envconfig:"SENDGRID_API_KEY" default:""`
	EmailFrom      string `envconfig:"EMAIL_FROM" default:"noreply@mark8ly.com"`

	// Marketing M2 — Storefront URL template for gift card delivery
	// emails. {slug} is substituted with the store slug. Empty disables
	// the "Shop storefront" CTA in gift card emails.
	StorefrontBaseURLTemplate string `envconfig:"STOREFRONT_BASE_URL_TEMPLATE" default:"https://{slug}.mark8ly.com"`

	// P7 — tax-ID validation (§19). NZTaxValidationEnabled gates the IRD
	// validator until counsel sign-off (§20.3); leave false in production
	// until legal approves. The remaining keys are per-registry secrets;
	// empty values mean "anonymous" for HMRC/VIES/ABN/ACRA (which permit
	// anonymous lookups) and "skip" for GSTN (token required in prod).
	NZTaxValidationEnabled bool   `envconfig:"NZ_TAX_VALIDATION_ENABLED" default:"false"`
	GSTNAuthToken          string `envconfig:"GSTN_AUTH_TOKEN" default:""`
	ABNGUID                string `envconfig:"AU_ABN_LOOKUP_GUID" default:""`
	TaxAttestationIPHashKey string `envconfig:"TAX_ATTESTATION_IP_HASH_KEY" default:""`

	// P14 — enterprise API key IP-hash key (§18.4). Same shape as the tax
	// attestation key but rotated independently; loaded from Secret Manager
	// in production. Empty disables IP hashing on last_used_ip_hash.
	APIKeyIPHashKey string `envconfig:"APIKEY_IP_HASH_KEY" default:""`
}

// Load reads .env (if present) and binds environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
