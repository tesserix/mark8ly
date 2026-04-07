package config

import (
	"os"
	"testing"
)

// clearAll unsets every env var the Config might read so each test starts
// from a known-empty state. envconfig treats "unset" and "empty string"
// differently for typed fields and `required:"true"`, so we have to truly
// unset them, not just set them to "".
func clearAll(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENV", "HTTP_PORT", "DATABASE_URL",
		"FGA_API_URL", "FGA_STORE_ID",
		"SENDGRID_API_KEY", "EMAIL_FROM", "GCS_BUCKET",
	} {
		os.Unsetenv(k)
	}
}

func TestLoad_ReadsRequiredFieldsFromEnv(t *testing.T) {
	clearAll(t)
	t.Setenv("DATABASE_URL", "postgres://test/test")
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("ENV", "test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://test/test" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://test/test")
	}
	if cfg.HTTPPort != 9999 {
		t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, 9999)
	}
	if cfg.Env != "test" {
		t.Errorf("Env = %q, want %q", cfg.Env, "test")
	}
}

func TestLoad_FailsWhenRequiredFieldMissing(t *testing.T) {
	clearAll(t)
	// DATABASE_URL is intentionally not set.
	_, err := Load()
	if err == nil {
		t.Error("Load() should fail when DATABASE_URL is unset (envconfig required)")
	}
}

func TestLoad_AppliesDefaultsForOptionalFields(t *testing.T) {
	clearAll(t)
	t.Setenv("DATABASE_URL", "postgres://test/test")
	// Leave HTTP_PORT, ENV, FGA_API_URL unset so envconfig applies the defaults.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPPort != 8086 {
		t.Errorf("HTTPPort default = %d, want %d", cfg.HTTPPort, 8086)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env default = %q, want %q", cfg.Env, "dev")
	}
	if cfg.FGAAPIURL != "http://openfga:8080" {
		t.Errorf("FGAAPIURL default = %q, want %q", cfg.FGAAPIURL, "http://openfga:8080")
	}
}
