// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strings"

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

	// Notification — two providers, order picked by EMAIL_PRIMARY_PROVIDER
	// ("sendgrid" or "resend"); the other becomes the per-message fallback.
	// Rendered emails are identical on both providers, so flipping the
	// order is purely a config change.
	SendGridAPIKey       string `envconfig:"SENDGRID_API_KEY"`
	ResendAPIKey         string `envconfig:"RESEND_API_KEY"`
	EmailPrimaryProvider string `envconfig:"EMAIL_PRIMARY_PROVIDER" default:"sendgrid"`
	EmailFrom            string `envconfig:"EMAIL_FROM" default:"noreply@mark8ly.local"`

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
	MarketplaceAPIURL string `envconfig:"MARKETPLACE_API_URL" default:"http://mark8ly-marketplace-api-admin.mark8ly.svc.cluster.local:8080"`

	// MarketplaceInternalAuthSecret is the shared secret used to sign
	// service-to-service calls to marketplace-api's HeaderTrustAuth
	// surface. Currently empty in prod (admin BFF migration pending).
	MarketplaceInternalAuthSecret string `envconfig:"MARKETPLACE_INTERNAL_AUTH_SECRET"`

	// AuditIngestSecret gates ONLY the /internal/audit-events endpoint
	// on marketplace-api. Forwarded as X-Internal-Auth on staff invite/
	// accept/revoke audit posts. Empty = audit endpoint runs in
	// permissive mode (rollout-safe default).
	AuditIngestSecret string `envconfig:"AUDIT_INGEST_SECRET"`

	// InternalAuthSecret gates this service's own /internal/* surface.
	// Mirrors marketplace-api / auth-bff / otto: when set, every request
	// to /internal/* must carry a matching X-Internal-Auth header.
	// Empty = the gate is a no-op (rollout-safe default for dev and for
	// the cutover window before in-cluster callers start sending the
	// header). Verified via constant-time compare in
	// internal/middleware.RequireInternalAuth.
	InternalAuthSecret string `envconfig:"INTERNAL_AUTH_SECRET"`

	// GIP (Google Identity Platform) admin settings used by the
	// password-reset flow. ProjectID + TenantID are required in prod.
	// If project, tenant and a usable key are not all present,
	// platform-api skips wiring the password-reset handler (dev
	// convenience — local dev without real GIP).
	GIPProjectID string `envconfig:"GIP_PROJECT_ID"`
	GIPTenantID  string `envconfig:"GIP_TENANT_ID"`

	// GIPWebAPIKey is the PUBLIC Firebase Web API key — the same value the
	// admin browser bundle embeds. Browser keys carry an HTTP-referrer
	// restriction, and a server sends no Referer, so GIP answers
	// "Requests from referer <empty> are blocked" (403) to anything this
	// service calls with it. Kept only as a fallback so a deployment that
	// has not yet been given a server key behaves exactly as before.
	GIPWebAPIKey string `envconfig:"GIP_WEB_API_KEY"`

	// GIPServerAPIKey is a key with NO referrer restriction, for
	// server-to-server GIP calls. Prefer it over GIPWebAPIKey; see
	// GIPKey below. Relaxing the web key instead is not an option —
	// it is public by construction.
	GIPServerAPIKey string `envconfig:"GIP_SERVER_API_KEY"`

	// Zitadel (#524 phase 5). All optional and unread unless
	// ZitadelEnabled is set — mirrors services/auth-bff/pkg/config's
	// ZITADEL_ENABLED shape. GIP remains the default provider for the
	// three account operations (password-reset send/confirm, delete)
	// until this flag flips.
	//
	// EnsureTenantClaim (the tenant_id custom claim written on invite
	// accept and read by onboarding) is DELIBERATELY NOT gated by this
	// flag — it always runs against GIP via the gipAdmin client built
	// from GIPProjectID/GIPTenantID/GIPKey() above, independent of
	// ZitadelEnabled. See cmd/server/provider_wiring.go.
	//
	// Deployment ordering: ZitadelIssuer, ZitadelLoginClientToken, and
	// ZitadelOrgID must all be set — and land whitespace-clean, per the
	// TrimSpace-on-assignment note in Load() below — BEFORE ZitadelEnabled
	// is flipped to true. ValidateZitadel enforces this at startup (it
	// panics rather than falling back to GIP silently), but the flag
	// itself defaults to false, so merging this config change alone
	// changes nothing; only a deliberate, later flip of ZITADEL_ENABLED
	// activates the Zitadel path.
	ZitadelEnabled          bool   `envconfig:"ZITADEL_ENABLED" default:"false"`
	ZitadelIssuer           string `envconfig:"ZITADEL_ISSUER"`
	ZitadelLoginClientToken string `envconfig:"ZITADEL_LOGIN_CLIENT_TOKEN"`
	// ZitadelOrgID scopes the email->user-id search
	// (zitadeladmin.Client.resolveUserIDByEmail) to the merchant org —
	// required because the login-client token is instance-level and the
	// shared Zitadel instance hosts other products' orgs too.
	ZitadelOrgID string `envconfig:"ZITADEL_ORG_ID"`

	// ZitadelAdminProjectID is the Zitadel project id of the merchant
	// admin app (mark8ly-admin). Invitation accept must create a project
	// grant on it for every invited teammate: that project has
	// projectRoleCheck: true, so a user holding no role on it cannot
	// complete the OIDC flow at all (finalize returns
	// 403 OIDC-foSyH49RvL) no matter how correct their FGA tuples are.
	//
	// REQUIRED whenever ZitadelEnabled is true (see ValidateZitadel).
	// There is deliberately no default: the id is instance-specific, and
	// baking the production one into the binary would make a
	// staging/replacement instance silently grant roles on a project
	// that isn't the one it is serving.
	ZitadelAdminProjectID string `envconfig:"ZITADEL_ADMIN_PROJECT_ID"`

	// ZitadelStaffRoleKey is the role key granted on
	// ZitadelAdminProjectID. It defaults to the one role the
	// mark8ly-admin project declares in the zitadel-bootstrap chart, so
	// no deployment has to set it; it is configurable only so a future
	// role split does not require a code change to roll out.
	ZitadelStaffRoleKey string `envconfig:"ZITADEL_STAFF_ROLE_KEY" default:"mark8ly.staff"`
}

// GIPKey returns the API key to use for server-side GIP calls: the
// server key when configured, otherwise the public web key.
//
// The fallback exists so this change can ship before the server key does.
// It is not a permanent arrangement — a web key will keep failing
// referrer-restricted admin operations like resetPassword.
func (c *Config) GIPKey() string {
	if c.GIPServerAPIKey != "" {
		return c.GIPServerAPIKey
	}
	return c.GIPWebAPIKey
}

// Load reads .env (if present) and binds environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error: .env is optional

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	// GCP Secret Manager stores random-base64 / random-hex secrets with a
	// trailing newline that openssl + gcloud secrets create both emit.
	// HTTP strips trailing LF from header values in transit, so the env
	// value (with newline) and the X-Internal-Auth header (without)
	// would never constant-time-equal. otto + marketplace-api already
	// trim defensively; mirror that here so the gate works the moment
	// the secret syncs.
	cfg.InternalAuthSecret = strings.TrimSpace(cfg.InternalAuthSecret)
	cfg.MarketplaceInternalAuthSecret = strings.TrimSpace(cfg.MarketplaceInternalAuthSecret)
	cfg.AuditIngestSecret = strings.TrimSpace(cfg.AuditIngestSecret)
	// Provider API keys go straight into Authorization headers — a
	// trailing LF from GCP SM would make net/http reject every request
	// with "invalid header field value".
	cfg.SendGridAPIKey = strings.TrimSpace(cfg.SendGridAPIKey)
	cfg.ResendAPIKey = strings.TrimSpace(cfg.ResendAPIKey)
	// ZitadelIssuer becomes an HTTP request base URL and ZitadelOrgID is
	// sent as a header value verbatim — a trailing newline from a mounted
	// secret broke exactly this shape elsewhere in this codebase for
	// ~25 hours before TrimSpace-on-assignment became the standing rule.
	// ZitadelLoginClientToken is a bearer credential; trim it for the same
	// reason SendGridAPIKey/ResendAPIKey are trimmed above.
	cfg.ZitadelIssuer = strings.TrimSpace(cfg.ZitadelIssuer)
	cfg.ZitadelLoginClientToken = strings.TrimSpace(cfg.ZitadelLoginClientToken)
	cfg.ZitadelOrgID = strings.TrimSpace(cfg.ZitadelOrgID)
	// Both go into JSON request bodies Zitadel matches EXACTLY (a project
	// id and a role key); a trailing newline from a mounted secret or a
	// heredoc-written ConfigMap would produce a 404 on the project or a
	// grant carrying a role nobody holds.
	cfg.ZitadelAdminProjectID = strings.TrimSpace(cfg.ZitadelAdminProjectID)
	cfg.ZitadelStaffRoleKey = strings.TrimSpace(cfg.ZitadelStaffRoleKey)
	return &cfg, nil
}

// ErrZitadelNotConfigured is returned by ValidateZitadel when ZITADEL_ENABLED
// is set but a value the Zitadel account-operations path cannot run safely
// without is missing. Wrapped errors carry the NAME of the missing variable
// only — never its value.
var ErrZitadelNotConfigured = errors.New("zitadel: enabled but not configured")

// ValidateZitadel reports whether platform-api may select Zitadel for its
// three account operations (password-reset send, password-reset confirm,
// delete account).
//
// It returns nil when Zitadel is disabled: GIP stays the provider and
// nothing new is required to boot. When it is enabled, every field below is
// mandatory and a missing one must fail startup loudly (see
// cmd/server/provider_wiring.go) rather than silently falling back to GIP —
// a misconfigured Zitadel deployment must never look like a working one.
//
// Mirrors services/auth-bff/pkg/config's ValidateZitadel shape.
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
	if c.ZitadelOrgID == "" {
		missing = append(missing, "ZITADEL_ORG_ID")
	}
	// DEPLOY ORDER: this is a NEW required variable. The chart must set
	// ZITADEL_ADMIN_PROJECT_ID before a build carrying it reaches a
	// deployment that already has ZITADEL_ENABLED=true, or platform-api
	// refuses to boot. It is required rather than optional because the
	// alternative — accepting an invitation and skipping the project
	// grant — produces exactly the failure this variable exists to
	// prevent: a teammate who looks provisioned and cannot sign in.
	// Refusing at startup surfaces that in the rollout; skipping the
	// grant surfaces it days later as a support ticket.
	if c.ZitadelAdminProjectID == "" {
		missing = append(missing, "ZITADEL_ADMIN_PROJECT_ID")
	}
	// ZitadelStaffRoleKey is NOT listed: it carries a default, so it can
	// only be empty if an operator explicitly set it to the empty string.
	if c.ZitadelStaffRoleKey == "" {
		missing = append(missing, "ZITADEL_STAFF_ROLE_KEY")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: missing %s", ErrZitadelNotConfigured, strings.Join(missing, ", "))
}
