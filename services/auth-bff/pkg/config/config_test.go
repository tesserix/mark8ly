package config

import (
	"os"
	"testing"
)

// clearAll unsets every env var the Config might read so each test starts
// from a known-empty state. envconfig treats "unset" and "empty string"
// differently, so we have to truly unset them.
func clearAll(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENV", "HTTP_PORT", "DATABASE_URL",
		"GIP_PROJECT_ID", "GIP_PROJECT_NUMBER", "GIP_WEB_API_KEY",
		"GIP_INTERNAL_TENANT_ID", "GIP_CUSTOMER_TENANT_ID", "GIP_PLATFORM_TENANT_ID",
		"OAUTH_CLIENT_ID", "OAUTH_CLIENT_SECRET",
		"SESSION_COOKIE_NAME", "SESSION_COOKIE_DOMAIN", "SESSION_ENCRYPT_KEY",
		"FGA_API_URL", "FGA_STORE_ID",
		"ZITADEL_ENABLED", "ZITADEL_ISSUER", "ZITADEL_LOGIN_CLIENT_TOKEN",
		"ZITADEL_ADMIN_PROJECT_ID", "ZITADEL_STOREFRONT_PROJECT_ID",
	} {
		os.Unsetenv(k)
	}
}

// setRequiredEnv populates every required field with a deterministic test value.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	clearAll(t)
	t.Setenv("DATABASE_URL", "postgres://test/test")
	t.Setenv("GIP_PROJECT_ID", "test-project")
	t.Setenv("GIP_PROJECT_NUMBER", "12345")
	t.Setenv("GIP_WEB_API_KEY", "test-key")
	t.Setenv("GIP_INTERNAL_TENANT_ID", "MP-Internal-test")
	t.Setenv("OAUTH_CLIENT_ID", "test-client-id")
	t.Setenv("OAUTH_CLIENT_SECRET", "test-client-secret")
	t.Setenv("SESSION_ENCRYPT_KEY", "thirtytwo-bytes-for-testing-only")
}

func TestLoad_ReadsRequiredFieldsFromEnv(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GIPProjectID != "test-project" {
		t.Errorf("GIPProjectID = %q, want %q", cfg.GIPProjectID, "test-project")
	}
	if cfg.OAuthClientSecret != "test-client-secret" {
		t.Errorf("OAuthClientSecret = %q, want %q", cfg.OAuthClientSecret, "test-client-secret")
	}
}

func TestLoad_FailsWhenGIPProjectIDMissing(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("GIP_PROJECT_ID")

	_, err := Load()
	if err == nil {
		t.Error("Load() should fail when GIP_PROJECT_ID is unset")
	}
}

func TestLoad_FailsWhenSessionEncryptKeyMissing(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("SESSION_ENCRYPT_KEY")

	_, err := Load()
	if err == nil {
		t.Error("Load() should fail when SESSION_ENCRYPT_KEY is unset — auth-bff cannot mint sessions without it")
	}
}

func TestLoad_AppliesDefaultsForOptionalFields(t *testing.T) {
	setRequiredEnv(t)
	// HTTP_PORT, ENV, SESSION_COOKIE_NAME left at their unset state by clearAll().

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort default = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env default = %q, want %q", cfg.Env, "dev")
	}
	if cfg.SessionCookieName != "m8_session" {
		t.Errorf("SessionCookieName default = %q, want %q", cfg.SessionCookieName, "m8_session")
	}
}

func TestZitadelIsDisabledAndUnrequiredByDefault(t *testing.T) {
	for _, k := range []string{"ZITADEL_ENABLED", "ZITADEL_ISSUER", "ZITADEL_LOGIN_CLIENT_TOKEN", "ZITADEL_ADMIN_PROJECT_ID", "ZITADEL_STOREFRONT_PROJECT_ID"} {
		os.Unsetenv(k)
	}
	// Only the pre-existing required vars are set.
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("GIP_PROJECT_ID", "p")
	t.Setenv("GIP_PROJECT_NUMBER", "1")
	t.Setenv("GIP_WEB_API_KEY", "k")
	t.Setenv("GIP_INTERNAL_TENANT_ID", "t")
	t.Setenv("OAUTH_CLIENT_ID", "c")
	t.Setenv("OAUTH_CLIENT_SECRET", "s")
	t.Setenv("SESSION_ENCRYPT_KEY", "thirtytwo-bytes-for-testing-only")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load must succeed with no Zitadel config at all: %v", err)
	}
	if cfg.ZitadelEnabled {
		t.Error("ZitadelEnabled must default to false")
	}
}
