package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds runtime settings for the otto service.
// All fields load from env vars via kelseyhightower/envconfig.
type Config struct {
	Env      string `envconfig:"ENV" default:"dev"`
	HTTPPort int    `envconfig:"HTTP_PORT" default:"8080"`

	MongoURL      string `envconfig:"MONGO_URL" required:"true"`
	MongoDatabase string `envconfig:"MONGO_DATABASE" default:"otto"`

	// CustomerSessionCookie — signed HttpOnly cookie used by anonymous
	// storefront visitors so they can reconnect to their own thread without
	// leaking access to anyone else's messages.
	CustomerSessionCookie string `envconfig:"CUSTOMER_SESSION_COOKIE" default:"otto_session"`
	CustomerSessionSecret string `envconfig:"CUSTOMER_SESSION_SECRET" required:"true"`
	CustomerCookieDomain  string `envconfig:"CUSTOMER_COOKIE_DOMAIN" default:""`
	CustomerCookieSecure  bool   `envconfig:"CUSTOMER_COOKIE_SECURE" default:"true"`

	// Trust boundary with the storefront/admin Next.js apps (matches the
	// convention used by marketplace-api: a shared key the proxy injects as
	// X-Internal-Auth so raw public traffic can never reach /api/v1/* routes
	// without going through the app's server-side proxy first).
	InternalAuthSecret string `envconfig:"INTERNAL_AUTH_SECRET" default:""`

	// CORS — comma-separated list of origins allowed to call the service
	// directly (the WebSocket upgrade needs this; REST calls go through the
	// Next.js proxy and do not).
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:""`
}

// Load reads .env (best-effort) then populates a Config from env vars.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
