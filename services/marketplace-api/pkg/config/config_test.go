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

// baseEnv sets only the two hard-required vars, leaving everything else
// (including SHIPPING_SECRET_STORE) at its default — used by the
// SHIPPING_SECRET_STORE tests below so they exercise defaulting in
// isolation from the prod fail-closed checks.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("MARKETPLACE_FGA_API_URL", "http://openfga:8080")
}

// The default is unchanged: an unset SHIPPING_SECRET_STORE is still
// "inline", so merging this PR cannot alter any deployment's behaviour.
func TestConfig_ShippingSecretStoreDefaultUnchanged(t *testing.T) {
	baseEnv(t)
	os.Unsetenv("SHIPPING_SECRET_STORE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShippingSecretStore != "inline" {
		t.Errorf("ShippingSecretStore = %q, want inline", cfg.ShippingSecretStore)
	}
}

// "bao" is accepted as a third valid mode.
func TestConfig_ShippingSecretStoreAcceptsBao(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "bao")
	t.Setenv("OPENBAO_ROLE", "marketplace-api")
	t.Setenv("GCP_PROJECT_ID", "test-project")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with SHIPPING_SECRET_STORE=bao, OPENBAO_ROLE and GCP_PROJECT_ID set: %v", err)
	}
	if cfg.ShippingSecretStore != "bao" {
		t.Errorf("ShippingSecretStore = %q, want bao", cfg.ShippingSecretStore)
	}
}

// An unknown value is rejected at startup, not silently coerced — a typo
// must not quietly leave the wrong backend primary.
func TestConfig_ShippingSecretStoreRejectsUnknownValue(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "totally-bogus")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with SHIPPING_SECRET_STORE=totally-bogus = nil, want error")
	}
}

// Selecting bao without OPENBAO_ROLE is a startup error, since Kubernetes
// login cannot work without it.
func TestConfig_BaoModeRequiresRole(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "bao")
	t.Setenv("OPENBAO_ROLE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with SHIPPING_SECRET_STORE=bao and no OPENBAO_ROLE = nil, want error")
	}
}

// Bonus coverage for the carry-forward risk called out in the task brief:
// carriersecrets.BaoPath hardcodes the "kv/" mount prefix, so a non-"kv"
// OPENBAO_KV_MOUNT must fail at boot, not at the first credential save.
func TestConfig_BaoModeRejectsNonKVMount(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "bao")
	t.Setenv("OPENBAO_ROLE", "marketplace-api")
	t.Setenv("OPENBAO_KV_MOUNT", "secret")
	t.Setenv("GCP_PROJECT_ID", "test-project")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with SHIPPING_SECRET_STORE=bao and OPENBAO_KV_MOUNT=secret = nil, want error")
	}
}

// bao mode still routes legacy gsm:// reads/destroys through GCP Secret
// Manager, so GCP_PROJECT_ID is a genuine prerequisite for "bao" mode too,
// not just "gcpsm" — selecting bao without it is a startup error.
// GCP_PROJECT_ID used to be REQUIRED in bao mode, on the reasoning that a
// bao-primary ChainStore still routed legacy gsm:// rows through GCP Secret
// Manager. mark8ly#621 removed the GCP backend entirely, so that reasoning
// no longer holds and the variable is dead input.
//
// This must be asserted before the env var is removed from the k8s
// deployments, not after: while Validate still demanded it, deleting the
// variable from the cluster would have crashlooped both engines at boot.
// That is the #610 failure mode in reverse — adding a required variable
// means "cluster first, then code", but removing one means "code first,
// then cluster".
func TestConfig_BaoModeNoLongerRequiresGCPProjectID(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "bao")
	t.Setenv("OPENBAO_ROLE", "marketplace-api")
	t.Setenv("GCP_PROJECT_ID", "")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() with SHIPPING_SECRET_STORE=bao and no GCP_PROJECT_ID = %v, want nil", err)
	}
}

// Rollback safety: ChainStore.Get/Destroy route a bao:// reference to
// OpenBao BY PREFIX regardless of which backend is primary (see
// chain.go). So a deployment rolled back from "bao" to "gcpsm" after any
// row has already migrated still needs a working OPENBAO_ROLE to resolve
// it — "gcpsm" mode must require it too, not only "bao" mode, or the
// rollback silently breaks checkout/shipping/webhooks for every migrated
// tenant instead of restoring service.
func TestConfig_GCPSMModeAlsoRequiresOpenBaoRole(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "gcpsm")
	t.Setenv("GCP_PROJECT_ID", "test-project")
	t.Setenv("OPENBAO_ROLE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with SHIPPING_SECRET_STORE=gcpsm and no OPENBAO_ROLE = nil, want error")
	}
}

// Same rollback-safety property, for OPENBAO_ADDR: an explicitly-empty
// address must also fail boot in "gcpsm" mode, since ChainStore still
// needs somewhere to dial for any already-migrated bao:// row.
func TestConfig_GCPSMModeAlsoRequiresOpenBaoAddr(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "gcpsm")
	t.Setenv("GCP_PROJECT_ID", "test-project")
	t.Setenv("OPENBAO_ROLE", "marketplace-api")
	t.Setenv("OPENBAO_ADDR", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with SHIPPING_SECRET_STORE=gcpsm and empty OPENBAO_ADDR = nil, want error")
	}
}

// An explicitly-empty SHIPPING_SECRET_STORE (set but blank, as opposed to
// unset) must be treated as "inline", not rejected as an unknown value —
// envconfig's `default` tag only fires when the var is UNSET. Every chart
// renders `| default "inline"` today so this is not a live risk, but it is
// a startup-crash trap worth closing.
func TestConfig_ShippingSecretStoreExplicitlyEmptyDefaultsToInline(t *testing.T) {
	baseEnv(t)
	t.Setenv("SHIPPING_SECRET_STORE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with SHIPPING_SECRET_STORE=\"\" (explicitly set, empty) = %v, want success", err)
	}
	if cfg.ShippingSecretStore != "inline" {
		t.Errorf("ShippingSecretStore = %q, want inline", cfg.ShippingSecretStore)
	}
}

// ZITADEL_ENABLED defaults to false, so an env that never mentions Zitadel
// at all must load exactly as before — no issuer/audience required.
func TestConfig_ZitadelDisabledByDefault(t *testing.T) {
	baseEnv(t)
	os.Unsetenv("ZITADEL_ENABLED")
	os.Unsetenv("ZITADEL_ISSUER")
	os.Unsetenv("ZITADEL_ADMIN_PROJECT_ID")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with Zitadel env unset = %v, want success", err)
	}
	if cfg.ZitadelEnabled {
		t.Error("ZitadelEnabled = true, want false (must default off)")
	}
}

// ZITADEL_ENABLED=true without an issuer must fail boot, not silently
// leave mobile admin routes on GIP or half-mounted.
func TestConfig_ZitadelEnabledRequiresIssuer(t *testing.T) {
	baseEnv(t)
	t.Setenv("ZITADEL_ENABLED", "true")
	t.Setenv("ZITADEL_ISSUER", "")
	t.Setenv("ZITADEL_ADMIN_PROJECT_ID", "389070376568619523")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with ZITADEL_ENABLED=true and no ZITADEL_ISSUER = nil, want error")
	}
}

// ZITADEL_ENABLED=true without the audience (project id) must also fail
// boot — NewZitadelVerifier treats the audience as required, and an
// unpinned audience would let a storefront-shopper token pass as an
// admin credential.
func TestConfig_ZitadelEnabledRequiresAdminProjectID(t *testing.T) {
	baseEnv(t)
	t.Setenv("ZITADEL_ENABLED", "true")
	t.Setenv("ZITADEL_ISSUER", "https://auth.tesserix.app")
	t.Setenv("ZITADEL_ADMIN_PROJECT_ID", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with ZITADEL_ENABLED=true and no ZITADEL_ADMIN_PROJECT_ID = nil, want error")
	}
}

// Fully configured Zitadel must load cleanly, in dev or otherwise —
// ValidateZitadel is checked unconditionally, unlike the prod-only
// secrets gate.
func TestConfig_ZitadelEnabledSucceedsWhenFullyConfigured(t *testing.T) {
	baseEnv(t)
	t.Setenv("ZITADEL_ENABLED", "true")
	t.Setenv("ZITADEL_ISSUER", "https://auth.tesserix.app")
	t.Setenv("ZITADEL_ADMIN_PROJECT_ID", "389070376568619523")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with Zitadel fully configured = %v, want success", err)
	}
	if !cfg.ZitadelEnabled {
		t.Error("ZitadelEnabled = false, want true")
	}
	if cfg.ZitadelIssuer != "https://auth.tesserix.app" {
		t.Errorf("ZitadelIssuer = %q, want https://auth.tesserix.app", cfg.ZitadelIssuer)
	}
	if cfg.ZitadelAdminProjectID != "389070376568619523" {
		t.Errorf("ZitadelAdminProjectID = %q, want 389070376568619523", cfg.ZitadelAdminProjectID)
	}
}

// LoadCarrierSecretJob is the narrow loader for background jobs (e.g.
// cmd/refund-sweep-cron) that only need to construct a carrier secret
// Store — it must accept a minimal env with none of Config's unrelated
// required/prod-only fields set.
func TestLoadCarrierSecretJob_MinimalEnvSucceeds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	os.Unsetenv("MARKETPLACE_FGA_API_URL")
	os.Unsetenv("SHIPPING_SECRET_STORE")
	os.Unsetenv("ENV")

	cfg, err := LoadCarrierSecretJob()
	if err != nil {
		t.Fatalf("LoadCarrierSecretJob: %v", err)
	}
	if cfg.DatabaseURL != "postgres://x/y" {
		t.Errorf("DatabaseURL = %q, want postgres://x/y", cfg.DatabaseURL)
	}
	if cfg.ShippingSecretStore != "inline" {
		t.Errorf("LoadCarrierSecretJob: ShippingSecretStore = %q, want inline", cfg.ShippingSecretStore)
	}
}

// It must NOT require MARKETPLACE_FGA_API_URL — that's the whole point:
// Load()/Config requires it unconditionally, but a carrier-secret job
// never touches FGA.
func TestLoadCarrierSecretJob_DoesNotRequireFGAAPIURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	os.Unsetenv("MARKETPLACE_FGA_API_URL")

	if _, err := LoadCarrierSecretJob(); err != nil {
		t.Fatalf("LoadCarrierSecretJob() with no MARKETPLACE_FGA_API_URL = %v, want success", err)
	}
}

func TestLoadCarrierSecretJob_RequiresDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")

	if _, err := LoadCarrierSecretJob(); err == nil {
		t.Fatal("LoadCarrierSecretJob() with no DATABASE_URL = nil, want error")
	}
}

// The same validateShippingSecretStore() the full Load() uses must reject
// an unknown SHIPPING_SECRET_STORE for the narrow loader too — this is
// the one piece of validation that must never drift between callers.
func TestLoadCarrierSecretJob_RejectsUnknownShippingSecretStore(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("SHIPPING_SECRET_STORE", "totally-bogus")

	if _, err := LoadCarrierSecretJob(); err == nil {
		t.Fatal("LoadCarrierSecretJob() with SHIPPING_SECRET_STORE=totally-bogus = nil, want error")
	}
}

// Same rollback-safety rule Load() enforces: "gcpsm" (and "bao") require
// OPENBAO_ROLE.
func TestLoadCarrierSecretJob_GCPSMModeRequiresOpenBaoRole(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("SHIPPING_SECRET_STORE", "gcpsm")
	t.Setenv("GCP_PROJECT_ID", "test-project")
	t.Setenv("OPENBAO_ROLE", "")

	if _, err := LoadCarrierSecretJob(); err == nil {
		t.Fatal("LoadCarrierSecretJob() with SHIPPING_SECRET_STORE=gcpsm and no OPENBAO_ROLE = nil, want error")
	}
}

func TestLoadCarrierSecretJob_BaoModeSucceedsWithFullSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("SHIPPING_SECRET_STORE", "bao")
	t.Setenv("OPENBAO_ROLE", "refund-sweep-cron")
	t.Setenv("GCP_PROJECT_ID", "test-project")

	cfg, err := LoadCarrierSecretJob()
	if err != nil {
		t.Fatalf("LoadCarrierSecretJob() with SHIPPING_SECRET_STORE=bao and required settings: %v", err)
	}
	if cfg.ShippingSecretStore != "bao" {
		t.Errorf("ShippingSecretStore = %q, want bao", cfg.ShippingSecretStore)
	}
}
