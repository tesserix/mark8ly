package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the MCP catalog connector's configuration.
type Config struct {
	StorefrontBaseURL string
	StorefrontKey     string
	MCPKey            string
	Port              int
	UpstreamTimeout   time.Duration
}

// Load reads and validates configuration from environment variables.
// It trims all values and returns an error if any required variable is missing or empty.
func Load() (Config, error) {
	cfg := Config{}

	// Required variables
	storefrontBaseURL := strings.TrimSpace(os.Getenv("STOREFRONT_BASE_URL"))
	if storefrontBaseURL == "" {
		return Config{}, fmt.Errorf("STOREFRONT_BASE_URL is required")
	}
	u, err := url.ParseRequestURI(storefrontBaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("STOREFRONT_BASE_URL must be a valid absolute URL")
	}
	cfg.StorefrontBaseURL = storefrontBaseURL

	storefrontKey := strings.TrimSpace(os.Getenv("STOREFRONT_KEY"))
	if storefrontKey == "" {
		return Config{}, fmt.Errorf("STOREFRONT_KEY is required")
	}
	cfg.StorefrontKey = storefrontKey

	mcpKey := strings.TrimSpace(os.Getenv("MCP_AUTH_KEY"))
	if mcpKey == "" {
		return Config{}, fmt.Errorf("MCP_AUTH_KEY is required")
	}
	cfg.MCPKey = mcpKey

	// Optional variables with defaults
	portStr := strings.TrimSpace(os.Getenv("PORT"))
	if portStr == "" {
		cfg.Port = 8765
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return Config{}, fmt.Errorf("PORT must be a valid integer: %w", err)
		}
		cfg.Port = port
	}

	upstreamTimeoutStr := strings.TrimSpace(os.Getenv("UPSTREAM_TIMEOUT"))
	if upstreamTimeoutStr == "" {
		cfg.UpstreamTimeout = 400 * time.Millisecond
	} else {
		timeout, err := time.ParseDuration(upstreamTimeoutStr)
		if err != nil {
			return Config{}, fmt.Errorf("UPSTREAM_TIMEOUT must be a valid duration: %w", err)
		}
		cfg.UpstreamTimeout = timeout
	}

	return cfg, nil
}
