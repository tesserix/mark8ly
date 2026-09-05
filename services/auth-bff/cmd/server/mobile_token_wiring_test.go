package main

import (
	"testing"

	"github.com/mark8ly/auth-bff/pkg/config"
)

// mobileTokenIssuanceEnabled mirrors the guard in main.go. It exists so the
// "all four or nothing" rule is testable without booting the server.
func mobileTokenIssuanceEnabled(cfg *config.Config) bool {
	return cfg.ZitadelAdminClientID != "" && cfg.ZitadelAdminClientSecret != "" &&
		cfg.ZitadelAdminRedirectURI != "" && cfg.ZitadelAdminProjectID != ""
}

// Half-configured must mean OFF, not half-on. A login that "succeeds"
// without a usable token is indistinguishable from a working one until the
// first API call 401s — from a device, that is the hardest possible failure
// to diagnose. Refusing at 500 is loud and immediate instead.
func TestMobileTokenIssuance_RequiresEveryValue(t *testing.T) {
	full := config.Config{
		ZitadelAdminClientID:     "id",
		ZitadelAdminClientSecret: "placeholder",
		ZitadelAdminRedirectURI:  "https://admin.mark8ly.com/auth/callback",
		ZitadelAdminProjectID:    "proj",
	}
	if !mobileTokenIssuanceEnabled(&full) {
		t.Fatal("a fully configured deployment must enable mobile token issuance")
	}

	for _, tc := range []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"no client id", func(c *config.Config) { c.ZitadelAdminClientID = "" }},
		{"no client secret", func(c *config.Config) { c.ZitadelAdminClientSecret = "" }},
		{"no redirect uri", func(c *config.Config) { c.ZitadelAdminRedirectURI = "" }},
		{"no project id", func(c *config.Config) { c.ZitadelAdminProjectID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := full
			tc.mutate(&cfg)
			if mobileTokenIssuanceEnabled(&cfg) {
				t.Fatalf("%s must disable mobile token issuance, not half-enable it", tc.name)
			}
		})
	}
}

// The default deployment (nothing set) must be unchanged from today: mobile
// token issuance off, web login untouched.
func TestMobileTokenIssuance_OffByDefault(t *testing.T) {
	if mobileTokenIssuanceEnabled(&config.Config{}) {
		t.Fatal("an unconfigured deployment must not enable mobile token issuance")
	}
}
