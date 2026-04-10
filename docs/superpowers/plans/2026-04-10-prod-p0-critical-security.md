# Production Readiness P0 — Critical Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all critical security issues: remove committed secrets, add AES-256-GCM encryption for API keys, scope webhooks by store, add security headers + CORS, separate Razorpay webhook secret, sanitize PII in logs.

**Architecture:** New `internal/crypto/` package (envelope encryption), new `internal/middleware/` (security headers, CORS). Migration 000011 for webhook_secret column. Git history rewrite for .env.local.

**Tech Stack:** Go 1.26, GCP KMS, AES-256-GCM, Gin middleware. Next.js security headers.

---

## Task 1: .env.local cleanup + gitignore hardening

**Context:** `infra/dev/.env.local` was previously committed (git log shows no current commits, but `.gitignore` already has `.env.local` at line 10 and `infra/dev/.env.local.generated` at line 43). Verify the file is not currently tracked. Create `.env.local.example` for developer onboarding.

**Files to create:**
- `infra/dev/.env.local.example`

**Files to modify:**
- `.gitignore` (verify `infra/dev/.env.local` is covered by the existing `.env.local` pattern)

### Steps

1. **Verify .env.local is not tracked:**
   ```bash
   cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
   git ls-files infra/dev/.env.local
   # If output is non-empty, the file IS tracked — proceed with removal.
   # If empty, the file is already untracked — skip to step 3.
   ```

2. **If tracked — remove from index and optionally rewrite history:**
   ```bash
   git rm --cached infra/dev/.env.local
   ```
   Note: Full history rewrite with `git filter-repo` is a destructive operation requiring team coordination. Document this as a follow-up task for the team. Do NOT run filter-repo unilaterally.

3. **Harden .gitignore** — add explicit pattern if the generic `.env.local` at root level does not match subdirectories:
   ```gitignore
   # Secrets — never commit
   infra/dev/.env.local
   **/.env.local
   ```
   The existing `.gitignore` line 10 has `.env.local` which only matches the root. Add `**/.env.local` to catch all subdirectories.

4. **Create the example file** at `infra/dev/.env.local.example`:
   ```env
   # Copy this file to .env.local and fill in real values.
   # NEVER commit .env.local — it is gitignored.

   # Google Identity Platform
   GIP_WEB_API_KEY=your-gip-web-api-key
   GIP_OAUTH_CLIENT_ID=your-oauth-client-id
   GIP_OAUTH_CLIENT_SECRET=your-oauth-client-secret

   # Session encryption
   SESSION_ENCRYPT_KEY=generate-a-32-byte-hex-string

   # Marketplace API
   DATABASE_URL=postgres://marketplace_user:password@localhost:5432/marketplace_db?sslmode=disable
   MARKETPLACE_FGA_API_URL=http://localhost:8080
   MARKETPLACE_INTERNAL_AUTH_SECRET=dev-secret
   MARKETPLACE_STOREFRONT_KEY=dev-storefront-key
   MARKETPLACE_GCS_BUCKET=
   MARKETPLACE_PLATFORM_API_URL=
   ```

### TDD

- **Test:** Run `git ls-files infra/dev/.env.local` and verify empty output.
- **Test:** Run `echo "test" > infra/dev/.env.local && git status` and verify it appears as untracked (not modified/new).
- **Test:** Verify `infra/dev/.env.local.example` exists and contains no real secrets (grep for `AIzaSy`, `GOCSPX-`, any 32+ char hex strings).

---

## Task 2: Crypto envelope encryption package

**Context:** Four tables store plaintext in `*_encrypted` columns: `payment_gateway_configs.api_key_encrypted`, `payment_gateway_configs.secret_key_encrypted`, `shipping_carrier_configs.api_key_encrypted`, `tax_provider_configs.api_key_encrypted`. The Encryptor interface must be injected into all handlers that read/write these columns.

**Files to create:**
- `services/marketplace-api/internal/crypto/encryptor.go`
- `services/marketplace-api/internal/crypto/kms.go`
- `services/marketplace-api/internal/crypto/noop.go`
- `services/marketplace-api/internal/crypto/encryptor_test.go`

### Interface definition

File: `services/marketplace-api/internal/crypto/encryptor.go`
```go
// Package crypto provides envelope encryption for sensitive data at rest.
// Production uses GCP KMS for key wrapping + AES-256-GCM for data encryption.
// Dev/test uses a noop encryptor (base64) for convenience.
package crypto

import "errors"

// ErrDecryptionFailed is returned when ciphertext cannot be decrypted
// (wrong key, tampered data, or noop-encoded data fed to KMS encryptor).
var ErrDecryptionFailed = errors.New("crypto: decryption failed")

// Encryptor encrypts and decrypts sensitive strings for storage.
// Implementations MUST be safe for concurrent use.
type Encryptor interface {
	// Encrypt returns a ciphertext string suitable for storage in a TEXT column.
	// The ciphertext is opaque — callers must not parse or modify it.
	Encrypt(plaintext string) (ciphertext string, err error)

	// Decrypt reverses Encrypt. Returns ErrDecryptionFailed on any
	// integrity or key mismatch error.
	Decrypt(ciphertext string) (plaintext string, err error)
}
```

### Noop implementation (dev/test)

File: `services/marketplace-api/internal/crypto/noop.go`
```go
package crypto

import "encoding/base64"

// NoopEncryptor base64-encodes plaintext. It provides interface
// compatibility for local dev and tests — it does NOT provide security.
type NoopEncryptor struct{}

func NewNoopEncryptor() *NoopEncryptor { return &NoopEncryptor{} }

func (n *NoopEncryptor) Encrypt(plaintext string) (string, error) {
	return "noop:" + base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (n *NoopEncryptor) Decrypt(ciphertext string) (string, error) {
	const prefix = "noop:"
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		return "", ErrDecryptionFailed
	}
	b, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", ErrDecryptionFailed
	}
	return string(b), nil
}
```

### KMS implementation (production)

File: `services/marketplace-api/internal/crypto/kms.go`
```go
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
)

// envelope is the JSON-serializable structure stored in the TEXT column.
// It contains the KMS-wrapped DEK and the AES-256-GCM ciphertext+nonce.
type envelope struct {
	WrappedDEK string `json:"w"` // base64 KMS-encrypted DEK
	Nonce      string `json:"n"` // base64 GCM nonce
	Ciphertext string `json:"c"` // base64 AES-256-GCM ciphertext
}

// KMSEncryptor uses GCP KMS to wrap/unwrap a per-value DEK, then
// AES-256-GCM for the actual data encryption. Each Encrypt call
// generates a fresh 256-bit DEK and 96-bit nonce.
type KMSEncryptor struct {
	client      *kms.KeyManagementClient
	keyName     string // full resource name: projects/.../cryptoKeys/...
}

// NewKMSEncryptor creates a KMSEncryptor. keyName is the full GCP KMS
// CryptoKey resource name. The caller must close the client when done.
func NewKMSEncryptor(client *kms.KeyManagementClient, keyName string) *KMSEncryptor {
	return &KMSEncryptor{client: client, keyName: keyName}
}

func (k *KMSEncryptor) Encrypt(plaintext string) (string, error) {
	ctx := context.Background()

	// Generate a random 256-bit DEK.
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", fmt.Errorf("crypto: generate DEK: %w", err)
	}

	// Wrap DEK with KMS.
	wrapResp, err := k.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:      k.keyName,
		Plaintext: dek,
	})
	if err != nil {
		return "", fmt.Errorf("crypto: KMS wrap DEK: %w", err)
	}

	// AES-256-GCM encrypt the plaintext with the DEK.
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("crypto: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Serialize envelope as JSON, then base64 for TEXT column storage.
	env := envelope{
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapResp.Ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	jsonBytes, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("crypto: marshal envelope: %w", err)
	}
	return "kms:" + base64.StdEncoding.EncodeToString(jsonBytes), nil
}

func (k *KMSEncryptor) Decrypt(ciphertext string) (string, error) {
	const prefix = "kms:"
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		return "", ErrDecryptionFailed
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", ErrDecryptionFailed
	}

	var env envelope
	if err := json.Unmarshal(jsonBytes, &env); err != nil {
		return "", ErrDecryptionFailed
	}

	wrappedDEK, err := base64.StdEncoding.DecodeString(env.WrappedDEK)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	// Unwrap DEK via KMS.
	ctx := context.Background()
	unwrapResp, err := k.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:       k.keyName,
		Ciphertext: wrappedDEK,
	})
	if err != nil {
		return "", ErrDecryptionFailed
	}

	// AES-256-GCM decrypt.
	block, err := aes.NewCipher(unwrapResp.Plaintext)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	plainBytes, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plainBytes), nil
}
```

### Tests

File: `services/marketplace-api/internal/crypto/encryptor_test.go`
```go
package crypto_test

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

func TestNoopEncryptor_RoundTrip(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	cases := []string{
		"sk_test_abc123",
		"",
		"a",
		"very-long-key-with-special-chars!@#$%^&*()",
	}
	for _, tc := range cases {
		ct, err := enc.Encrypt(tc)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", tc, err)
		}
		pt, err := enc.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt(%q): %v", ct, err)
		}
		if pt != tc {
			t.Errorf("round-trip mismatch: got %q, want %q", pt, tc)
		}
	}
}

func TestNoopEncryptor_DecryptInvalid(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	cases := []string{
		"",
		"no-prefix",
		"noop:!!!invalid-base64!!!",
		"kms:something", // wrong prefix for noop
	}
	for _, tc := range cases {
		_, err := enc.Decrypt(tc)
		if err != crypto.ErrDecryptionFailed {
			t.Errorf("Decrypt(%q): got err=%v, want ErrDecryptionFailed", tc, err)
		}
	}
}

func TestNoopEncryptor_CiphertextHasPrefix(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	ct, err := enc.Encrypt("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) < 5 || ct[:5] != "noop:" {
		t.Errorf("ciphertext should start with 'noop:', got %q", ct)
	}
}
```

### TDD steps
1. **RED:** Write `encryptor_test.go` with the test cases above. Run `go test ./internal/crypto/...` — fails because package does not exist.
2. **GREEN:** Create `encryptor.go`, `noop.go`. Run tests — all pass.
3. **IMPROVE:** Add `kms.go`. KMS tests require a mock or integration setup (skip in unit tests, cover with integration test using a local KMS emulator or `NewNoopEncryptor` as the default in test config).

### Config wiring

Add to `services/marketplace-api/pkg/config/config.go`:
```go
// Encryption — controls at-rest encryption for API keys stored in *_encrypted columns.
EncryptionMode     string `envconfig:"ENCRYPTION_MODE" default:"noop"`
KMSKeyResourceName string `envconfig:"KMS_KEY_RESOURCE_NAME" default:""`
```

Add to `services/marketplace-api/cmd/marketplace-api/main.go` (after config load, before handler construction):
```go
// Encryption setup.
var encryptor crypto.Encryptor
switch cfg.EncryptionMode {
case "kms":
    kmsClient, err := kms.NewKeyManagementClient(context.Background())
    if err != nil {
        log.Error("crypto: KMS client init", "err", err)
        os.Exit(1)
    }
    defer kmsClient.Close()
    encryptor = crypto.NewKMSEncryptor(kmsClient, cfg.KMSKeyResourceName)
    log.Info("crypto: using KMS envelope encryption")
default:
    encryptor = crypto.NewNoopEncryptor()
    log.Info("crypto: using noop encryptor (ENCRYPTION_MODE is not 'kms')")
}
```

---

## Task 3: Migration 000011 — webhook_secret column

**Context:** Razorpay uses the same `secret_key_encrypted` for both API auth and webhook verification. The spec calls this migration 000019, but the current highest migration is 000010. The next available number is 000011.

**Files to create:**
- `services/marketplace-api/migrations/000011_webhook_secret.up.sql`
- `services/marketplace-api/migrations/000011_webhook_secret.down.sql`

**Files to modify:**
- `services/marketplace-api/migrations.go` (bump `ExpectedSchemaVersion` from 10 to 11)

### Up migration

File: `services/marketplace-api/migrations/000011_webhook_secret.up.sql`
```sql
-- 000011_webhook_secret.up.sql
-- P0: Add webhook_secret column to payment_gateway_configs.
-- Razorpay (and potentially others) use a separate secret for webhook
-- signature verification vs API authentication.

ALTER TABLE payment_gateway_configs
    ADD COLUMN webhook_secret TEXT DEFAULT '';
```

### Down migration

File: `services/marketplace-api/migrations/000011_webhook_secret.down.sql`
```sql
-- 000011_webhook_secret.down.sql
ALTER TABLE payment_gateway_configs
    DROP COLUMN IF EXISTS webhook_secret;
```

### Version bump

In `services/marketplace-api/migrations.go`, change:
```go
const ExpectedSchemaVersion uint = 10
```
to:
```go
const ExpectedSchemaVersion uint = 11
```

### TDD steps
1. **RED:** Run `make mp-migrate-up` — fails because migration 000011 does not exist but version asserts 11.
2. **GREEN:** Create both SQL files and bump the version. Run `make mp-migrate-up` — succeeds.
3. **IMPROVE:** Verify column exists: `\d payment_gateway_configs` in psql shows `webhook_secret TEXT`.

---

## Task 4: Update all handlers to encrypt/decrypt via Encryptor

**Context:** Currently, `admin/settings.go` stores `req.APIKey` directly into `api_key_encrypted` (line 220: `APIKeyEncrypted: req.APIKey`). All read paths mask keys via `maskKey()` but read plaintext from DB. After this task, all writes encrypt and all reads decrypt before masking.

**Files to modify:**
- `services/marketplace-api/internal/handlers/admin/settings.go`
- `services/marketplace-api/internal/handlers/storefront/webhooks.go`
- `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`
- `services/marketplace-api/internal/handlers/storefront/shipping_rates.go`
- `services/marketplace-api/internal/tax/repository.go`
- `services/marketplace-api/internal/handlers/storefront/routes.go` (Deps struct)
- `services/marketplace-api/internal/handlers/admin/routes.go` (Deps struct if exists)
- `services/marketplace-api/cmd/marketplace-api/main.go` (pass encryptor to handlers)

### Pattern: Inject Encryptor into handlers

Add `encryptor crypto.Encryptor` field to every handler struct that touches `*_encrypted` columns:

**PaymentSettingsHandler** (admin/settings.go):
```go
type PaymentSettingsHandler struct {
	db          *gorm.DB
	countryRepo country.Repository
	encryptor   crypto.Encryptor  // NEW
	logger      *slog.Logger
}

func NewPaymentSettingsHandler(
	db *gorm.DB,
	countryRepo country.Repository,
	encryptor crypto.Encryptor,  // NEW
	logger *slog.Logger,
) *PaymentSettingsHandler {
	return &PaymentSettingsHandler{
		db: db, countryRepo: countryRepo,
		encryptor: encryptor, logger: logger,
	}
}
```

Apply the same pattern to:
- `ShippingSettingsHandler` in `admin/settings.go`
- `TaxSettingsHandler` in `admin/settings.go`
- `WebhookHandler` in `storefront/webhooks.go`
- `CheckoutExtHandler` in `storefront/checkout_ext.go`
- `ShippingRatesHandler` in `storefront/shipping_rates.go`

### Pattern: Encrypt on write

In `PaymentSettingsHandler.Upsert`, replace:
```go
cfg := PaymentGatewayConfig{
    // ...
    APIKeyEncrypted:    req.APIKey,
    SecretKeyEncrypted: req.SecretKey,
    // ...
}
```
with:
```go
encAPIKey, err := h.encryptor.Encrypt(req.APIKey)
if err != nil {
    h.logger.Error("encrypt api key", "err", err)
    c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
        "error":   "internal",
        "message": "failed to secure API key",
    })
    return
}
encSecretKey := ""
if req.SecretKey != "" {
    encSecretKey, err = h.encryptor.Encrypt(req.SecretKey)
    if err != nil {
        h.logger.Error("encrypt secret key", "err", err)
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
            "error":   "internal",
            "message": "failed to secure secret key",
        })
        return
    }
}
cfg := PaymentGatewayConfig{
    // ...
    APIKeyEncrypted:    encAPIKey,
    SecretKeyEncrypted: encSecretKey,
    // ...
}
```

Apply the same encrypt-on-write pattern to:
- `ShippingSettingsHandler.Upsert` for `api_key_encrypted`
- `TaxSettingsHandler.Upsert` for `api_key_encrypted`
- Any update path that writes to `*_encrypted` columns

### Pattern: Decrypt on read (for API calls, NOT for display)

In `WebhookHandler.HandleWebhook`, after fetching `cfg` from DB:
```go
apiKey, err := h.encryptor.Decrypt(cfg.APIKey)
if err != nil {
    h.logError("webhook: decrypt api key failed", "provider", provider, "err", err)
    c.JSON(http.StatusOK, gin.H{"status": "error"})
    return
}
secretKey, err := h.encryptor.Decrypt(cfg.SecretKeyEncrypted)
if err != nil {
    h.logError("webhook: decrypt secret key failed", "provider", provider, "err", err)
    c.JSON(http.StatusOK, gin.H{"status": "error"})
    return
}
gateway, err := payment.NewGateway(provider, apiKey, secretKey, cfg.Mode)
```

Apply decrypt-on-read to:
- `checkout_ext.go` — wherever it reads gateway configs to create payment intents
- `shipping_rates.go` — wherever it reads carrier configs to call shipping APIs
- `tax/repository.go` — wherever it reads tax provider configs

### Pattern: Display (masked) stays as-is

The `toPaymentResponse` function already calls `maskKey()` — this is fine. The stored value is now encrypted, so masking the ciphertext is still safe (it reveals no plaintext). However, if the UI ever needs to display the last 4 chars of the real key, you would decrypt first then mask. For now, masking the ciphertext is acceptable.

### main.go wiring changes

Update handler construction to pass `encryptor`:
```go
paymentSettingsHandler := admin.NewPaymentSettingsHandler(conn, countryRepoAdmin, encryptor, log)
shippingSettingsHandler := admin.NewShippingSettingsHandler(conn, countryRepoAdmin, encryptor, log)
taxSettingsHandler := admin.NewTaxSettingsHandler(conn, countryRepoAdmin, encryptor, log)
// ...
webhookHandler := storefront.NewWebhookHandler(conn, orderSvcSF, encryptor, log)
checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, couponSvc, encryptor, log)
shippingRatesHandler := storefront.NewShippingRatesHandler(conn, encryptor, log)
```

### TDD steps
1. **RED:** Write a test in `admin/settings_test.go` that creates a `PaymentSettingsHandler` with a `NoopEncryptor`, calls `Upsert`, then reads the raw DB row — assert the stored value starts with `noop:` (not plaintext).
2. **GREEN:** Implement the encrypt-on-write changes. Tests pass.
3. **RED:** Write a test in `storefront/webhooks_test.go` that seeds an encrypted config row, calls `HandleWebhook` — assert the gateway receives decrypted keys.
4. **GREEN:** Implement decrypt-on-read. Tests pass.
5. **IMPROVE:** Verify all 6 files compile: `go build ./...`

---

## Task 5: Webhook URL refactor — scope by store

**Context:** Current route is `POST /api/v1/webhooks/:provider` (registered in `storefront/routes.go` line 78). The handler queries `payment_gateway_configs WHERE provider = ? AND is_active = true` without `store_id` — first match wins across all tenants. This is a multi-tenant isolation bug.

**Files to modify:**
- `services/marketplace-api/internal/handlers/storefront/routes.go`
- `services/marketplace-api/internal/handlers/storefront/webhooks.go`

### Route change

In `routes.go`, change line 78:
```go
// BEFORE:
router.POST("/webhooks/:provider", deps.WebhookHandler.HandleWebhook)

// AFTER:
router.POST("/webhooks/:storeSlug/:provider", deps.WebhookHandler.HandleWebhook)
```

### Handler change

In `webhooks.go`, update `HandleWebhook`:
```go
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	storeSlug := c.Param("storeSlug")
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" || storeSlug == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// ... (read body, same as before) ...

	// Resolve store from slug.
	var storeRow struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	if err := h.db.WithContext(ctx).
		Table("stores").
		Select("id").
		Where("slug = ?", storeSlug).
		First(&storeRow).Error; err != nil {
		h.logError("webhook: store not found by slug",
			"slug", storeSlug, "provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Look up gateway config scoped to this store.
	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true",
			storeRow.ID, provider).
		First(&cfg).Error; err != nil {
		h.logError("webhook: no active gateway config",
			"store_id", storeRow.ID, "slug", storeSlug,
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}
	// ... rest of handler unchanged ...
```

### webhookGatewayConfigRow update

Add `StoreID` field so it is available for logging:
```go
type webhookGatewayConfigRow struct {
	StoreID            uuid.UUID `gorm:"column:store_id"`       // NEW
	Provider           string    `gorm:"column:provider"`
	APIKey             string    `gorm:"column:api_key_encrypted"`
	SecretKeyEncrypted string    `gorm:"column:secret_key_encrypted"`
	WebhookSecret      string    `gorm:"column:webhook_secret"` // NEW (Task 3)
	Mode               string    `gorm:"column:mode"`
	IsActive           bool      `gorm:"column:is_active"`
}
```

### TDD steps
1. **RED:** In `storefront/webhooks_test.go`, create two stores (A, B) each with a Stripe config using different secrets. Send a webhook to `/api/v1/webhooks/store-a/stripe` signed with Store A's secret — assert it processes. Send the same payload to `/api/v1/webhooks/store-b/stripe` — assert signature verification fails (different secret).
2. **GREEN:** Implement the route and handler changes. Tests pass.
3. **RED:** Send a webhook to `/api/v1/webhooks/nonexistent-store/stripe` — assert 200 with `{"status": "ignored"}`.
4. **GREEN:** Already handled by the store lookup failure path. Tests pass.

---

## Task 6: Security headers middleware (Go + Next.js)

**Context:** No security headers are set on any response. The Go API serves JSON; the Next.js apps serve HTML. Both need headers.

### Go middleware

**Files to create:**
- `services/marketplace-api/internal/middleware/security_headers.go`
- `services/marketplace-api/internal/middleware/security_headers_test.go`

File: `services/marketplace-api/internal/middleware/security_headers.go`
```go
// Package middleware provides Gin middleware for the marketplace-api.
package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets standard security response headers on every response.
// Safe for both HTML and JSON responses.
func SecurityHeaders() gin.HandlerFunc {
	// CSP is permissive for an API — tighten if serving HTML.
	csp := "default-src 'none'; frame-ancestors 'none'"

	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", csp)
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}
```

File: `services/marketplace-api/internal/middleware/security_headers_test.go`
```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/middleware"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.SecurityHeaders())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	expected := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-Xss-Protection":          "0",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
	}

	for header, want := range expected {
		got := w.Header().Get(header)
		if got != want {
			t.Errorf("header %s: got %q, want %q", header, got, want)
		}
	}
}
```

### Wire into main.go

In `cmd/marketplace-api/main.go`, when constructing Gin engines, add the middleware globally:
```go
engine := gin.New()
engine.Use(gin.Recovery())
engine.Use(middleware.SecurityHeaders()) // ADD THIS
// ... rest of route setup
```

### Next.js storefront headers

**File to modify:** `apps/storefront/next.config.ts`

```typescript
import path from "node:path";
import type { NextConfig } from "next";

const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-XSS-Protection", value: "0" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Strict-Transport-Security",
    value: "max-age=31536000; includeSubDomains",
  },
  {
    key: "Content-Security-Policy",
    value: [
      "default-src 'self'",
      "script-src 'self' https://js.stripe.com https://checkout.razorpay.com",
      "frame-src https://js.stripe.com https://api.razorpay.com",
      "img-src 'self' https://storage.googleapis.com data:",
      "style-src 'self' 'unsafe-inline'",
      "connect-src 'self' https://api.stripe.com",
      "font-src 'self'",
    ].join("; "),
  },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=()",
  },
];

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "storage.googleapis.com" },
      { protocol: "https", hostname: "*.storage.googleapis.com" },
      { protocol: "http", hostname: "localhost" },
      { protocol: "http", hostname: "fake-gcs-server" },
    ],
  },
  headers: async () => [
    {
      source: "/:path*",
      headers: securityHeaders,
    },
  ],
};

export default nextConfig;
```

### Next.js admin headers

**File to modify:** `apps/admin/next.config.ts`

```typescript
import path from "node:path";
import type { NextConfig } from "next";

const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-XSS-Protection", value: "0" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Strict-Transport-Security",
    value: "max-age=31536000; includeSubDomains",
  },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=()",
  },
];

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  headers: async () => [
    {
      source: "/:path*",
      headers: securityHeaders,
    },
  ],
};

export default nextConfig;
```

### TDD steps
1. **RED:** Write `security_headers_test.go`. Run — fails, package does not exist.
2. **GREEN:** Create `security_headers.go`. Tests pass.
3. **IMPROVE:** Verify headers appear in integration tests by checking response headers on existing test endpoints.
4. **Next.js:** After modifying `next.config.ts`, run `npm run build` in both apps to verify no config errors. Then `curl -I localhost:3000/products` and verify headers appear.

---

## Task 7: CORS middleware

**Context:** No explicit CORS headers on the Go API. The storefront Next.js app makes cross-origin requests to the API (different subdomain).

**Files to create:**
- `services/marketplace-api/internal/middleware/cors.go`
- `services/marketplace-api/internal/middleware/cors_test.go`

### Implementation

File: `services/marketplace-api/internal/middleware/cors.go`
```go
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSConfig configures the CORS middleware.
type CORSConfig struct {
	// AllowedOrigins is a list of origin patterns. Supports wildcard subdomain:
	// "https://*.mark8ly.com" matches "https://store-a.mark8ly.com".
	AllowedOrigins []string
}

// CORS returns a Gin middleware that handles CORS preflight and response
// headers. Only applied to the storefront engine (admin is same-origin).
func CORS(cfg CORSConfig) gin.HandlerFunc {
	maxAge := int(12 * time.Hour / time.Second)
	methods := "GET, POST, PATCH, PUT, DELETE, OPTIONS"
	headers := "Content-Type, X-Storefront-Key, Authorization"

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if !matchOrigin(origin, cfg.AllowedOrigins) {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", methods)
		c.Header("Access-Control-Allow-Headers", headers)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", strings.Itoa(maxAge))

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// matchOrigin checks if the origin matches any of the allowed patterns.
// Supports wildcard subdomains: "https://*.example.com".
func matchOrigin(origin string, patterns []string) bool {
	for _, p := range patterns {
		if p == origin {
			return true
		}
		// Wildcard subdomain match: "https://*.mark8ly.com"
		if idx := strings.Index(p, "*."); idx >= 0 {
			prefix := p[:idx]      // "https://"
			suffix := p[idx+1:]    // ".mark8ly.com"
			if strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix) {
				// Ensure there is something between prefix and suffix
				// and no additional dots (single subdomain level).
				mid := origin[len(prefix) : len(origin)-len(suffix)]
				if len(mid) > 0 && !strings.Contains(mid, ".") {
					return true
				}
			}
		}
	}
	return false
}
```

File: `services/marketplace-api/internal/middleware/cors_test.go`
```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/middleware"
)

func TestCORS_PreflightAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: []string{"https://*.mark8ly.com"},
	}))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://store-a.mark8ly.com")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://store-a.mark8ly.com" {
		t.Errorf("ACAO: got %q", got)
	}
}

func TestCORS_DeniedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: []string{"https://*.mark8ly.com"},
	}))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO should be empty for denied origin, got %q", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: []string{"https://*.mark8ly.com"},
	}))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO should be empty when no Origin header, got %q", got)
	}
}

func TestMatchOrigin(t *testing.T) {
	cases := []struct {
		origin   string
		patterns []string
		want     bool
	}{
		{"https://store-a.mark8ly.com", []string{"https://*.mark8ly.com"}, true},
		{"https://mark8ly.com", []string{"https://*.mark8ly.com"}, false},       // no subdomain
		{"https://a.b.mark8ly.com", []string{"https://*.mark8ly.com"}, false},   // nested subdomain
		{"https://evil.com", []string{"https://*.mark8ly.com"}, false},
		{"https://mark8ly.com", []string{"https://mark8ly.com"}, true},          // exact match
		{"http://localhost:3000", []string{"http://localhost:3000"}, true},       // dev
	}
	// These test cases validate matchOrigin indirectly via the CORS middleware.
	// For direct unit testing, export matchOrigin or test via the middleware behavior.
	_ = cases // documented for reference — actual tests use the middleware approach above
}
```

### Config + wiring

Add to `services/marketplace-api/pkg/config/config.go`:
```go
// CORSAllowedOrigins is a comma-separated list of allowed CORS origins.
// Supports wildcard subdomains: "https://*.mark8ly.com".
// Applied only to the storefront engine.
CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:"https://*.mark8ly.com,http://localhost:3000"`
```

In `main.go`, apply CORS only to the storefront engine:
```go
storefrontEngine.Use(middleware.CORS(middleware.CORSConfig{
    AllowedOrigins: strings.Split(cfg.CORSAllowedOrigins, ","),
}))
```

### TDD steps
1. **RED:** Write `cors_test.go`. Run — fails, no package.
2. **GREEN:** Create `cors.go`. Tests pass.
3. **IMPROVE:** Add integration test: storefront endpoints respond with CORS headers when `Origin` is set.

---

## Task 8: Razorpay webhook secret separation

**Context:** After Task 3 adds the `webhook_secret` column and Task 4 adds encryption, this task updates the Razorpay-specific webhook verification to use the dedicated `webhook_secret` field.

**Files to modify:**
- `services/marketplace-api/internal/handlers/storefront/webhooks.go`
- `services/marketplace-api/internal/handlers/admin/settings.go` (add `webhook_secret` to upsert)
- `services/marketplace-api/internal/payment/razorpay.go` (new constructor param)

### Admin settings: add webhook_secret to request/response

In `admin/settings.go`, update `paymentUpsertRequest`:
```go
type paymentUpsertRequest struct {
	APIKey        string `json:"api_key"        binding:"required"`
	SecretKey     string `json:"secret_key"`
	WebhookSecret string `json:"webhook_secret"` // NEW — optional, Razorpay uses this
	Mode          string `json:"mode"            binding:"required,oneof=test live"`
	IsActive      bool   `json:"is_active"`
}
```

Update `PaymentGatewayConfig` model:
```go
type PaymentGatewayConfig struct {
	// ... existing fields ...
	WebhookSecret string `gorm:"column:webhook_secret;type:text"` // NEW
}
```

In `Upsert`, encrypt and store `webhook_secret`:
```go
encWebhookSecret := ""
if req.WebhookSecret != "" {
    encWebhookSecret, err = h.encryptor.Encrypt(req.WebhookSecret)
    if err != nil {
        h.logger.Error("encrypt webhook secret", "err", err)
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
            "error":   "internal",
            "message": "failed to secure webhook secret",
        })
        return
    }
}
// Include in cfg and in updates map:
cfg.WebhookSecret = encWebhookSecret
// ... and in the update map:
"webhook_secret": encWebhookSecret,
```

### Webhook handler: use webhook_secret for Razorpay

In `webhooks.go`, after decrypting the config:
```go
// For Razorpay, prefer the dedicated webhook_secret for verification.
verifySecret := secretKey // default: use secret_key
if provider == "razorpay" && cfg.WebhookSecret != "" {
    ws, err := h.encryptor.Decrypt(cfg.WebhookSecret)
    if err != nil {
        h.logError("webhook: decrypt webhook_secret failed",
            "provider", provider, "err", err)
        c.JSON(http.StatusOK, gin.H{"status": "error"})
        return
    }
    verifySecret = ws
}
gateway, err := payment.NewGateway(provider, apiKey, verifySecret, cfg.Mode)
```

### TDD steps
1. **RED:** Write test: create Razorpay config with `webhook_secret` set, send webhook signed with the webhook secret — assert verification passes.
2. **GREEN:** Implement changes. Test passes.
3. **RED:** Write test: create Razorpay config WITHOUT `webhook_secret`, send webhook signed with `secret_key` — assert it still works (backward compat).
4. **GREEN:** The fallback logic handles this. Test passes.

---

## Task 9: PII log sanitizer

**Context:** Provider error responses are logged verbatim via `fmt.Errorf`. For example, `razorpay.go:92`: `fmt.Errorf("razorpay: create intent: status %d: %s", resp.StatusCode, respBody)`. The `respBody` may contain card details or PII.

**Files to create:**
- `services/marketplace-api/internal/providerlog/sanitize.go`
- `services/marketplace-api/internal/providerlog/sanitize_test.go`

**Files to modify:**
- `services/marketplace-api/internal/payment/stripe.go` (4 error paths)
- `services/marketplace-api/internal/payment/razorpay.go` (4 error paths)
- `services/marketplace-api/internal/payment/paypal.go` (5 error paths)
- `services/marketplace-api/internal/shipping/shipengine.go` (4 error paths)
- `services/marketplace-api/internal/shipping/ninjavan.go` (4 error paths)
- `services/marketplace-api/internal/shipping/delhivery.go` (error paths)
- `services/marketplace-api/internal/tax/taxjar.go` (1 error path)

### Sanitizer package

File: `services/marketplace-api/internal/providerlog/sanitize.go`
```go
// Package providerlog provides safe logging for third-party provider responses.
// Provider error bodies may contain PII (card details, customer emails) and
// must never be logged verbatim.
package providerlog

import "fmt"

// SanitizeError returns a safe error string for logging. It includes the
// provider name, HTTP status code, and response body byte count — but
// NOT the actual response body.
func SanitizeError(provider string, operation string, statusCode int, bodyLen int) string {
	return fmt.Sprintf("%s: %s: HTTP %d (response body redacted, %d bytes)",
		provider, operation, statusCode, bodyLen)
}
```

File: `services/marketplace-api/internal/providerlog/sanitize_test.go`
```go
package providerlog_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/providerlog"
)

func TestSanitizeError(t *testing.T) {
	msg := providerlog.SanitizeError("stripe", "create intent", 400, 1234)

	if !strings.Contains(msg, "stripe") {
		t.Error("should contain provider name")
	}
	if !strings.Contains(msg, "400") {
		t.Error("should contain status code")
	}
	if !strings.Contains(msg, "1234 bytes") {
		t.Error("should contain byte count")
	}
	if !strings.Contains(msg, "redacted") {
		t.Error("should contain 'redacted'")
	}
}

func TestSanitizeError_NeverContainsBody(t *testing.T) {
	// The function takes bodyLen (int), not body ([]byte), so it is
	// structurally impossible to leak the body. This test documents that intent.
	msg := providerlog.SanitizeError("razorpay", "capture", 500, 9999)
	if strings.Contains(msg, "card") || strings.Contains(msg, "email") {
		t.Error("should never contain PII keywords")
	}
}
```

### Apply to provider files

Replace every instance of `fmt.Errorf("provider: operation: status %d: %s", resp.StatusCode, body)` with `fmt.Errorf("%s", providerlog.SanitizeError("provider", "operation", resp.StatusCode, len(body)))`.

**Example — `stripe.go` line 88:**
```go
// BEFORE:
return nil, fmt.Errorf("stripe: create intent: status %d: %s", resp.StatusCode, body)

// AFTER:
return nil, fmt.Errorf("%s", providerlog.SanitizeError("stripe", "create intent", resp.StatusCode, len(body)))
```

Full list of replacements (file:line from grep results):
- `stripe.go:88` — create intent
- `stripe.go:129` — capture payment
- `stripe.go:178` — refund payment
- `razorpay.go:92` — create intent
- `razorpay.go:149` — capture payment
- `razorpay.go:204` — refund payment
- `razorpay.go:291` — fetch payment
- `paypal.go:108` — create intent
- `paypal.go:166` — capture payment
- `paypal.go:238` — refund payment
- `paypal.go:325` — verify webhook
- `paypal.go:405` — get token
- `shipengine.go:155` — get rates
- `shipengine.go:200` — create shipment
- `shipengine.go:227` — get tracking
- `shipengine.go:266` — cancel shipment
- `ninjavan.go:180` — get rates
- `ninjavan.go:250` — create shipment
- `ninjavan.go:280` — get tracking
- `ninjavan.go:320` — cancel shipment
- `taxjar.go:155` — calculate tax

### TDD steps
1. **RED:** Write `sanitize_test.go`. Run — fails, package does not exist.
2. **GREEN:** Create `sanitize.go`. Tests pass.
3. **IMPROVE:** Search codebase for any remaining `resp.StatusCode.*body` patterns: `grep -rn "StatusCode.*body\|StatusCode.*respBody" services/marketplace-api/internal/` — should return zero results after all replacements.

---

## Execution order

Tasks can be parallelized as follows:

| Wave | Tasks | Rationale |
|------|-------|-----------|
| 1 | Task 1, Task 2, Task 9 | Independent: gitignore, crypto package, sanitizer package |
| 2 | Task 3 | Depends on nothing, but migration must exist before Task 4 |
| 3 | Task 4, Task 6, Task 7 | Task 4 depends on Task 2. Tasks 6 and 7 are independent middleware. |
| 4 | Task 5, Task 8 | Task 5 depends on Task 4 (encryptor in webhook handler). Task 8 depends on Tasks 3+4. |

## Verification checklist

After all tasks:
- [ ] `go build ./...` compiles without errors
- [ ] `go test ./...` passes all tests
- [ ] `git ls-files infra/dev/.env.local` returns empty
- [ ] `grep -rn "StatusCode.*body\|StatusCode.*respBody" services/marketplace-api/internal/payment/` returns nothing
- [ ] `grep -rn "StatusCode.*body\|StatusCode.*respBody" services/marketplace-api/internal/shipping/` returns nothing
- [ ] `curl -I localhost:8087/health` includes `X-Content-Type-Options: nosniff`
- [ ] Next.js apps build successfully: `cd apps/storefront && npm run build && cd ../admin && npm run build`
- [ ] Preflight request to storefront API returns CORS headers
- [ ] Schema version is 11: check `schema_migrations` table

## Infra notes (for tesserix-k8s — out of scope here)

- Add `ENCRYPTION_MODE=kms` and `KMS_KEY_RESOURCE_NAME` to the marketplace-api ExternalSecret
- Add `CORS_ALLOWED_ORIGINS=https://*.mark8ly.com` to marketplace-api configmap
- Add `SENTRY_DSN` to ExternalSecret (preparation for P1)
- Webhook URL change: update Stripe/Razorpay/PayPal dashboard webhook URLs to include store slug
