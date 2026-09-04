package config

import (
	"errors"
	"testing"
)

func validZitadelConfig() Config {
	return Config{
		ZitadelEnabled:          true,
		ZitadelIssuer:           "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken: "pat",
		ZitadelOrgID:            "339070697432875523",
		ZitadelAdminProjectID:   "389070376568619523",
		ZitadelStaffRoleKey:     "mark8ly.staff",
	}
}

func TestValidateZitadel_DisabledIsAlwaysNil(t *testing.T) {
	cfg := Config{ZitadelEnabled: false}
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("ValidateZitadel() with the flag off = %v, want nil", err)
	}
}

func TestValidateZitadel_FullyConfiguredPasses(t *testing.T) {
	cfg := validZitadelConfig()
	if err := cfg.ValidateZitadel(); err != nil {
		t.Fatalf("ValidateZitadel() = %v, want nil", err)
	}
}

func TestValidateZitadel_RefusesOnEachMissingValue(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"issuer", func(c *Config) { c.ZitadelIssuer = "" }, "ZITADEL_ISSUER"},
		{"login client token", func(c *Config) { c.ZitadelLoginClientToken = "" }, "ZITADEL_LOGIN_CLIENT_TOKEN"},
		{"org id", func(c *Config) { c.ZitadelOrgID = "" }, "ZITADEL_ORG_ID"},
		{"admin project id", func(c *Config) { c.ZitadelAdminProjectID = "" }, "ZITADEL_ADMIN_PROJECT_ID"},
		{"staff role key", func(c *Config) { c.ZitadelStaffRoleKey = "" }, "ZITADEL_STAFF_ROLE_KEY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validZitadelConfig()
			tc.mutate(&cfg)
			err := cfg.ValidateZitadel()
			if err == nil {
				t.Fatalf("ValidateZitadel() = nil, want an error mentioning %s", tc.wantSub)
			}
			if !errors.Is(err, ErrZitadelNotConfigured) {
				t.Fatalf("err = %v, want it to wrap ErrZitadelNotConfigured", err)
			}
		})
	}
}
