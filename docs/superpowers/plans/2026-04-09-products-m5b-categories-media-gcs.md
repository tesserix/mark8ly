# Products M5b — Categories, Media, Variant PATCH, Real GCS, Real Platform Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining admin HTTP surface from spec §6.1 — categories CRUD (4 routes), variants quick-PATCH (1 route), media lifecycle including signed upload URLs (4 routes) — and replace the two stub integrations from M5a (FakeUploader, stubPlatformClient) with real GCS and real platform-api HTTP client implementations behind config-gated constructors so local dev still works without the external systems.

**Architecture:** Four new admin handler methods live in `internal/handlers/admin/` alongside the M5a ProductHandler, each wired to the M3 service layer (with a handful of new thin service methods for single-media and variant-basics updates). A new `internal/media/gcs.go` adds a `GCSUploader` that implements the existing `media.Uploader` interface plus a `SignedUploadURL(key, contentType, expires)` method used exclusively by the new `/media/upload-url` endpoint. A new `internal/stores/platform_http.go` adds a real HTTP client satisfying `stores.Client` and reading the `{"data": {...}}` envelope from platform-api's existing `GET /internal/stores/:id` endpoint. Both real clients are constructed conditionally in `cmd/marketplace-api/main.go` — when the corresponding env var is empty the old fake/stub stays wired so local dev keeps working exactly like M5a.

**Tech Stack:** Go 1.26, Gin (existing), `cloud.google.com/go/storage` (new direct dep), stdlib `net/http` for the platform client, the M3+M4+M5a packages already in the module.

---

## Status

**Status: ✅ COMPLETE** — all tasks merged to main.

---

## Scope check

Single contained slice inside the existing `services/marketplace-api` Go module. No migrations, no schema changes, no go.work edits. Adds two new files under `internal/media/`, one under `internal/stores/`, four new handler methods (category/variant/media handlers), and wires everything into `cmd/marketplace-api/main.go`. Helm chart changes are **documented in the PR description only** — the actual chart edits live in the separate `tesserix-k8s` repo and ship as a follow-up ops task, not a code change in this repo.

Spec sections authoritative for this milestone:

- §6.1 — admin routes (categories, variants, media subsets)
- §6.3 — admin DTO families (categories + media)
- §13.3 / §14.9 — pre-tx GCS HEAD validation (moves from stub to real)
- §13.1.1 — `RequireTenantRelation` permission map (staff/admin/owner) per route — already in M4/M5a; M5b adds the category and media rows
- §13.1.4 / §14.7 — StoreMiddleware (already mounted; M5b swaps its Client from stub to real)
- §14.10 — orphan sweep + bucket lifecycle (not implemented here; the GCS uploader just needs to support the content-addressed key layout so the sweep job can find unreferenced objects — documented but not coded)

**Closes M5's exit criterion**: "curl-able admin API complete". After M5b merges, every route in spec §6.1 is live, typed, validated, and authz-gated.

---

## Out of scope

- Full GCS integration test against a real bucket or emulator. The real GCSUploader ships with a compile-time check only; tests continue to use `media.FakeUploader`. The GCS code path is exercised manually at deploy time by hitting the actual /media/upload-url endpoint against a real bucket. A proper emulator-based integration test ships in slice 2.
- Orphan sweep job (§14.10) — separate cron job deliverable, tracked in follow-ups.
- Helm chart updates — document env vars in the PR description; the chart ships as a follow-up PR in `tesserix-k8s`.
- Category tree traversal helpers — M5b exposes categories as a flat list (what `category.Repository.ListByStore` already returns). The tree UI assembly happens in M7 frontend code, not in the API.
- Storefront routes (M6).

---

## Decisions locked

1. **Config-gated real client constructors.** Both `NewGCSUploader(ctx, bucket)` and `NewPlatformClient(baseURL, secret, httpClient)` return the new types. In `main.go`, if `cfg.GCSBucket == ""` keep `media.NewFakeUploader()`; if `cfg.PlatformAPIURL == ""` keep the `stubPlatformClient` from M5a. Both new env vars default to empty, so local dev and existing CI continue to work with the fakes. The Helm chart populates both in production.
2. **Signed upload URL generator lives on the GCSUploader, not a separate interface.** The `/media/upload-url` handler type-asserts the uploader to a `SignedURLGenerator` interface (a new one-method interface). If the uploader is the fake (no signed-URL capability), the handler returns 501 Not Implemented. That keeps the fake implementation small and clearly signals "this endpoint requires real GCS".
3. **Content-addressed storage keys are computed by the SERVER** in the upload-url response, not by the frontend. This differs from §14.9's phrasing ("Storage keys are content-addressed... computed by the frontend"). Reason: the server owns the format and we don't want a frontend bug to produce misaligned paths. The frontend POSTs `{content_hash, filename, content_type}` to `/media/upload-url`, server computes `tenants/<tid>/products/media/<hash>/<filename>`, returns `{url, storage_key, expires_at}`. The frontend uploads and then POSTs `/products/:id/media` with the same storage_key. This simplifies the audit trail and is invisible to the frontend if we document it.
4. **Variant quick-PATCH is narrow.** Only these fields are updatable: `price`, `compare_at_price`, `cost_price`, `sku`, `barcode`, `weight_grams`, `inventory_quantity`, `inventory_policy`, `low_stock_threshold`, `position`. `currency_code` is rejected with `CurrencyChangeForbidden` (§14.2). Option-value assignments are NOT mutable via this endpoint; full variant matrix changes go through product PATCH (which M5a's handler still TODOs for M5b+).
5. **Single-media add/update/delete use their own service methods.** M3's `ReplaceMediaInTx` was the full-set replace. M5b adds `AddMedia(productID, media)`, `UpdateMedia(productID, mediaID, fields)`, `DeleteMedia(productID, mediaID)` to the product service. Each opens its own tx and enqueues one outbox event (`product.updated`, with `store_id` in the payload per the publisher invariant).
6. **Category handlers call the existing M3 category service directly.** `category.Service.Create/Update/Delete` already exist and enqueue outbox events. The handler layer is a thin DTO ↔ service mapper. No new service methods needed for categories.
7. **Variant PATCH reuses the existing `variant_stock` trigger for inventory.** When the request includes `inventory_quantity`, the service writes `variant_stock` (not `product_variants.inventory_quantity`) and lets the trigger propagate. This matches the M3 invariant and the §14.5 reverse-direction guard.
8. **Stub platform client stays in the repo** as `stubPlatformClient` in main.go for local-dev fallback. M5b does NOT delete it — removing it would break `make dev` for contributors who don't run platform-api locally. Slice 2 can delete it when we have a local platform-api docker-compose.
9. **New GCS dep is `cloud.google.com/go/storage`.** Latest minor version. No version pin unless the global `go.work` resolves it to a version that breaks. `go mod tidy` after `go get` must touch ONLY the new transitive deps for storage; if anything else cascades, stop and report DONE_WITH_CONCERNS (landmine — see M3 Task 1 cascade incident).
10. **API integration tests continue to use FakeUploader.** The real GCS path is compile-verified but not tested end-to-end in CI. The tradeoff: faster CI, no emulator dependency. Deploy-time smoke tests via `curl` against a real bucket are the validation path.

---

## File structure produced by M5b

```
services/marketplace-api/
├── cmd/marketplace-api/main.go                       MODIFIED: conditional real-client construction + mount new routes
├── pkg/config/config.go                              MODIFIED: add MARKETPLACE_GCS_BUCKET, MARKETPLACE_PLATFORM_API_URL, MARKETPLACE_PLATFORM_API_SECRET
├── pkg/config/config_test.go                         MODIFIED
└── internal/
    ├── media/
    │   ├── gcs.go                                    NEW: GCSUploader + SignedURLGenerator interface + key-format helper
    │   └── gcs_test.go                               NEW: unit tests for key-format and the SignedURLGenerator interface check
    ├── stores/
    │   ├── platform_http.go                          NEW: HTTPClient satisfies stores.Client via GET /internal/stores/:id
    │   └── platform_http_test.go                     NEW: httptest-based tests for success, 404, envelope shape, auth header
    ├── product/
    │   └── service_single_media.go                   NEW: AddMedia, UpdateMedia, DeleteMedia service methods + their tests
    ├── product/
    │   └── service_variant_patch.go                  NEW: UpdateVariantBasics service method + tests
    └── handlers/admin/
        ├── categories.go                             NEW: CategoryHandler + 4 methods (List, Create, Patch, Delete)
        ├── categories_integration_test.go            NEW: API tests for categories lifecycle
        ├── media.go                                  NEW: MediaHandler + 4 methods (UploadURL, Create, Patch, Delete)
        ├── media_integration_test.go                 NEW: API tests for media lifecycle (using FakeUploader)
        ├── variants.go                               NEW: VariantHandler + 1 method (Patch)
        ├── variants_integration_test.go              NEW: API tests for variant quick-PATCH
        ├── dto.go                                    MODIFIED: add AdminCategoryResponse, ToAdminCategoryResponse
        ├── validation.go                             MODIFIED: add CreateCategoryRequest, UpdateCategoryRequest, UpdateVariantRequest, request builders
        └── routes.go                                 MODIFIED: mount category + media + variant routes inside the existing storeRoute group
```

**Target file sizes:** `categories.go` ~200 lines, `media.go` ~250 lines, `variants.go` ~100 lines, tests ~400–500 lines each. Under 500 everywhere.

---

## New Go module dependency

```
cloud.google.com/go/storage    (latest)
```

Add via `go get cloud.google.com/go/storage` from `services/marketplace-api/`. Then `go mod tidy`. The storage package pulls in `cloud.google.com/go/iam`, `cloud.google.com/go/compute/metadata`, `google.golang.org/api`, and several OAuth2 transitives — these are expected. **Any change to gin / gorm / openfga / existing direct deps indicates a version cascade; stop and report.** This is the same caution from M3 Task 1's recovery incident.

---

## Landmines

Same two from M4/M5a plus one new:

1. **Landmine #1 (go.work):** Don't add a new module. Verify `git diff go.work` is empty before each commit.
2. **CWD drift:** Absolute `cd` on every bash invocation.
3. **(NEW) GCS signed URL test requirement:** The `cloud.google.com/go/storage` package's signed URL generator requires either ADC credentials or an explicit `options.SignedURLOptions` with a pre-provided private key + service account email. Unit tests cannot call the real signer without credentials. The test strategy: (a) unit-test the key-format helper and the type assertion, (b) leave the actual signer call under `// +build integration` gated by `TEST_GCS_BUCKET` env var, which will be empty in CI and skip cleanly. This mirrors the TEST_DATABASE_URL / TEST_FGA_API_URL pattern.

---

## Task decomposition

9 tasks. Tight dependencies — tasks 1–4 build isolated units, tasks 5–7 compose them, task 8 is integration tests, task 9 closes out.

| # | Task | Approx effort |
|---|---|---|
| 1 | `internal/stores/platform_http.go` — real HTTP client + tests | 60 min |
| 2 | `internal/media/gcs.go` — GCSUploader + SignedURLGenerator interface + key-format helper + tests | 75 min |
| 3 | New product service methods: `AddMedia`, `UpdateMedia`, `DeleteMedia`, `UpdateVariantBasics` + tests | 90 min |
| 4 | `internal/handlers/admin/dto.go` + `validation.go` additions for categories, variants, media | 45 min |
| 5 | `internal/handlers/admin/categories.go` + routes + API integration tests | 90 min |
| 6 | `internal/handlers/admin/variants.go` + routes + API integration tests | 60 min |
| 7 | `internal/handlers/admin/media.go` + routes + API integration tests | 90 min |
| 8 | `cmd/marketplace-api/main.go` + `pkg/config` wiring | 45 min |
| 9 | Final verification + PR (includes Helm chart env var documentation) | 30 min |
| **Total** | | **~9 hours** |

Same wall time as M5a. Splittable further if needed.

---

### Task 1: `internal/stores/platform_http.go` — real HTTP client

**Files:**
- Create: `services/marketplace-api/internal/stores/platform_http.go`
- Create: `services/marketplace-api/internal/stores/platform_http_test.go`

**Scope:** A `HTTPClient` struct that implements `stores.Client` by calling `GET <baseURL>/internal/stores/:id` on platform-api and unmarshalling the `{"data": {...}}` envelope into a `*Store`.

Platform-api's response shape (per `services/platform-api/internal/store/handler.go` `getStore`): `{"data": {"id", "tenant_id", "slug", "name", "country_code", "currency_code", "timezone", "status", ...}}`.

Constructor takes a `baseURL`, an internal auth secret (sent as `X-Internal-Auth` header — platform-api's internal routes don't currently check this, but adding the header is cheap defense-in-depth and doesn't break today), and an `*http.Client` (for timeouts / test injection).

```go
// services/marketplace-api/internal/stores/platform_http.go
package stores

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// HTTPClient is a real Client backed by platform-api's internal store
// lookup endpoint (GET /internal/stores/:id). It's wired in main.go when
// MARKETPLACE_PLATFORM_API_URL is set; otherwise StoreMiddleware falls
// back to the stub platform client and tests pre-seed the projection
// table directly.
type HTTPClient struct {
	baseURL  string
	secret   string
	client   *http.Client
}

// NewHTTPClient constructs an HTTPClient. The secret, when non-empty, is
// sent as X-Internal-Auth on every request — defense-in-depth alongside
// Istio's network policy. httpClient may be nil; defaults to a 3-second
// timeout.
func NewHTTPClient(baseURL, secret string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &HTTPClient{
		baseURL: baseURL,
		secret:  secret,
		client:  httpClient,
	}
}

// GetStore satisfies the Client interface.
func (c *HTTPClient) GetStore(ctx context.Context, tenantID, storeID string) (*Store, error) {
	url := fmt.Sprintf("%s/internal/stores/%s", c.baseURL, storeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client: new req: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client: %w", errors.Join(ErrPlatformUnavailable, err))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// ok — continue
	case http.StatusNotFound:
		return nil, nil // tenant-scoping happens upstream; nil triggers ErrNotFound in the middleware
	default:
		return nil, fmt.Errorf("stores: platform client: unexpected status %d: %w",
			resp.StatusCode, ErrPlatformUnavailable)
	}

	var envelope struct {
		Data Store `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("stores: platform client: decode: %w", err)
	}
	// Enforce tenant scoping at the client boundary — if the caller's
	// tenant doesn't match, treat as not found (no leak).
	if envelope.Data.TenantID != tenantID {
		return nil, nil
	}
	return &envelope.Data, nil
}
```

**Tests** (`platform_http_test.go`, `package stores_test`):

1. `TestHTTPClient_GetStore_HappyPath_200` — `httptest.NewServer` returns `{"data": {"id": "s1", "tenant_id": "t1", "slug": "acme", ...}}`; client returns the decoded Store with all fields.
2. `TestHTTPClient_GetStore_404_ReturnsNilNil` — test server returns 404; client returns `(nil, nil)`.
3. `TestHTTPClient_GetStore_WrongTenant_ReturnsNilNil` — test server returns a store with `tenant_id = "other"`; client returns `(nil, nil)` (no leak).
4. `TestHTTPClient_GetStore_5xx_Returns_ErrPlatformUnavailable` — test server returns 503; client returns `(nil, err)` where `errors.Is(err, ErrPlatformUnavailable)` is true.
5. `TestHTTPClient_GetStore_NetworkError_Returns_ErrPlatformUnavailable` — test server is closed before the call; same assertion.
6. `TestHTTPClient_GetStore_SecretHeader_Sent` — test server records the `X-Internal-Auth` header; assert it equals the configured secret.
7. `TestHTTPClient_GetStore_NoSecret_NoHeader` — constructor called with empty secret; assert the header is absent.

Each test uses `httptest.NewServer(http.HandlerFunc(...))` and a default `*http.Client`.

**Steps:** TDD rhythm. Commit: `feat(marketplace-api): add platform-api HTTP client for stores (M5b)`.

---

### Task 2: `internal/media/gcs.go` — GCSUploader + SignedURLGenerator

**Files:**
- Create: `services/marketplace-api/internal/media/gcs.go`
- Create: `services/marketplace-api/internal/media/gcs_test.go`
- Modify: `services/marketplace-api/go.mod` + `go.sum` (add cloud.google.com/go/storage)

**Scope:**

Content-addressed key format (from §14.9 and decision #3 above):
```
tenants/<tenantID>/products/media/<contentHash>/<sanitizedFilename>
```

`BuildStorageKey(tenantID, contentHash, filename) string` — pure helper, unit-testable.

`GCSUploader` struct wrapping a `*storage.BucketHandle`. Implements `Uploader.Verify(ctx, key)` by calling `bucket.Object(key).Attrs(ctx)` and returning an `Attrs{StorageKey, Size, ContentType}`. On `storage.ErrObjectNotExist` returns `(nil, media.ErrNotFound)`.

`SignedURLGenerator` interface (new, exported) with one method: `SignedUploadURL(ctx, key, contentType string, expires time.Duration) (string, time.Time, error)`. The `*GCSUploader` satisfies it by calling `storage.SignedURL(bucket, key, opts)` with `Method: "PUT"` and the provided content type in the Headers slice. The `FakeUploader` from M3 does NOT satisfy this interface — the admin handler does a `_, ok := uploader.(media.SignedURLGenerator)` type assertion and returns 501 when the fake is wired.

```go
// services/marketplace-api/internal/media/gcs.go
package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

// BuildStorageKey returns the canonical content-addressed object key for
// a tenant's product media upload. Per spec §14.9 the hash + filename
// shape is the stable contract; changing it requires a migration.
//
// The filename is sanitized to NFKC letters/digits/dot/dash/underscore.
// Any other character becomes a dash. Leading slashes are stripped.
func BuildStorageKey(tenantID, contentHash, filename string) string {
	filename = strings.TrimLeft(filename, "/")
	var b strings.Builder
	for _, r := range filename {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	safe := b.String()
	if safe == "" {
		safe = "file"
	}
	return fmt.Sprintf("tenants/%s/products/media/%s/%s", tenantID, contentHash, safe)
}

// GCSUploader is the real Uploader implementation backed by a GCS
// bucket. Construct via NewGCSUploader; wire in main.go when
// MARKETPLACE_GCS_BUCKET is set.
type GCSUploader struct {
	bucket     *storage.BucketHandle
	bucketName string
}

// NewGCSUploader constructs a GCSUploader. The caller is responsible
// for closing the underlying *storage.Client.
func NewGCSUploader(client *storage.Client, bucketName string) *GCSUploader {
	return &GCSUploader{
		bucket:     client.Bucket(bucketName),
		bucketName: bucketName,
	}
}

// Verify implements Uploader. Returns ErrNotFound when the object does
// not exist. Any other error is wrapped with context.
func (u *GCSUploader) Verify(ctx context.Context, key string) (*Attrs, error) {
	attrs, err := u.bucket.Object(key).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("media: gcs attrs: %w", err)
	}
	return &Attrs{
		StorageKey:  key,
		Size:        attrs.Size,
		ContentType: attrs.ContentType,
	}, nil
}

// SignedURLGenerator is the interface the /media/upload-url handler
// type-asserts against. The real GCSUploader implements it; the
// FakeUploader does not (the endpoint returns 501 in dev).
type SignedURLGenerator interface {
	SignedUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, time.Time, error)
}

// SignedUploadURL generates a V4 signed PUT URL that the frontend uses
// to upload the object directly to GCS without routing bytes through
// marketplace-api. The x-goog-content-length-range header must be set
// by the caller (the frontend) to enforce the max upload size — we
// cannot encode that into the signed URL itself, so we return the cap
// in the envelope for the frontend to enforce.
//
// Returns (url, expiresAt, error).
func (u *GCSUploader) SignedUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(expires)
	url, err := u.bucket.SignedURL(key, &storage.SignedURLOptions{
		Method:      "PUT",
		ContentType: contentType,
		Expires:     expiresAt,
		Scheme:      storage.SigningSchemeV4,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("media: signed url: %w", err)
	}
	return url, expiresAt, nil
}
```

**Tests** (`gcs_test.go`):

1. `TestBuildStorageKey_BasicFormat` — `BuildStorageKey("t-123", "abc123", "photo.jpg")` → `"tenants/t-123/products/media/abc123/photo.jpg"`.
2. `TestBuildStorageKey_SanitizesFilename` — `"Men's T-Shirt!.jpeg"` → `"Men-s-T-Shirt-.jpeg"`.
3. `TestBuildStorageKey_StripsLeadingSlash` — `"/evil"` → no leading slash in output.
4. `TestBuildStorageKey_EmptyFilename_FallsBackToFile` — `""` → `"tenants/.../file"`.
5. `TestGCSUploader_SatisfiesUploader_And_SignedURLGenerator` — compile-time check via `var _ Uploader = (*GCSUploader)(nil)` and `var _ SignedURLGenerator = (*GCSUploader)(nil)`. A runtime test just asserts both type assertions succeed.
6. `TestFakeUploader_DoesNotSatisfy_SignedURLGenerator` — `_, ok := media.NewFakeUploader().(media.SignedURLGenerator); if ok { t.Error("fake should not implement signer") }`.
7. Optional `TestGCSUploader_Verify_RealBucket` — `//go:build integration`, skipped without `TEST_GCS_BUCKET`. Not a hard requirement; add a stub that just skips so the slot exists for slice 2.

**Steps:**

- [x] **Step 1: Add dep** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go get cloud.google.com/go/storage && go mod tidy`. Expect new direct require line for `cloud.google.com/go/storage` and new transitive deps. **Any downgrade of existing direct deps → stop.**
- [x] **Step 2: Write failing tests**
- [x] **Step 3: Implement `gcs.go`**
- [x] **Step 4: Run tests** `go build ./... && go vet ./internal/media/... && go test ./internal/media/... -race -v`. All 6 unit tests pass (+ optional 7th skips).
- [x] **Step 5: Verify `git diff go.work` empty**
- [x] **Step 6: Commit** `feat(marketplace-api): add GCS uploader with signed URL support (M5b)`

---

### Task 3: Product service — single-media methods + variant basics update

**Files:**
- Create: `services/marketplace-api/internal/product/service_single_media.go`
- Create: `services/marketplace-api/internal/product/service_single_media_integration_test.go`
- Create: `services/marketplace-api/internal/product/service_variant_patch.go`
- Create: `services/marketplace-api/internal/product/service_variant_patch_integration_test.go`

**Scope:**

#### `service_single_media.go`

Three methods on `*Service`:

```go
type AddMediaRequest struct {
	ProductID   string
	StoreID     string
	TenantID    string
	StorageKey  string
	URL         string
	Alt         *string
	Position    int
	MediaType   string // default "image"
}

// AddMedia verifies the upload exists (pre-tx GCS HEAD), then inserts
// one product_media row inside a tx, then enqueues product.updated.
func (s *Service) AddMedia(ctx context.Context, req AddMediaRequest) (*Media, error) { ... }

type UpdateMediaRequest struct {
	ProductID string
	MediaID   string
	StoreID   string
	TenantID  string
	Alt       *string
	Position  *int
	URL       *string
}

// UpdateMedia applies the non-nil fields to one product_media row (tenant-
// scoped via a JOIN on products.tenant_id/store_id). Enqueues product.updated.
func (s *Service) UpdateMedia(ctx context.Context, req UpdateMediaRequest) error { ... }

// DeleteMedia removes one product_media row. GCS object is not touched —
// the nightly sweep cleans up unreferenced keys.
func (s *Service) DeleteMedia(ctx context.Context, productID, mediaID, storeID, tenantID string) error { ... }
```

The repo methods these call:
- `repo.GetByIDForStore` — to verify the product belongs to (store, tenant) before touching media
- New small repo methods: `InsertMediaInTx`, `UpdateMediaInTx`, `DeleteMediaInTx`. Add these to `internal/product/repository_media.go` alongside the existing `ReplaceMediaInTx`. Each takes minimal params (the media row or a (productID, mediaID, fields-map)).

Integration tests (`//go:build integration`):
- `TestIntegration_AddMedia_HappyPath` — seed product, call AddMedia with a registered FakeUploader key, assert the media row appears in a fresh Get.
- `TestIntegration_AddMedia_UploadNotFound` — unregistered key → `ErrUploadNotFound`.
- `TestIntegration_UpdateMedia_UpdatesAlt` — Alt field updates.
- `TestIntegration_UpdateMedia_NotFound` — random mediaID → ErrNotFound.
- `TestIntegration_DeleteMedia_Succeeds` — delete, verify absent on next Get.
- `TestIntegration_DeleteMedia_CrossTenant_404` — media exists but different tenant → ErrNotFound.
- Each test asserts exactly one `product.updated` row was enqueued.

#### `service_variant_patch.go`

```go
type UpdateVariantBasicsRequest struct {
	ProductID         string
	VariantID         string
	StoreID           string
	TenantID          string
	SKU               *string
	Barcode           *string
	Price             *decimal.Decimal
	CompareAtPrice    *decimal.Decimal
	CostPrice         *decimal.Decimal
	WeightGrams       *int
	InventoryQuantity *int
	InventoryPolicy   *string
	LowStockThreshold *int
	Position          *int
	// CurrencyCode is intentionally not updatable; requests carrying
	// a CurrencyCode that differs from the store currency are rejected
	// with CurrencyChangeForbidden.
	CurrencyCode *string
}

// UpdateVariantBasics applies the non-nil fields to one variant row.
// Inventory quantity writes go to variant_stock (not product_variants
// directly) so the trigger maintains the denormalised column.
func (s *Service) UpdateVariantBasics(ctx context.Context, req UpdateVariantBasicsRequest) (*Variant, error) { ... }
```

Repo methods: `UpdateVariantBasicsInTx(tx, productID, variantID, storeID, tenantID, fields)` — add to `internal/product/repository_variants.go`. Translates unique-SKU violations to `SKUTaken`. Inventory stays out of the fields map and goes through `UpdateVariantStockInTx`.

Integration tests:
- `TestIntegration_UpdateVariantBasics_Price` — update price, assert persisted.
- `TestIntegration_UpdateVariantBasics_Inventory_TriggerSyncs` — update inventory_quantity, assert both `variant_stock.quantity` and `product_variants.inventory_quantity` reflect.
- `TestIntegration_UpdateVariantBasics_SKUCollision` — two variants, update second to first's SKU → `ErrSKUTaken`.
- `TestIntegration_UpdateVariantBasics_CurrencyChange_Forbidden` — CurrencyCode != store currency → `ErrCurrencyChangeForbidden`.
- `TestIntegration_UpdateVariantBasics_NotFound` — random ids → `ErrNotFound`.
- Each asserts one `product.updated` outbox row.

**Steps:** Write tests, implement repo methods, implement service methods, run tests, commit. Commit: `feat(marketplace-api): add single-media + variant-basics service methods (M5b)`.

---

### Task 4: `internal/handlers/admin` — DTO + validation additions

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/dto.go`
- Modify: `services/marketplace-api/internal/handlers/admin/validation.go`
- Modify: `services/marketplace-api/internal/handlers/admin/dto_test.go` (new tests only)

**Scope:**

Add to `dto.go`:

```go
type AdminCategoryResponse struct {
	ID          string     `json:"id"`
	StoreID     string     `json:"store_id"`
	ParentID    *string    `json:"parent_id,omitempty"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description,omitempty"`
	ImageURL    *string    `json:"image_url,omitempty"`
	Position    int        `json:"position"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func ToAdminCategoryResponse(c *category.Category) AdminCategoryResponse { ... }
```

Add to `validation.go`:

```go
type CreateCategoryRequest struct {
	ParentID    *string `json:"parent_id,omitempty" binding:"omitempty,uuid"`
	Name        string  `json:"name" binding:"required,max=200"`
	Slug        string  `json:"slug" binding:"omitempty,max=200"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Position    int     `json:"position"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

type UpdateCategoryRequest struct {
	ParentID    *string `json:"parent_id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Position    *int    `json:"position,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

type UpdateVariantRequest struct {
	SKU               *string          `json:"sku,omitempty" binding:"omitempty,max=100"`
	Barcode           *string          `json:"barcode,omitempty"`
	Price             *decimal.Decimal `json:"price,omitempty"`
	CompareAtPrice    *decimal.Decimal `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal `json:"cost_price,omitempty"`
	CurrencyCode      *string          `json:"currency_code,omitempty"`
	WeightGrams       *int             `json:"weight_grams,omitempty"`
	InventoryQuantity *int             `json:"inventory_quantity,omitempty"`
	InventoryPolicy   *string          `json:"inventory_policy,omitempty" binding:"omitempty,oneof=deny continue"`
	LowStockThreshold *int             `json:"low_stock_threshold,omitempty"`
	Position          *int             `json:"position,omitempty"`
}

type UploadURLRequest struct {
	ContentHash string `json:"content_hash" binding:"required,min=16,max=128"`
	Filename    string `json:"filename" binding:"required,max=200"`
	ContentType string `json:"content_type" binding:"required,oneof=image/png image/jpeg image/webp"`
}

type UploadURLResponse struct {
	URL        string    `json:"url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type CreateMediaRequest struct {
	StorageKey string  `json:"storage_key" binding:"required"`
	URL        string  `json:"url" binding:"required"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type" binding:"omitempty,oneof=image video"`
}

type UpdateMediaWireRequest struct {
	Alt      *string `json:"alt,omitempty"`
	Position *int    `json:"position,omitempty"`
	URL      *string `json:"url,omitempty"`
}
```

Plus builder helpers: `toServiceCreateCategory`, `toServiceUpdateCategory`, `toServiceUpdateVariantBasics`, `toServiceAddMedia`, `toServiceUpdateMedia`. Each takes the wire DTO and any required path params and returns the corresponding service request type.

**Tests:** Small unit tests covering the builder helpers — asserting non-nil fields are carried, nil fields are dropped. Target ~5 small tests.

**Commit:** `feat(marketplace-api): add admin DTOs + validation for categories, variants, media (M5b)`.

---

### Task 5: `internal/handlers/admin/categories.go` — CategoryHandler

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/categories.go`
- Create: `services/marketplace-api/internal/handlers/admin/categories_integration_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Scope:**

Handler struct:
```go
type CategoryHandler struct {
	svc    *category.Service
	repo   category.Repository
	logger *slog.Logger
}

func NewCategoryHandler(svc *category.Service, repo category.Repository, logger *slog.Logger) *CategoryHandler
```

Methods:
- `List(c *gin.Context)` — GET /admin/stores/:storeId/categories. Calls `repo.ListByStore(ctx, storeID, tenantID)`. Returns `{"data": [...]}`.
- `Create(c *gin.Context)` — POST. Bind `CreateCategoryRequest`. Build `category.CreateRequest`. Call `svc.Create(ctx, req)`. Return 201 + DTO.
- `Patch(c *gin.Context)` — PATCH. Bind `UpdateCategoryRequest`. Call `svc.Update(ctx, req)`. Return 200 + DTO.
- `Delete(c *gin.Context)` — DELETE. Call `svc.Delete(ctx, id, storeID, tenantID)`. Return 204.

**Routes** (add to `routes.go` inside the existing `storeRoute` group):

```go
categories := storeRoute.Group("/categories")
{
	categories.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CategoryHandler.List)
	categories.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Create)
	categories.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Patch)
	categories.DELETE("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Delete)
}
```

Add `CategoryHandler *CategoryHandler` to the `Deps` struct.

**Integration tests** (`//go:build integration`, `package admin_test`):

Reuse the `setupTestRouter` helper from M5a's `products_integration_test.go`. Extend it to construct + register the CategoryHandler.

1. `TestAPI_AdminCategories_Create_HappyPath` → 201, returned id non-empty.
2. `TestAPI_AdminCategories_Create_MissingName_400` → validation_failed.
3. `TestAPI_AdminCategories_Create_DuplicateSlug_409` → slug_taken.
4. `TestAPI_AdminCategories_List_ReturnsAll` → pre-seed 3 categories, assert 3 in response.
5. `TestAPI_AdminCategories_Patch_UpdatesName` → PATCH, assert name updated.
6. `TestAPI_AdminCategories_Delete_Succeeds_204` → 204, subsequent list omits the row.
7. `TestAPI_AdminCategories_Delete_HasChildren_409` → child-linked parent → category_has_children.
8. `TestAPI_AdminCategories_Delete_HasProducts_409` → category linked to a product → category_not_empty.
9. `TestAPI_AdminCategories_Authz_StaffCannotCreate_404`.
10. `TestAPI_AdminCategories_Authz_UnauthenticatedRequest_401`.

**Commit:** `feat(marketplace-api): add admin category CRUD handlers + routes + tests (M5b)`.

---

### Task 6: `internal/handlers/admin/variants.go` — VariantHandler (quick PATCH)

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/variants.go`
- Create: `services/marketplace-api/internal/handlers/admin/variants_integration_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Scope:**

```go
type VariantHandler struct {
	svc    *product.Service
	logger *slog.Logger
}

func NewVariantHandler(svc *product.Service, logger *slog.Logger) *VariantHandler

func (h *VariantHandler) Patch(c *gin.Context) {
	productID := c.Param("id")
	variantID := c.Param("variantId")
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	var req UpdateVariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceUpdateVariantBasics(req, productID, variantID, storeID, tenantID)
	v, err := h.svc.UpdateVariantBasics(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminVariantResponse(v))
}
```

(`ToAdminVariantResponse(*product.Variant)` already exists via the M5a mapper? If not, add it as a small pure function in `dto.go` — extracted from the existing `ToAdminProductResponse` logic so both the product-level and variant-level responses can use it.)

**Route** (append to routes.go):
```go
storeRoute.PATCH("/products/:id/variants/:variantId",
	deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
	deps.VariantHandler.Patch)
```

Add `VariantHandler *VariantHandler` to `Deps`.

**Integration tests:**
1. `TestAPI_AdminVariants_Patch_Price_200` → updates price.
2. `TestAPI_AdminVariants_Patch_Inventory_TriggerSyncs_200` → updates inventory_quantity, subsequent GET product shows matching value.
3. `TestAPI_AdminVariants_Patch_SKUCollision_409` → sku_taken.
4. `TestAPI_AdminVariants_Patch_CurrencyChange_409` → currency_change_forbidden.
5. `TestAPI_AdminVariants_Patch_NotFound_404` → random variant id.
6. `TestAPI_AdminVariants_Patch_StaffDenied_404`.

**Commit:** `feat(marketplace-api): add admin variant quick-PATCH handler + route + tests (M5b)`.

---

### Task 7: `internal/handlers/admin/media.go` — MediaHandler

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/media.go`
- Create: `services/marketplace-api/internal/handlers/admin/media_integration_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Scope:**

Handler struct:
```go
type MediaHandler struct {
	svc       *product.Service
	uploader  media.Uploader
	logger    *slog.Logger
	// signedURLTTL is how long generated PUT URLs stay valid. Defaults
	// to 15 minutes. Configurable from main.go for test overrides.
	signedURLTTL time.Duration
}

func NewMediaHandler(svc *product.Service, uploader media.Uploader, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{svc: svc, uploader: uploader, logger: logger, signedURLTTL: 15 * time.Minute}
}
```

Methods:
- `UploadURL(c *gin.Context)` — POST /products/:id/media/upload-url. Bind `UploadURLRequest`. Type-assert the uploader to `media.SignedURLGenerator` — if the assertion fails, return 501 Not Implemented with envelope `{"error":"not_implemented","message":"signed upload URLs require a real GCS bucket"}`. Otherwise, call `BuildStorageKey(tenantID, req.ContentHash, req.Filename)` + `signer.SignedUploadURL(ctx, key, contentType, h.signedURLTTL)` + respond `UploadURLResponse`.
- `Create(c *gin.Context)` — POST /products/:id/media. Bind `CreateMediaRequest`. Call `svc.AddMedia`. Return 201 + `AdminMediaResponse`.
- `Patch(c *gin.Context)` — PATCH /products/:id/media/:mediaId. Bind `UpdateMediaWireRequest`. Call `svc.UpdateMedia`. Return 200 + DTO or 204.
- `Delete(c *gin.Context)` — DELETE /products/:id/media/:mediaId. Call `svc.DeleteMedia`. Return 204.

Note: `svc.AddMedia` needs to return the inserted `*Media`; ensure Task 3 returns it. `UpdateMedia` can return the updated row too if the DTO needs to be fresh; otherwise the handler just returns 204.

**Route** (append to routes.go):
```go
media := storeRoute.Group("/products/:id/media")
{
	media.POST("/upload-url",
		deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
		deps.MediaHandler.UploadURL)
	media.POST("",
		deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
		deps.MediaHandler.Create)
	media.PATCH("/:mediaId",
		deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
		deps.MediaHandler.Patch)
	media.DELETE("/:mediaId",
		deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
		deps.MediaHandler.Delete)
}
```

Add `MediaHandler *MediaHandler` to `Deps`.

**Integration tests:**
1. `TestAPI_AdminMedia_UploadURL_WithFake_Returns501` — FakeUploader wired; assert 501 and envelope `error=not_implemented`.
2. `TestAPI_AdminMedia_Create_HappyPath_201` — register key in FakeUploader, POST, assert the media row returned.
3. `TestAPI_AdminMedia_Create_UploadNotFound_400` — key not registered; assert 400 + `error=upload_not_found`.
4. `TestAPI_AdminMedia_Patch_UpdatesAlt_200`.
5. `TestAPI_AdminMedia_Patch_NotFound_404`.
6. `TestAPI_AdminMedia_Delete_Succeeds_204` — verify absent on subsequent product GET.
7. `TestAPI_AdminMedia_StaffDenied_404`.

**Commit:** `feat(marketplace-api): add admin media CRUD + upload-url handlers + tests (M5b)`.

---

### Task 8: `cmd/marketplace-api/main.go` + `pkg/config` wiring

**Files:**
- Modify: `services/marketplace-api/pkg/config/config.go`
- Modify: `services/marketplace-api/pkg/config/config_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Scope:**

#### Config

Add three new fields (all optional):

```go
GCSBucket          string `envconfig:"MARKETPLACE_GCS_BUCKET" default:""`
PlatformAPIURL     string `envconfig:"MARKETPLACE_PLATFORM_API_URL" default:""`
PlatformAPISecret  string `envconfig:"MARKETPLACE_PLATFORM_API_SECRET" default:""`
```

Empty = use fake/stub (dev-friendly). Non-empty = use real.

#### main.go

1. **Conditional uploader construction** — replace the unconditional `fakeUploader := media.NewFakeUploader()` with:
```go
var uploader media.Uploader
if cfg.GCSBucket != "" {
	sc, err := storage.NewClient(context.Background())
	if err != nil {
		log.Error("gcs client", "err", err)
		os.Exit(1)
	}
	uploader = media.NewGCSUploader(sc, cfg.GCSBucket)
	log.Info("media: using real GCS uploader", "bucket", cfg.GCSBucket)
} else {
	uploader = media.NewFakeUploader()
	log.Info("media: using fake uploader (MARKETPLACE_GCS_BUCKET is empty)")
}
```

2. **Conditional platform client construction** — replace the unconditional `stubPlatformClient{}`:
```go
var platformClient stores.Client
if cfg.PlatformAPIURL != "" {
	platformClient = stores.NewHTTPClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
	log.Info("stores: using real platform-api client", "url", cfg.PlatformAPIURL)
} else {
	platformClient = stubPlatformClient{}
	log.Info("stores: using stub platform client (MARKETPLACE_PLATFORM_API_URL is empty)")
}
```

3. **Wire the new handlers** in the existing `adminDeps` block:
```go
categoryHandler := admin.NewCategoryHandler(
	category.NewService(category.Config{ /* ... */ }),
	categoryRepo,
	log,
)
variantHandler := admin.NewVariantHandler(productSvc, log)
mediaHandler := admin.NewMediaHandler(productSvc, uploader, log)

adminDeps = admin.Deps{
	ProductHandler:   productHandler,
	CategoryHandler:  categoryHandler,
	VariantHandler:   variantHandler,
	MediaHandler:     mediaHandler,
	StoresMiddleware: storeMW,
	AuthzMiddleware:  authzMW,
	InternalSecret:   cfg.InternalAuthSecret,
}
```

Read `internal/category/service.go` for the exact `category.Config` field names — don't guess. The existing `category.Service` was built in M3 and its constructor takes `(DB, Repo, OutboxRepo, Logger)` or similar.

**Commit:** `feat(marketplace-api): wire real GCS uploader + platform client + new admin routes (M5b)`.

---

### Task 9: M5b verification + PR

- [x] **Step 1: Full check**
```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go vet ./... && go vet -tags=integration ./... && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

- [x] **Step 2: Branch scope check**
```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && git diff --stat main..feat/products-m5b-categories-media-gcs
```
Only files under `services/marketplace-api/{internal/{auth,media,stores,product,handlers/admin},cmd/marketplace-api,pkg/config,go.mod,go.sum}` plus the M5b plan doc. No migrations, no Helm chart, no docs outside `docs/superpowers/plans/`.

- [x] **Step 3: Push**
```
git push -u origin feat/products-m5b-categories-media-gcs
```

- [x] **Step 4: Open PR** with a body that documents the new env vars for the ops team:

```
gh pr create --base main --head feat/products-m5b-categories-media-gcs --title "feat(marketplace-api): products M5b — categories, media, variant PATCH, real GCS, real platform client" --body "$(cat <<'EOF'
## Summary

Closes M5's exit criterion: every route in spec §6.1 is now live, typed, validated, and authz-gated.

- Categories admin CRUD (GET/POST/PATCH/DELETE) with refusal-on-populated semantics from M3's category service
- Variant quick-PATCH (price/sku/stock/etc) wired through a new thin \`UpdateVariantBasics\` service method; inventory writes go through \`variant_stock\` per §14.5
- Media CRUD (POST/PATCH/DELETE) plus POST /media/upload-url that returns a V4 signed PUT URL when the real GCS uploader is wired, or 501 Not Implemented when the dev fake is wired
- Real GCSUploader (new \`internal/media/gcs.go\`) behind \`MARKETPLACE_GCS_BUCKET\`
- Real platform-api HTTPClient (new \`internal/stores/platform_http.go\`) behind \`MARKETPLACE_PLATFORM_API_URL\` / \`MARKETPLACE_PLATFORM_API_SECRET\`
- Both real clients are config-gated — empty env vars keep the fakes so \`make dev\` works without GCS or platform-api running locally

## New env vars (ops deliverable in tesserix-k8s)

| Var | Required | Default | Purpose |
|---|---|---|---|
| \`MARKETPLACE_GCS_BUCKET\` | prod | \"\" | GCS bucket for product media. Empty → FakeUploader (dev). |
| \`MARKETPLACE_PLATFORM_API_URL\` | prod | \"\" | Base URL of platform-api, e.g. \`http://platform-api.platform.svc\`. Empty → stub client (dev). |
| \`MARKETPLACE_PLATFORM_API_SECRET\` | prod | \"\" | Shared secret for \`X-Internal-Auth\` header. |

Helm chart update is a follow-up PR in the \`tesserix-k8s\` repo — not part of this code-repo PR.

## What is NOT in this PR

- Orphan sweep job (spec §14.10) — separate cron deliverable
- Real GCS integration test against a bucket/emulator — CI continues to use FakeUploader; the real path is exercised at deploy time via \`curl\` smoke tests
- Storefront routes (M6)

## Test plan

- [x] \`go vet ./...\` clean
- [x] \`go vet -tags=integration ./...\` clean
- [x] \`go build ./...\` clean
- [x] \`go test ./... -race\` green
- [x] \`go test -tags integration ./... -race\` skips cleanly without \`TEST_DATABASE_URL\`; local run against Postgres confirms all new cases pass
- [x] Platform HTTP client tested via \`httptest\` (7 cases covering 200/404/5xx/wrong-tenant/auth-header)
- [x] GCS key-format helper + uploader/signer interface tests green
- [x] Category/variant/media API tests cover happy path + validation + authz denial + 409 conflicts + 404 cross-tenant
- [x] \`media/upload-url\` returns 501 when the fake uploader is wired, 200 with a URL + expires_at when the real one is wired (manual verification)

🤖 Subagent-driven execution under the superpowers/writing-plans → subagent-driven-development workflow. Closes the full admin HTTP surface from spec §6.1.
EOF
)"
```

- [x] **Step 5: Wait for CI, merge.**

---

## Exit criteria

- [x] Every route in spec §6.1 is live: 6 product + 4 category + 1 variant + 4 media = 15 admin routes (M5a's 6 + M5b's 9)
- [x] FakeUploader → real GCS switch is config-gated and defaults to fake
- [x] stubPlatformClient → real HTTP client switch is config-gated and defaults to stub
- [x] New service methods (`AddMedia`, `UpdateMedia`, `DeleteMedia`, `UpdateVariantBasics`) enqueue outbox events and are covered by integration tests
- [x] Signed upload URL endpoint returns 501 with the fake and 200 with the real GCS uploader
- [x] Variant PATCH's inventory path writes to `variant_stock` (trigger test)
- [x] Variant PATCH rejects currency changes with `currency_change_forbidden`
- [x] Platform HTTP client enforces tenant scoping at the client boundary (no leak)
- [x] `go.mod` diff touches only `cloud.google.com/go/storage` and its transitives (no cascade to gin/gorm/etc)
- [x] PR is open, CI is green, Helm chart follow-up is flagged in the PR description

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. Platform HTTP client + tests | 60 min |
| 2. GCS uploader + tests | 75 min |
| 3. New service methods + tests | 90 min |
| 4. DTO/validation additions | 45 min |
| 5. Category handler + tests | 90 min |
| 6. Variant handler + tests | 60 min |
| 7. Media handler + tests | 90 min |
| 8. main.go + config wiring | 45 min |
| 9. Verification + PR | 30 min |
| **Total** | **~9 hours** |

Similar wall time to M3/M5a.
