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
