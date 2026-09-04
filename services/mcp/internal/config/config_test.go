package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad_TrimsSecretsOnAssignment(t *testing.T) {
	t.Setenv("STOREFRONT_BASE_URL", "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080\n")
	t.Setenv("STOREFRONT_KEY", "  sfkey\n")
	t.Setenv("MCP_AUTH_KEY", "mcpkey\n")

	cfg, err := Load()
	require.NoError(t, err)

	// A trailing newline from a mounted secret has cost this codebase a
	// ~25-hour outage before. Assert the TRIMMED values, not just non-empty.
	require.Equal(t, "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080", cfg.StorefrontBaseURL)
	require.Equal(t, "sfkey", cfg.StorefrontKey)
	require.Equal(t, "mcpkey", cfg.MCPKey)
	require.Equal(t, 400*time.Millisecond, cfg.UpstreamTimeout)
}

func TestLoad_MissingRequiredFailsClosed(t *testing.T) {
	for _, missing := range []string{"STOREFRONT_BASE_URL", "STOREFRONT_KEY", "MCP_AUTH_KEY"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("STOREFRONT_BASE_URL", "http://x:8080")
			t.Setenv("STOREFRONT_KEY", "k")
			t.Setenv("MCP_AUTH_KEY", "m")
			t.Setenv(missing, "")

			_, err := Load()
			require.Error(t, err, "%s is required; starting without it would serve an unauthenticated or unreachable connector", missing)
			require.Contains(t, err.Error(), missing)
		})
	}
}

func TestLoad_StorefrontBaseURLValidation_NoScheme(t *testing.T) {
	t.Setenv("STOREFRONT_BASE_URL", "mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080")
	t.Setenv("STOREFRONT_KEY", "k")
	t.Setenv("MCP_AUTH_KEY", "m")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "STOREFRONT_BASE_URL")
}

func TestLoad_StorefrontBaseURLValidation_NotAURL(t *testing.T) {
	t.Setenv("STOREFRONT_BASE_URL", "not a url")
	t.Setenv("STOREFRONT_KEY", "k")
	t.Setenv("MCP_AUTH_KEY", "m")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "STOREFRONT_BASE_URL")
}

func TestLoad_StorefrontBaseURLValidation_ValidURL(t *testing.T) {
	t.Setenv("STOREFRONT_BASE_URL", "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080\n")
	t.Setenv("STOREFRONT_KEY", "  sfkey\n")
	t.Setenv("MCP_AUTH_KEY", "mcpkey\n")

	cfg, err := Load()
	require.NoError(t, err)

	// Verify the trimmed value is still correctly stored
	require.Equal(t, "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080", cfg.StorefrontBaseURL)
}
