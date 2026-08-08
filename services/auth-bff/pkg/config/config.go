// Package config loads runtime configuration for auth-bff.
package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for auth-bff.
//
// auth-bff talks to real Google Identity Platform in every environment.
// Local dev uses the same GCP project as prod (or a separate dev project)
// — never an emulator. Bugs found in dev are real bugs.
type Config struct {
	Env         string `envconfig:"ENV" default:"dev"`
	HTTPPort    int    `envconfig:"HTTP_PORT" default:"8080"`
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Google Identity Platform
	GIPProjectID        string `envconfig:"GIP_PROJECT_ID" required:"true"`
	GIPProjectNumber    string `envconfig:"GIP_PROJECT_NUMBER" required:"true"`
	GIPWebAPIKey        string `envconfig:"GIP_WEB_API_KEY" required:"true"`
	GIPInternalTenantID string `envconfig:"GIP_INTERNAL_TENANT_ID" required:"true"` // staff/admin pool (e.g. MP-Internal-e986p)
	GIPCustomerTenantID string `envconfig:"GIP_CUSTOMER_TENANT_ID"`                 // storefront end-user pool (later)
	GIPPlatformTenantID string `envconfig:"GIP_PLATFORM_TENANT_ID"`                 // tesserix-home pool (later)

	// OAuth client (for the OIDC redirect flow auth-bff orchestrates)
	OAuthClientID     string `envconfig:"OAUTH_CLIENT_ID" required:"true"`
	OAuthClientSecret string `envconfig:"OAUTH_CLIENT_SECRET" required:"true"`

	// Cookie session
	SessionCookieName   string `envconfig:"SESSION_COOKIE_NAME" default:"m8_session"`
	SessionCookieDomain string `envconfig:"SESSION_COOKIE_DOMAIN" default:".mark8ly.local"`
	SessionEncryptKey   string `envconfig:"SESSION_ENCRYPT_KEY" required:"true"`

	// OpenFGA — used by autologin to verify membership tuple before issuing session
	FGAAPIURL  string `envconfig:"FGA_API_URL" default:"http://openfga:8080"`
	FGAStoreID string `envconfig:"FGA_STORE_ID"`

	// marketplace-api audit ingest. Empty URL disables emit (so dev
	// without marketplace-api still works).
	//
	// MarketplaceInternalAuthSecret is the existing service-to-service
	// secret reused by other inbound paths (e.g. auth-bff's
	// /internal/users handler that marketplace-api calls). Kept here for
	// backwards compatibility.
	//
	// AuditIngestSecret is the narrow secret gating ONLY the audit
	// ingest endpoint on marketplace-api. Forwarded as X-Internal-Auth
	// when set. Empty = audit endpoint runs in permissive mode.
	MarketplaceAPIURL             string `envconfig:"MARKETPLACE_API_URL"`
	MarketplaceInternalAuthSecret string `envconfig:"MARKETPLACE_INTERNAL_AUTH_SECRET"`
	AuditIngestSecret             string `envconfig:"AUDIT_INGEST_SECRET"`

	// platform-api notification send endpoint, used for the sign-in code
	// and the new-device alert. Empty URL leaves the whole email-OTP gate
	// off: unrecognised devices are still alerted about, but not
	// challenged. See EmailOTPPepper.
	PlatformAPIURL            string `envconfig:"PLATFORM_API_URL"`
	PlatformAPIInternalSecret string `envconfig:"PLATFORM_API_INTERNAL_AUTH_SECRET"`
	NotificationSupportEmail  string `envconfig:"NOTIFICATION_SUPPORT_EMAIL" default:"help@mark8ly.com"`
	NotificationSecurityURL   string `envconfig:"NOTIFICATION_SECURITY_URL" default:"https://admin.mark8ly.com/settings/security"`

	// EmailOTPPepper is the server-side secret mixed into every stored
	// code hash, so a database leak alone does not yield usable codes.
	// Must be at least 16 bytes. Empty disables the email-OTP gate.
	EmailOTPPepper string `envconfig:"EMAIL_OTP_PEPPER"`
}

// Load reads .env (if present) and binds environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
