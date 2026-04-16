// Package config loads runtime configuration from environment variables.
package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for platform-api.
type Config struct {
	Env         string `envconfig:"ENV" default:"dev"`
	HTTPPort    int    `envconfig:"HTTP_PORT" default:"8086"`
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// OpenFGA
	FGAAPIURL  string `envconfig:"FGA_API_URL" default:"http://openfga:8080"`
	FGAStoreID string `envconfig:"FGA_STORE_ID"`

	// Notification (inlined SendGrid for now)
	SendGridAPIKey string `envconfig:"SENDGRID_API_KEY"`
	EmailFrom      string `envconfig:"EMAIL_FROM" default:"noreply@mark8ly.local"`

	// Storage (inlined GCS for now)
	GCSBucket string `envconfig:"GCS_BUCKET"`

	// Admin base URL template. Used to build the accept-invite link
	// in invitation emails AND the admin login link in onboarding
	// welcome emails. In dev the default is http://localhost:4202;
	// in prod it's the per-tenant admin subdomain
	// https://%s-admin.mark8ly.com — callers format %s with the
	// tenant slug. If the template contains no %s verb (dev), the
	// slug is ignored and the flat host is used directly.
	AdminBaseURLTemplate string `envconfig:"ADMIN_BASE_URL_TEMPLATE" default:"http://localhost:4202"`
	// Storefront base URL template. Customer-facing store URL
	// surfaced to the merchant on the onboarding welcome page.
	// Same {slug} → tenant-slug substitution pattern as AdminBaseURLTemplate.
	StorefrontBaseURLTemplate string `envconfig:"STOREFRONT_BASE_URL_TEMPLATE" default:"http://localhost:4203"`
	// Onboarding base URL. Used to build the magic-link verification
	// URL in the email_verification email. Flat host — no per-slug
	// substitution, since the onboarding funnel is not tenant-scoped.
	// Dev: http://localhost:4201; prod: https://mark8ly.com.
	OnboardingBaseURL string `envconfig:"ONBOARDING_BASE_URL" default:"http://localhost:4201"`

	// AdminResetBaseURL is the fully-qualified origin for the admin
	// password-reset landing page. Flat (not per-slug) because the
	// link carries the oobCode and lands on the canonical admin host;
	// merchants can navigate into their own subdomain after resetting.
	// Dev: http://localhost:4202; prod: https://admin.mark8ly.com.
	AdminResetBaseURL string `envconfig:"ADMIN_RESET_BASE_URL" default:"http://localhost:4202"`

	// MarketplaceAPIURL is the base URL of the marketplace-api admin
	// service (internal endpoints). Platform-api calls it after
	// onboarding to create a tenant's self-vendor. Phase 1 of the
	// tenant/vendor/store refactor. Default targets the in-cluster
	// Knative admin revision; override with an env var for local dev.
	MarketplaceAPIURL string `envconfig:"MARKETPLACE_API_URL" default:"http://mark8ly-marketplace-api-admin.mark8ly.svc.cluster.local:8086"`

	// MarketplaceInternalAuthSecret is the shared secret used to sign
	// service-to-service calls to marketplace-api's HeaderTrustAuth
	// surface. Currently empty in prod (admin BFF migration pending).
	MarketplaceInternalAuthSecret string `envconfig:"MARKETPLACE_INTERNAL_AUTH_SECRET"`

	// AuditIngestSecret gates ONLY the /internal/audit-events endpoint
	// on marketplace-api. Forwarded as X-Internal-Auth on staff invite/
	// accept/revoke audit posts. Empty = audit endpoint runs in
	// permissive mode (rollout-safe default).
	AuditIngestSecret string `envconfig:"AUDIT_INGEST_SECRET"`

	// GIP (Google Identity Platform) admin settings used by the
	// password-reset flow. ProjectID + TenantID are required in prod;
	// WebAPIKey is the public Firebase Web API key. If any of the
	// three are blank, platform-api skips wiring the password-reset
	// handler (dev convenience — local dev without real GIP).
	GIPProjectID string `envconfig:"GIP_PROJECT_ID"`
	GIPTenantID  string `envconfig:"GIP_TENANT_ID"`
	GIPWebAPIKey string `envconfig:"GIP_WEB_API_KEY"`
}

// Load reads .env (if present) and binds environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error: .env is optional

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
