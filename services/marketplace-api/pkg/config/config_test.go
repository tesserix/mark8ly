package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Setenv("MARKETPLACE_FGA_API_URL", "http://openfga:8080")
	defer os.Unsetenv("MARKETPLACE_FGA_API_URL")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no DATABASE_URL = nil, want error")
	}
}

func TestLoad_RequiresFGAAPIURL(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x/y")
	defer os.Unsetenv("DATABASE_URL")
	os.Unsetenv("MARKETPLACE_FGA_API_URL")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no MARKETPLACE_FGA_API_URL = nil, want error")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x/y")
	defer os.Unsetenv("DATABASE_URL")
	os.Setenv("MARKETPLACE_FGA_API_URL", "http://openfga:8080")
	defer os.Unsetenv("MARKETPLACE_FGA_API_URL")
	os.Unsetenv("ENV")
	os.Unsetenv("MODE")
	os.Unsetenv("HTTP_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.Mode != "both" {
		t.Errorf("Mode = %q, want both", cfg.Mode)
	}
	if cfg.HTTPPort != 8087 {
		t.Errorf("HTTPPort = %d, want 8087", cfg.HTTPPort)
	}
}

func TestLoad_LoadsShippingCarrierCredentials(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x/y")
	defer os.Unsetenv("DATABASE_URL")
	os.Setenv("MARKETPLACE_FGA_API_URL", "http://openfga:8080")
	defer os.Unsetenv("MARKETPLACE_FGA_API_URL")

	os.Setenv("SHIPENGINE_CARRIER_ACCOUNT_IE", "se-ie-acct-123")
	defer os.Unsetenv("SHIPENGINE_CARRIER_ACCOUNT_IE")
	os.Setenv("SHIPENGINE_CARRIER_ACCOUNT_NZ", "se-nz-acct-456")
	defer os.Unsetenv("SHIPENGINE_CARRIER_ACCOUNT_NZ")
	os.Setenv("NINJAVAN_VN_API_KEY", "nv-vn-key")
	defer os.Unsetenv("NINJAVAN_VN_API_KEY")
	os.Setenv("NINJAVAN_VN_CLIENT_ID", "nv-vn-cid")
	defer os.Unsetenv("NINJAVAN_VN_CLIENT_ID")
	os.Setenv("NINJAVAN_VN_CLIENT_SECRET", "nv-vn-secret")
	defer os.Unsetenv("NINJAVAN_VN_CLIENT_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShipEngineCarrierAccountIE != "se-ie-acct-123" {
		t.Errorf("ShipEngineCarrierAccountIE = %q, want se-ie-acct-123", cfg.ShipEngineCarrierAccountIE)
	}
	if cfg.ShipEngineCarrierAccountNZ != "se-nz-acct-456" {
		t.Errorf("ShipEngineCarrierAccountNZ = %q, want se-nz-acct-456", cfg.ShipEngineCarrierAccountNZ)
	}
	if cfg.NinjaVanVNAPIKey != "nv-vn-key" {
		t.Errorf("NinjaVanVNAPIKey = %q, want nv-vn-key", cfg.NinjaVanVNAPIKey)
	}
	if cfg.NinjaVanVNClientID != "nv-vn-cid" {
		t.Errorf("NinjaVanVNClientID = %q, want nv-vn-cid", cfg.NinjaVanVNClientID)
	}
	if cfg.NinjaVanVNClientSecret != "nv-vn-secret" {
		t.Errorf("NinjaVanVNClientSecret = %q, want nv-vn-secret", cfg.NinjaVanVNClientSecret)
	}
}
