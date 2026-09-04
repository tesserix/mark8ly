package config

import (
	"errors"
	"strings"
	"testing"
)

func zitadelReadyConfig() Config {
	return Config{
		ZitadelEnabled:                            true,
		ZitadelIssuer:                             "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken:                   "pat",
		MarketplaceInternalAuthSecret:             "s3cret-internal",
		ZitadelReturnURLAllowedHostsAdmin:         []string{"admin.mark8ly.com"},
		ZitadelReturnURLAllowedSuffixesStorefront: []string{"mark8ly.com"},
		ZitadelGoogleIDPID:                        "386381087862948767",
		ZitadelOrgID:                              "339070697432875523",
		PlatformAPIURL:                            "http://mark8ly-platform-api.mark8ly.svc.cluster.local:8086",
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
		{"admin return url allowlist", func(c *Config) {
			c.ZitadelReturnURLAllowedHostsAdmin = nil
			c.ZitadelReturnURLAllowedSuffixesAdmin = nil
		}, "ZITADEL_RETURN_URL_ALLOWED_HOSTS_ADMIN"},
		{"storefront return url allowlist", func(c *Config) {
			c.ZitadelReturnURLAllowedHostsStorefront = nil
			c.ZitadelReturnURLAllowedSuffixesStorefront = nil
		}, "ZITADEL_RETURN_URL_ALLOWED_HOSTS_STOREFRONT"},
		{"google idp id", func(c *Config) { c.ZitadelGoogleIDPID = "" }, "ZITADEL_GOOGLE_IDP_ID"},
		{"org id", func(c *Config) { c.ZitadelOrgID = "" }, "ZITADEL_ORG_ID"},
		{"platform api url", func(c *Config) { c.PlatformAPIURL = "" }, "PLATFORM_API_URL"},
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

// TestValidateZitadelAcceptsEitherAllowlistFieldAlonePerFlow: hosts and
// suffixes serve different shapes (fixed admin host vs. per-tenant
// storefront subdomains) and a deployment need not use both within a flow.
func TestValidateZitadelAcceptsEitherAllowlistFieldAlonePerFlow(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.ZitadelReturnURLAllowedSuffixesAdmin = nil // was already nil; hosts alone carries admin
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("admin hosts alone: ValidateZitadel = %v, want nil", err)
	}

	cfg = zitadelReadyConfig()
	cfg.ZitadelReturnURLAllowedHostsStorefront = nil // was already nil; suffixes alone carries storefront
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("storefront suffixes alone: ValidateZitadel = %v, want nil", err)
	}
}

// TestValidateZitadelKeepsAdminAndStorefrontAllowlistsIndependent: this is
// the config-level half of the admin/storefront split — configuring one
// flow's allowlist must never satisfy the other's requirement. (The actual
// cross-flow rejection — a storefront subdomain used as an admin
// successUrl — is exercised by choosing which ReturnURLAllowlist to
// validate against, not by Config; this test only pins that ValidateZitadel
// requires BOTH sets, not either-or across flows.)
func TestValidateZitadelKeepsAdminAndStorefrontAllowlistsIndependent(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.ZitadelReturnURLAllowedHostsStorefront = nil
	cfg.ZitadelReturnURLAllowedSuffixesStorefront = nil
	// Admin is still fully configured, but storefront is now empty — must
	// still fail, since a flat "any allowlist configured" check would have
	// silently passed here.
	if err := cfg.ValidateZitadel(); err == nil {
		t.Fatal("ValidateZitadel = nil; admin being configured must not excuse an empty storefront allowlist")
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

// TestValidateZitadelRefusesWithoutThePlatformAPIURL is review Finding 4:
// PLATFORM_API_URL is what internal/notify posts to, and notify is the only
// way the sign-up verification code ever reaches a shopper. main.go builds
// no notify client when it is empty, so CustomerHandler gets a nil mailer
// and register creates-then-rolls-back-then-503s on every single attempt —
// customer sign-up 100% broken on a process reporting healthy. A log.Warn
// was the only signal; refusing to boot is the correct one.
func TestValidateZitadelRefusesWithoutThePlatformAPIURL(t *testing.T) {
	cfg := zitadelReadyConfig()
	cfg.PlatformAPIURL = ""

	err := cfg.ValidateZitadel()
	if err == nil {
		t.Fatal("ValidateZitadel = nil; a Zitadel-enabled auth-bff with no PLATFORM_API_URL must refuse to start rather than serve a sign-up that can never succeed")
	}
	if !errors.Is(err, ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap ErrZitadelNotConfigured", err)
	}
	if !strings.Contains(err.Error(), "PLATFORM_API_URL") {
		t.Fatalf("err = %v, want it to name the missing variable", err)
	}
	// The operator reading this at boot must be told what breaks, not just
	// which key is absent.
	if !strings.Contains(err.Error(), "sign-up") {
		t.Fatalf("err = %v, want it to say what stops working without the variable", err)
	}
}
