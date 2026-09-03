// Package config loads runtime configuration for auth-bff.
package config

import (
	"errors"
	"fmt"
	"strings"

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

	// Zitadel (#524 phase 2). All optional and unread unless ZitadelEnabled is
	// set: GIP remains the live provider until the phase 6 cutover.
	ZitadelEnabled             bool   `envconfig:"ZITADEL_ENABLED" default:"false"`
	ZitadelIssuer              string `envconfig:"ZITADEL_ISSUER"`
	ZitadelLoginClientToken    string `envconfig:"ZITADEL_LOGIN_CLIENT_TOKEN"`
	ZitadelAdminProjectID      string `envconfig:"ZITADEL_ADMIN_PROJECT_ID"`
	ZitadelStorefrontProjectID string `envconfig:"ZITADEL_STOREFRONT_PROJECT_ID"`

	// Zitadel IDP-intent return-URL allowlists (internal/zitadellogin's
	// ReturnURLAllowlist) — the only control preventing an open redirect on
	// a completed federated sign-in, since Zitadel itself does not validate
	// successUrl/failureUrl at all. Comma-separated. Hosts match exactly
	// (e.g. "admin.mark8ly.com"); Suffixes permit any subdomain of the
	// given domain (e.g. "mark8ly.com" permits "shop.mark8ly.com") but
	// never the bare domain itself.
	//
	// Deliberately split in two, one per flow, rather than a single shared
	// list: merchants self-provision storefront subdomains
	// (*.mark8ly.com), so a flat allowlist covering both flows would make
	// every merchant-controlled storefront a valid successUrl for an ADMIN
	// sign-in too — a merchant-controlled origin able to receive a
	// completed admin login. Admin gets its own narrow set (the fixed
	// admin host, no subdomain suffixes needed); Storefront gets the
	// tenant-subdomain suffix. The zitadellogin.ReturnURLAllowlist type
	// itself stays flow-agnostic — selecting which one applies to a given
	// request is the caller's job (the handler wired to /auth/zitadel/*
	// vs. /auth/customer/*), not something this type or Config decides.
	ZitadelReturnURLAllowedHostsAdmin         []string `envconfig:"ZITADEL_RETURN_URL_ALLOWED_HOSTS_ADMIN"`
	ZitadelReturnURLAllowedSuffixesAdmin      []string `envconfig:"ZITADEL_RETURN_URL_ALLOWED_SUFFIXES_ADMIN"`
	ZitadelReturnURLAllowedHostsStorefront    []string `envconfig:"ZITADEL_RETURN_URL_ALLOWED_HOSTS_STOREFRONT"`
	ZitadelReturnURLAllowedSuffixesStorefront []string `envconfig:"ZITADEL_RETURN_URL_ALLOWED_SUFFIXES_STOREFRONT"`
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

// ErrZitadelNotConfigured is returned by ValidateZitadel when ZITADEL_ENABLED
// is set but a value the Zitadel login path cannot run safely without is
// missing. Wrapped errors carry the NAME of the missing variable only —
// never its value.
var ErrZitadelNotConfigured = errors.New("zitadel: enabled but not configured")

// ValidateZitadel reports whether the Zitadel login path may be mounted.
//
// It returns nil when Zitadel is disabled: nothing is mounted, so nothing
// needs configuring. When it is enabled, every field below is mandatory and
// a missing one is a boot failure, not a request-time one.
//
// MarketplaceInternalAuthSecret is in that list because /auth/zitadel/login
// and /auth/customer/login answer whether a {login_name, password} pair is
// valid, auth-bff is publicly reachable, and the login-client PAT behind
// them is instance-level. Without the shared secret those routes are an
// unauthenticated credential oracle over every user in the instance. A
// silently-unauthenticated auth endpoint is exactly what the header check
// exists to prevent, so refusing to boot is the only safe answer.
func (c *Config) ValidateZitadel() error {
	if !c.ZitadelEnabled {
		return nil
	}
	var missing []string
	if c.ZitadelIssuer == "" {
		missing = append(missing, "ZITADEL_ISSUER")
	}
	if c.ZitadelLoginClientToken == "" {
		missing = append(missing, "ZITADEL_LOGIN_CLIENT_TOKEN")
	}
	if c.MarketplaceInternalAuthSecret == "" {
		missing = append(missing, "MARKETPLACE_INTERNAL_AUTH_SECRET")
	}
	// Both flows are mounted together whenever Zitadel is enabled (see
	// cmd/server/main.go — there is no separate flag to run one without
	// the other today), so both allowlists are required here. If that ever
	// changes, this check must follow whatever signal decides a flow is
	// actually mounted, not just assume both.
	if len(c.ZitadelReturnURLAllowedHostsAdmin) == 0 && len(c.ZitadelReturnURLAllowedSuffixesAdmin) == 0 {
		missing = append(missing, "ZITADEL_RETURN_URL_ALLOWED_HOSTS_ADMIN or ZITADEL_RETURN_URL_ALLOWED_SUFFIXES_ADMIN")
	}
	if len(c.ZitadelReturnURLAllowedHostsStorefront) == 0 && len(c.ZitadelReturnURLAllowedSuffixesStorefront) == 0 {
		missing = append(missing, "ZITADEL_RETURN_URL_ALLOWED_HOSTS_STOREFRONT or ZITADEL_RETURN_URL_ALLOWED_SUFFIXES_STOREFRONT")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: missing %s", ErrZitadelNotConfigured, strings.Join(missing, ", "))
}
