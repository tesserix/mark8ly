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

	// GIP — used by the onboarding "verify-google" path to validate a
	// Google-issued GIP id_token via Identity Toolkit accounts:lookup
	// before bypassing the magic-link verification step.
	GIPAPIKey   string `envconfig:"GIP_API_KEY"`
	GIPTenantID string `envconfig:"GIP_TENANT_ID"`
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
