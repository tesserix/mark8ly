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

// prodEnv sets the minimum env for a non-dev Load, then lets the caller
// override individual vars to assert each fail-closed check in isolation.
func prodEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"DATABASE_URL":                     "postgres://x/y",
		"MARKETPLACE_FGA_API_URL":          "http://openfga:8080",
		"ENV":                              "prod",
		"MARKETPLACE_INTERNAL_AUTH_SECRET": "internal-secret",
		"CUSTOMER_SESSION_SECRET":          "customer-secret",
		"ENCRYPTION_MODE":                  "aes",
		"ENCRYPTION_KEY":                   "0123456789abcdef0123456789abcdef",
	} {
		t.Setenv(k, v)
	}
}

func TestLoad_ProdRequiresInternalAuthSecret(t *testing.T) {
	prodEnv(t)
	t.Setenv("MARKETPLACE_INTERNAL_AUTH_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with empty MARKETPLACE_INTERNAL_AUTH_SECRET in prod = nil, want error")
	}
}

func TestLoad_ProdRequiresCustomerSessionSecret(t *testing.T) {
	prodEnv(t)
	t.Setenv("CUSTOMER_SESSION_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with empty CUSTOMER_SESSION_SECRET in prod = nil, want error")
	}
}

func TestLoad_ProdRejectsNoopEncryption(t *testing.T) {
	prodEnv(t)
	t.Setenv("ENCRYPTION_MODE", "noop")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with ENCRYPTION_MODE=noop in prod = nil, want error")
	}
}

func TestLoad_ProdRequiresEncryptionKeyForAES(t *testing.T) {
	prodEnv(t)
	t.Setenv("ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() with ENCRYPTION_MODE=aes and empty ENCRYPTION_KEY = nil, want error")
	}
}

func TestLoad_ProdSucceedsWhenFullyConfigured(t *testing.T) {
	prodEnv(t)
	if _, err := Load(); err != nil {
		t.Fatalf("Load() with a complete prod config: %v", err)
	}
}

func TestLoad_DevToleratesEmptySecrets(t *testing.T) {
	prodEnv(t)
	t.Setenv("ENV", "dev")
	t.Setenv("MARKETPLACE_INTERNAL_AUTH_SECRET", "")
	t.Setenv("CUSTOMER_SESSION_SECRET", "")
	t.Setenv("ENCRYPTION_MODE", "noop")
	t.Setenv("ENCRYPTION_KEY", "")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() in dev with empty secrets: %v", err)
	}
}
