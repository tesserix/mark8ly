package config

import (
	"errors"
	"strings"
	"testing"
)

func zitadelReadyConfig() Config {
	return Config{
		ZitadelEnabled:                  true,
		ZitadelIssuer:                   "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken:         "pat",
		MarketplaceInternalAuthSecret:   "s3cret-internal",
		ZitadelReturnURLAllowedHosts:    []string{"admin.mark8ly.com"},
		ZitadelReturnURLAllowedSuffixes: []string{"mark8ly.com"},
	}
}

// TestValidateZitadelRefusesWithoutTheInternalSecret is the boot guard for
// the credential-oracle fix: /auth/zitadel/login and /auth/customer/login
// answer whether a {login_name, password} pair is valid, auth-bff is
// publicly reachable, and the header check that gates them is a no-op when
// the secret is empty. Booting in that state would publish exactly the
// endpoint the header exists to prevent.
func TestValidateZitadelRefusesWithoutTheInternalSecret(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.MarketplaceInternalAuthSecret = ""

	err := cfg.ValidateZitadel()
	if err == nil {
		t.Fatal("ValidateZitadel = nil; a Zitadel-enabled auth-bff with no internal secret must refuse to start")
	}
	if !errors.Is(err, ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap ErrZitadelNotConfigured", err)
	}
	if !strings.Contains(err.Error(), "MARKETPLACE_INTERNAL_AUTH_SECRET") {
		t.Fatalf("err = %v, want it to name the missing variable", err)
	}
}

func TestValidateZitadelRefusesOnEachMissingValue(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
		want string
	}{
		{"issuer", func(c *Config) { c.ZitadelIssuer = "" }, "ZITADEL_ISSUER"},
		{"login client token", func(c *Config) { c.ZitadelLoginClientToken = "" }, "ZITADEL_LOGIN_CLIENT_TOKEN"},
		{"internal secret", func(c *Config) { c.MarketplaceInternalAuthSecret = "" }, "MARKETPLACE_INTERNAL_AUTH_SECRET"},
		{"return url allowlist", func(c *Config) {
			c.ZitadelReturnURLAllowedHosts = nil
			c.ZitadelReturnURLAllowedSuffixes = nil
		}, "ZITADEL_RETURN_URL_ALLOWED_HOSTS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := zitadelReadyConfig()
			tc.mut(&cfg)
			err := cfg.ValidateZitadel()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want an error naming %s", err, tc.want)
			}
		})
	}
}

// TestValidateZitadelNeverEchoesASecretValue: the error is logged and
// panicked on at boot, so it must name variables, never their contents.
func TestValidateZitadelNeverEchoesASecretValue(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.ZitadelIssuer = ""
	err := cfg.ValidateZitadel()
	if err == nil {
		t.Fatal("want an error")
	}
	for _, secret := range []string{cfg.ZitadelLoginClientToken, cfg.MarketplaceInternalAuthSecret} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("err = %v leaks a configured secret value", err)
		}
	}
}

// TestValidateZitadelAcceptsEitherAllowlistFieldAlone: hosts and suffixes
// serve different shapes (fixed admin host vs. per-tenant storefront
// subdomains) and a deployment need not use both.
func TestValidateZitadelAcceptsEitherAllowlistFieldAlone(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.ZitadelReturnURLAllowedSuffixes = nil
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("hosts alone: ValidateZitadel = %v, want nil", err)
	}

	cfg = zitadelReadyConfig()
	cfg.ZitadelReturnURLAllowedHosts = nil
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("suffixes alone: ValidateZitadel = %v, want nil", err)
	}
}

func TestValidateZitadelPassesWhenFullyConfigured(t *testing.T) {
	cfg := zitadelReadyConfig()
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("ValidateZitadel = %v, want nil", err)
	}
}

// TestValidateZitadelIgnoresEverythingWhenDisabled pins that this change
// does not touch the GIP path: with ZITADEL_ENABLED unset (production
// today), no Zitadel route is mounted and nothing new is required to boot.
func TestValidateZitadelIgnoresEverythingWhenDisabled(t *testing.T) {
	cfg := Config{ZitadelEnabled: false}
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("ValidateZitadel = %v, want nil when Zitadel is disabled", err)
	}
}
