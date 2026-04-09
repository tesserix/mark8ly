# Products M5a — Admin HTTP Surface (Products Lifecycle) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a curl-able, end-to-end admin HTTP surface for the **product lifecycle** — list, get, create, update, delete, copy — wired through the full middleware chain (auth → tenant → store → authz → handler) and covered by API integration tests against real Postgres + FakeUploader. M5b will follow with categories, variants quick-PATCH, media CRUD, real GCS, and the real platform-api client.

**Architecture:** A new `internal/handlers/admin` package owns the products handler, the typed DTO mappers from spec §6.3, the validation layer, and the route registration helper. A new `internal/auth` package owns the header-trust auth + tenant middleware (mirrors platform-api's `gosharedmw.IstioAuth()` pattern — Istio's AuthorizationPolicy verifies the JWT upstream and forwards `X-User-Id` / `X-Tenant-Id` claim headers; marketplace-api trusts those headers because Istio is the only ingress path). Errors flow through `apperrors.Error` and a tiny renderer translates them to the §13.4/§14.13 envelope. The admin engine in `cmd/marketplace-api/main.go` mounts the route group with the full chain from §14.7. The real GCS uploader and the real platform-api client stay stubbed for M5a — admin tests use the M3 `media.FakeUploader`, and the existing M3 `stores` projection + `StoreMiddleware` already work without a real platform client because tests pre-seed the projection table directly.

**Tech Stack:** Go 1.26, Gin (existing), `github.com/google/uuid`, `github.com/shopspring/decimal`, the M3 packages (`pkg/apperrors`, `internal/product`, `internal/category`, `internal/stores`, `internal/outbox`, `internal/media`), the M4 `internal/authz` package.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

This is a single contained slice inside the existing `services/marketplace-api` Go module. It does not add a new module, does not touch `go.work`, does not modify migrations or the Helm chart. It adds files under `services/marketplace-api/internal/auth/`, `services/marketplace-api/internal/handlers/admin/`, and modifies `cmd/marketplace-api/main.go` and `pkg/httpserver/` to wire the new admin route group.

Spec sections authoritative for this milestone:

- §3.2 (middleware chain order — Auth → Tenant → Store → Authz → Handler)
- §6.1 admin routes — **products subset only** (6 of 16 routes); categories/variants/media deferred to M5b
- §6.3 DTO families — Admin DTOs only (Storefront DTOs ship in M6)
- §6.4 → §13.3 → §14.9 — create-product transaction flow (already implemented in M3 service.Create — handlers just translate request DTO → service request)
- §6.5 → §13.4 → §14.13 — error envelope (translation layer ships here)
- §13.1.1 — RequireTenantRelation per route (the M4 middleware factory; mounted here for the first time)
- §13.5 — UX corrections that affect the API contract (e.g., `confirm-dialog` only on hard delete — but this is UI-side, not API-side; no impact on M5a)
- §14.7 (StoreMiddleware) — already implemented in M3, mounted here

**Out of scope (M5b):** categories admin handlers, variants quick-PATCH, media POST/PATCH/DELETE, real GCS uploader, real platform-api client, signed-URL generator for `/media/upload-url`, Helm chart env var additions.

---

## Decisions locked for this milestone

1. **Header-trust auth, no in-process JWT verification.** Marketplace-api trusts `X-User-Id` and `X-Tenant-Id` headers because Istio's upstream `AuthorizationPolicy` verifies the JWT and forwards the claims. Document this clearly in the auth package godoc and add a defense-in-depth shared-secret header (`X-Internal-Auth`) gated by an env var so accidental direct exposure (e.g., port-forwarding around Istio) is rejected. Slice 2 may add full GIP JWT verification if marketplace-api is ever exposed without an Istio sidecar.
2. **Auth middleware always 401, never leaks shape.** Missing or invalid `X-User-Id`/`X-Tenant-Id` returns `401 unauthorized` with envelope `{"error":"unauthorized","message":"authentication required"}`. The 404-on-deny rule (§13.1.1) only applies to AUTHORIZATION failures (FGA Check returning false), not to AUTHENTICATION failures. Distinct codes, distinct status.
3. **`storeId` path param goes through StoreMiddleware unchanged.** M3 already implemented the singleflight + serve-stale logic. M5a just mounts it. Tests pre-seed `stores` rows directly via raw SQL.
4. **Validation happens in the handler, not the service.** Gin binding tags + a tiny custom validator handle shape validation (`required`, `min`, `max`, `oneof`, `len`). Semantic validation (matrix shape, currency override, etc.) stays in the service layer where M3 left it. The handler's job is request parsing + DTO marshalling + error translation, nothing more.
5. **One handler struct per resource, methods per route.** `ProductHandler` has `List`, `Get`, `Create`, `Patch`, `Delete`, `Copy`. Constructor takes a `*product.Service`. No clever route grouping inside the handler — route registration is a separate function so M5b can plug in CategoryHandler the same way.
6. **DTO mappers are pure functions.** `ToAdminProductResponse(*product.Aggregate) AdminProductResponse` lives in `internal/handlers/admin/dto.go` and is unit-tested without any DB. No method on the model, no method on the service — just functions that take the aggregate type and return the wire type.
7. **Error translation is centralized.** `RespondErr(c *gin.Context, err error)` checks `errors.As` against `*apperrors.Error`, maps the code to an HTTP status via a `codeStatus` map, and writes the envelope JSON. Unknown errors fall through to 500 with a generic envelope (and a logged stack trace).
8. **API integration tests use real Postgres + real Gin engine + FakeUploader.** They drive HTTP via `httptest.NewRecorder()` and `router.ServeHTTP(w, req)` against a router constructed exactly like production (same middleware chain, same handler mounts) — except `media.Uploader` is the in-memory fake from M3 Task 10. This is the same pattern platform-api uses for its API tests.
9. **No Helm chart changes in M5a.** The new env vars (`MARKETPLACE_INTERNAL_AUTH_SECRET`, optional) are documented in the plan but added to the chart in M5b alongside `MARKETPLACE_GCS_BUCKET` and `MARKETPLACE_PLATFORM_API_URL`. M5a's `pkg/config` adds the field as `required:"false"` so existing dev environments don't break.
10. **JSON field naming = snake_case.** Use `json:"id"`, `json:"store_id"`, `json:"created_at"`, etc. on every DTO field. This matches the platform-api response shape and is the convention the marketplace-admin frontend already expects.
11. **Pagination is offset-based, default `limit=20 max=100`.** `?page=N&page_size=K`. Returns `{"data": [...], "meta": {"page": N, "page_size": K, "total": T, "total_pages": P}}`. Matches the shape from §6 (the spec doesn't pin a wire format; this is the `platform-api` convention). The repository's `ListAdmin` already takes `Page, PageSize` per Task 11.

---

## File structure produced by M5a

```
services/marketplace-api/
├── cmd/marketplace-api/main.go                       MODIFIED: construct services + handler, mount admin route group
├── pkg/config/config.go                              MODIFIED: add MARKETPLACE_INTERNAL_AUTH_SECRET (optional, default empty = disabled)
├── pkg/config/config_test.go                         MODIFIED: cover the new var
├── pkg/httpserver/httpserver.go                      MODIFIED: expose admin RouterGroup so handlers can mount on it (or accept this lives in main.go — see Task 7)
└── internal/
    ├── auth/
    │   ├── middleware.go                             NEW: HeaderTrustAuth + TenantFromHeader middleware, sentinel respond helpers
    │   └── middleware_test.go                        NEW: unit tests for missing-header, present, internal-secret reject/accept
    └── handlers/
        └── admin/
            ├── errors.go                             NEW: RespondErr + codeStatus map
            ├── errors_test.go                        NEW: unit tests for typed-error → status mapping
            ├── dto.go                                NEW: AdminProductResponse + AdminProductOption + AdminVariantResponse + AdminMediaResponse + AdminCategoryRef + ToAdminProductResponse mapper
            ├── dto_test.go                           NEW: unit tests for the mapper (assert no leakage of forbidden fields, decimal serialization, etc.)
            ├── validation.go                         NEW: request structs with binding tags, plus toServiceRequest helpers
            ├── products.go                           NEW: ProductHandler struct + 6 methods (List, Get, Create, Patch, Delete, Copy)
            ├── routes.go                             NEW: RegisterAdmin(router *gin.RouterGroup, deps Deps) — wires every route in the products subset with its middleware chain
            └── products_integration_test.go          NEW: API tests (build tag integration; full HTTP stack)
```

**Target file sizes:**
- `dto.go` ~250 lines (lots of struct fields + a mapper)
- `products.go` ~400 lines (6 methods × ~50 lines each)
- `products_integration_test.go` ~600+ lines (lots of cases)
- everything else under 200 lines

---

## New Go module dependencies

**None.** All imports come from packages already in `go.mod` (gin, gorm, decimal, uuid, the existing apperrors/product/category/stores/outbox/media/authz internal packages, plus stdlib `net/http/httptest`).

---

## Landmines (from auto-memory)

Same as M4 — only `go.work` (landmine #1) applies. M5a is pure Go inside the existing module.

Additional M5a-specific guard:

- **API integration tests need a real Postgres** (`TEST_DATABASE_URL`). Tests skip cleanly when unset, mirroring the M3 pattern. If you want to run them locally, point `TEST_DATABASE_URL` at the same Postgres that M3 tests use.
- **FakeUploader registration order matters.** API tests must register every storage_key referenced in a Create request BEFORE invoking the handler. The fake's `Register()` is not goroutine-safe internally, so register up-front in test setup.

---

## Task decomposition

9 tasks. The package boundaries are tight, so each task can run as its own subagent with minimal cross-dependencies. Tasks 1–6 are sequential (each builds on the previous). Tasks 7–9 close out the milestone.

| # | Task | Approx effort |
|---|---|---|
| 1 | `internal/auth` — HeaderTrust middleware + tests | 60 min |
| 2 | `internal/handlers/admin/errors.go` — RespondErr + codeStatus map + tests | 45 min |
| 3 | `internal/handlers/admin/dto.go` — Admin DTOs + ToAdminProductResponse mapper + unit tests | 90 min |
| 4 | `internal/handlers/admin/validation.go` — request structs with binding tags + toServiceRequest helpers | 60 min |
| 5 | `internal/handlers/admin/products.go` — ProductHandler with 6 methods | 2 hours |
| 6 | `internal/handlers/admin/routes.go` — RegisterAdmin route registration | 30 min |
| 7 | `cmd/marketplace-api/main.go` + `pkg/config` — wire dependencies, mount admin route group | 60 min |
| 8 | `internal/handlers/admin/products_integration_test.go` — full HTTP stack tests | 2 hours |
| 9 | Verification + PR | 30 min |
| **Total** | | **~9 hours** |

Bigger than M4 (which was ~4 hours), comparable to M3. Splittable into two PRs if needed but designed to ship as one.

---

### Task 1: `internal/auth` — HeaderTrust middleware

**Files:**
- Create: `services/marketplace-api/internal/auth/middleware.go`
- Create: `services/marketplace-api/internal/auth/middleware_test.go`

**Scope:**
- `HeaderTrustAuth(internalSecret string) gin.HandlerFunc` — reads `X-User-Id`, `X-Tenant-Id`, optionally `X-Internal-Auth`. Sets `c.Set("user_id", id)` and `c.Set("tenant_id", id)`. If `internalSecret != ""` and `X-Internal-Auth != internalSecret`, returns 401. If `X-User-Id` or `X-Tenant-Id` missing/empty, returns 401.
- The 401 envelope: `{"error":"unauthorized","message":"authentication required"}`.

```go
// Package auth holds marketplace-api's request authentication middleware.
//
// Marketplace-api trusts X-User-Id and X-Tenant-Id headers because Istio's
// upstream AuthorizationPolicy verifies the JWT and forwards the claims.
// In production the headers are rewritten by Istio's request authentication
// filter and cannot be set by external callers. In dev (where Istio is not
// in front of the binary), tests pass the headers directly. As a defense-
// in-depth measure, MARKETPLACE_INTERNAL_AUTH_SECRET (when set) requires a
// matching X-Internal-Auth header — accidental direct exposure (e.g.,
// port-forwarding around Istio) is rejected.
//
// This package does NOT verify JWTs in-process. Slice 2 may add a real GIP
// verifier if marketplace-api is ever fronted by something other than Istio.
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeaderTrustAuth returns a gin middleware that reads the user/tenant
// claim headers populated upstream by Istio. internalSecret, when
// non-empty, requires X-Internal-Auth to match.
func HeaderTrustAuth(internalSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if internalSecret != "" && c.GetHeader("X-Internal-Auth") != internalSecret {
			respondUnauthorized(c)
			return
		}
		userID := c.GetHeader("X-User-Id")
		tenantID := c.GetHeader("X-Tenant-Id")
		if userID == "" || tenantID == "" {
			respondUnauthorized(c)
			return
		}
		c.Set("user_id", userID)
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

func respondUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"error":   "unauthorized",
		"message": "authentication required",
	})
}
```

**Tests** (`middleware_test.go`, `package auth_test`):

1. `TestHeaderTrustAuth_HappyPath` — `X-User-Id` + `X-Tenant-Id` set; middleware calls Next; downstream handler reads `c.GetString("user_id")` and gets the value.
2. `TestHeaderTrustAuth_MissingUserID_Returns401` — only `X-Tenant-Id` set; aborts with 401.
3. `TestHeaderTrustAuth_MissingTenantID_Returns401` — only `X-User-Id` set; aborts with 401.
4. `TestHeaderTrustAuth_NoInternalSecret_AcceptsAnything` — internalSecret = "", no `X-Internal-Auth` header sent; allowed.
5. `TestHeaderTrustAuth_InternalSecretMismatch_Returns401` — internalSecret set, `X-Internal-Auth` doesn't match; aborts with 401.
6. `TestHeaderTrustAuth_InternalSecretMatch_AllowsRequest` — internalSecret set, header matches; allowed.

Tests use `gin.New() + httptest.NewRequest + httptest.NewRecorder + router.ServeHTTP`. The downstream test handler asserts `user_id` is set in the context.

**Steps:**

- [ ] **Step 1:** Write all 6 failing tests
- [ ] **Step 2:** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go test ./internal/auth/...` — confirm compile failure
- [ ] **Step 3:** Implement `middleware.go` verbatim
- [ ] **Step 4:** Run tests `go test ./internal/auth/... -race -v` — 6 PASS
- [ ] **Step 5:** `git add services/marketplace-api/internal/auth && git commit -m "feat(marketplace-api): add header-trust auth middleware (M5a)"`

---

### Task 2: `internal/handlers/admin/errors.go` — error envelope renderer

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/errors.go`
- Create: `services/marketplace-api/internal/handlers/admin/errors_test.go`

**Scope:**
- `RespondErr(c *gin.Context, err error)` — translates a typed error into the §13.4/§14.13 JSON envelope and the appropriate HTTP status code.
- A `codeStatus` map mapping every `apperrors.Code` to an HTTP status. Unknown codes → 500.
- Generic 500 fallthrough for non-`*apperrors.Error` errors (these get logged with the wrapped error chain at ERROR level by the centralized logger; the response body is generic).

```go
// Package admin holds the marketplace-api admin HTTP handlers.
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// codeStatus maps every typed error code to its HTTP status. The set is
// closed; adding a new code requires updating this map (deliberately
// noisy so a code without a status is a compile-time TODO via
// IsKnownCode coverage in the test).
var codeStatus = map[apperrors.Code]int{
	apperrors.CodeValidationFailed:        http.StatusBadRequest,
	apperrors.CodeVariantMatrixMismatch:   http.StatusBadRequest,
	apperrors.CodeTooManyOptions:          http.StatusBadRequest,
	apperrors.CodeTooManyVariants:         http.StatusBadRequest,
	apperrors.CodeCurrencyMismatch:        http.StatusBadRequest,
	apperrors.CodeTargetStoreInvalid:      http.StatusBadRequest,
	apperrors.CodeUploadNotFound:          http.StatusBadRequest,
	apperrors.CodeForbidden:               http.StatusForbidden,
	apperrors.CodeNotFound:                http.StatusNotFound,
	apperrors.CodeHandleTaken:             http.StatusConflict,
	apperrors.CodeSKUTaken:                http.StatusConflict,
	apperrors.CodeSlugTaken:               http.StatusConflict,
	apperrors.CodeCategoryNotEmpty:        http.StatusConflict,
	apperrors.CodeCategoryHasChildren:     http.StatusConflict,
	apperrors.CodeCurrencyChangeForbidden: http.StatusConflict,
	apperrors.CodePayloadTooLarge:         http.StatusRequestEntityTooLarge,
	apperrors.CodeUnsupportedMediaType:    http.StatusUnsupportedMediaType,
	apperrors.CodeRateLimited:             http.StatusTooManyRequests,
}

// RespondErr writes the standard error envelope for the given error.
// Typed errors (*apperrors.Error) render with their code, message, and
// details. Untyped errors render as a generic 500 with the actual error
// stack logged via slog.
func RespondErr(c *gin.Context, err error, logger *slog.Logger) {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		status, ok := codeStatus[ae.Code]
		if !ok {
			status = http.StatusInternalServerError
		}
		c.AbortWithStatusJSON(status, envelope(string(ae.Code), ae.Message, ae.Details))
		return
	}
	if logger != nil {
		logger.Error("unhandled handler error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError,
		envelope("internal", "internal server error", nil))
}

func envelope(code, msg string, details map[string]any) map[string]any {
	out := map[string]any{"error": code, "message": msg}
	if len(details) > 0 {
		out["details"] = details
	}
	return out
}
```

**Tests** (`errors_test.go`, `package admin_test`):

1. `TestRespondErr_TypedError_RendersEnvelope` — call with `apperrors.HandleTaken("foo", "foo-2")`; assert status 409, body `{"error":"handle_taken", ...}` and `details.suggested == "foo-2"`.
2. `TestRespondErr_NotFound_Returns404` — call with `apperrors.NotFound("product")`; assert 404 + `error=not_found`.
3. `TestRespondErr_ValidationFailed_Returns400` — same with `apperrors.ValidationFailed("title", "required")`; assert 400 + `details.field == "title"`.
4. `TestRespondErr_PayloadTooLarge_Returns413` — `apperrors.PayloadTooLarge("k", 11<<20)` → 413.
5. `TestRespondErr_UntypedError_Returns500_GenericBody` — pass `errors.New("boom")`; assert 500, body has `error=internal`, no leak of "boom".
6. `TestCodeStatus_CoversAllCodes` — iterate the codes from `apperrors.IsKnownCode` (or a hardcoded list in the test) and assert each has an entry in `codeStatus`. Catches a future plan revision adding a code without updating the map.

**Steps:** Same TDD rhythm as Task 1. Commit message: `feat(marketplace-api): add admin error envelope renderer (M5a)`.

---

### Task 3: `internal/handlers/admin/dto.go` — Admin DTOs + mapper

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/dto.go`
- Create: `services/marketplace-api/internal/handlers/admin/dto_test.go`

**Scope:**

DTO families per spec §6.3 (Admin only — Storefront ships in M6):

```go
package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

type AdminProductResponse struct {
	ID                  string                 `json:"id"`
	StoreID             string                 `json:"store_id"`
	Handle              string                 `json:"handle"`
	Title               string                 `json:"title"`
	Description         *string                `json:"description,omitempty"`
	Status              string                 `json:"status"`
	Tags                []string               `json:"tags"`
	SEOTitle            *string                `json:"seo_title,omitempty"`
	SEODescription      *string                `json:"seo_description,omitempty"`
	PrimaryCategoryID   *string                `json:"primary_category_id,omitempty"`
	CopySourceProductID *string                `json:"copy_source_product_id,omitempty"`
	Categories          []AdminCategoryRef     `json:"categories"`
	Options             []AdminProductOption   `json:"options"`
	Variants            []AdminVariantResponse `json:"variants"`
	Media               []AdminMediaResponse   `json:"media"`
	PublishedAt         *time.Time             `json:"published_at,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

type AdminProductOption struct {
	ID     string                  `json:"id"`
	Name   string                  `json:"name"`
	Position int                   `json:"position"`
	Values []AdminProductOptionValue `json:"values"`
}

type AdminProductOptionValue struct {
	ID       string `json:"id"`
	Value    string `json:"value"`
	Position int    `json:"position"`
}

type AdminVariantResponse struct {
	ID                string                  `json:"id"`
	SKU               string                  `json:"sku"`
	Barcode           *string                 `json:"barcode,omitempty"`
	Price             decimal.Decimal         `json:"price"`
	CompareAtPrice    *decimal.Decimal        `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal        `json:"cost_price,omitempty"`
	CurrencyCode      string                  `json:"currency_code"`
	WeightGrams       *int                    `json:"weight_grams,omitempty"`
	InventoryQuantity int                     `json:"inventory_quantity"`
	InventoryPolicy   string                  `json:"inventory_policy"`
	LowStockThreshold *int                    `json:"low_stock_threshold,omitempty"`
	OptionValues      []AdminVariantOptionRef `json:"option_values"`
	Position          int                     `json:"position"`
}

type AdminVariantOptionRef struct {
	OptionName    string `json:"option_name"`
	OptionValueID string `json:"option_value_id"`
	Value         string `json:"value"`
}

type AdminMediaResponse struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	StorageKey string  `json:"storage_key"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type"`
	Width      *int    `json:"width,omitempty"`
	Height     *int    `json:"height,omitempty"`
	Bytes      *int64  `json:"bytes,omitempty"`
}

type AdminCategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ToAdminProductResponse converts a domain Aggregate (product + nested
// rows loaded via Preload) into the wire DTO. Pure function — no DB
// access. The mapper resolves option-value names from the loaded options
// so each variant's OptionValues references include human-readable names
// without an extra DB hop.
func ToAdminProductResponse(a *product.Aggregate, categories []AdminCategoryRef) AdminProductResponse {
	// Build option_value_id → (option_name, value) lookup once.
	type ovInfo struct{ optionName, value string }
	lookup := make(map[string]ovInfo)
	options := make([]AdminProductOption, 0, len(a.Options))
	for _, opt := range a.Options {
		values := make([]AdminProductOptionValue, 0, len(opt.Values))
		for _, v := range opt.Values {
			values = append(values, AdminProductOptionValue{ID: v.ID, Value: v.Value, Position: v.Position})
			lookup[v.ID] = ovInfo{optionName: opt.Name, value: v.Value}
		}
		options = append(options, AdminProductOption{ID: opt.ID, Name: opt.Name, Position: opt.Position, Values: values})
	}

	// Build variant_id → []OptionValueLink lookup.
	linksByVariant := map[string][]product.VariantOptionValue{}
	for _, link := range a.VariantOption {
		linksByVariant[link.VariantID] = append(linksByVariant[link.VariantID], link)
	}

	variants := make([]AdminVariantResponse, 0, len(a.Variants))
	for _, v := range a.Variants {
		ovs := make([]AdminVariantOptionRef, 0, len(linksByVariant[v.ID]))
		for _, link := range linksByVariant[v.ID] {
			info := lookup[link.OptionValueID]
			ovs = append(ovs, AdminVariantOptionRef{
				OptionName:    info.optionName,
				OptionValueID: link.OptionValueID,
				Value:         info.value,
			})
		}
		variants = append(variants, AdminVariantResponse{
			ID: v.ID, SKU: v.SKU, Barcode: v.Barcode,
			Price: v.Price, CompareAtPrice: v.CompareAtPrice, CostPrice: v.CostPrice,
			CurrencyCode: v.CurrencyCode, WeightGrams: v.WeightGrams,
			InventoryQuantity: v.InventoryQuantity, InventoryPolicy: v.InventoryPolicy,
			LowStockThreshold: v.LowStockThreshold, OptionValues: ovs, Position: v.Position,
		})
	}

	media := make([]AdminMediaResponse, 0, len(a.Media))
	for _, m := range a.Media {
		media = append(media, AdminMediaResponse{
			ID: m.ID, URL: m.URL, StorageKey: m.StorageKey,
			Alt: m.Alt, Position: m.Position, MediaType: m.MediaType,
			Width: m.Width, Height: m.Height, Bytes: m.Bytes,
		})
	}

	tags := []string(a.Product.Tags)
	if tags == nil {
		tags = []string{}
	}

	return AdminProductResponse{
		ID:                  a.Product.ID,
		StoreID:             a.Product.StoreID,
		Handle:              a.Product.Handle,
		Title:               a.Product.Title,
		Description:         a.Product.Description,
		Status:              a.Product.Status,
		Tags:                tags,
		SEOTitle:            a.Product.SEOTitle,
		SEODescription:      a.Product.SEODescription,
		PrimaryCategoryID:   a.Product.PrimaryCategoryID,
		CopySourceProductID: a.Product.CopySourceProductID,
		Categories:          categories,
		Options:             options,
		Variants:            variants,
		Media:               media,
		PublishedAt:         a.Product.PublishedAt,
		CreatedAt:           a.Product.CreatedAt,
		UpdatedAt:           a.Product.UpdatedAt,
	}
}
```

**Tests** (`dto_test.go`, `package admin_test`):

1. `TestToAdminProductResponse_FullGraph` — build a sample `product.Aggregate` with 1 product, 2 options × 2 values × 4 variants, 2 media items, 2 category refs. Call mapper. Assert every field maps correctly: variant count = 4, each variant has the right OptionValues with resolved names, media count = 2, options preserved with positions.
2. `TestToAdminProductResponse_NilTags_BecomesEmptySlice` — product with `Tags: nil`; assert response has `Tags: []` (not null).
3. `TestToAdminProductResponse_OmitNilFields` — product with `Description: nil, SEOTitle: nil`; assert JSON marshals these as omitted (not `null`).
4. `TestToAdminProductResponse_DecimalSerialization` — variant with `Price: decimal.NewFromFloat(19.99)`; marshal to JSON and assert the string is `"19.99"` (decimal.Decimal renders as a JSON string by default with shopspring/decimal).
5. `TestAdminVariantResponse_IncludesCostPriceAndInventoryQuantity` — admin DTO MUST expose these fields (storefront DTO MUST NOT — this guards against accidentally cross-pollinating types in M6). Reflect over the struct and assert the field names exist.

**Steps:** TDD rhythm. Commit: `feat(marketplace-api): add admin DTO mapper (M5a)`.

---

### Task 4: `internal/handlers/admin/validation.go` — request structs

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/validation.go`
- Create: `services/marketplace-api/internal/handlers/admin/validation_test.go`

**Scope:** Request DTOs that gin's `c.BindJSON` unmarshals into, with `binding` tags for shape validation. Plus `toServiceCreateRequest`, `toServiceUpdateRequest`, `toServiceCopyRequest` helpers that translate the wire DTO → `product.CreateRequest` / `product.UpdateRequest` / `product.CopyRequest`.

Key wire types:

```go
type CreateProductRequest struct {
	Handle            string                       `json:"handle"`
	Title             string                       `json:"title" binding:"required,max=300"`
	Description       *string                      `json:"description,omitempty"`
	Status            string                       `json:"status" binding:"omitempty,oneof=draft active archived"`
	Tags              []string                     `json:"tags"`
	SEOTitle          *string                      `json:"seo_title,omitempty"`
	SEODescription    *string                      `json:"seo_description,omitempty"`
	PrimaryCategoryID *string                      `json:"primary_category_id,omitempty"`
	Options           []CreateProductOptionInput   `json:"options"`
	Variants          []CreateProductVariantInput  `json:"variants" binding:"required,min=1"`
	Media             []CreateProductMediaInput    `json:"media"`
	CategoryIDs       []string                     `json:"category_ids"`
}

type CreateProductOptionInput struct {
	Name   string   `json:"name" binding:"required,max=100"`
	Values []string `json:"values" binding:"required,min=1"`
}

type CreateProductVariantInput struct {
	SKU               string                          `json:"sku" binding:"required,max=100"`
	Barcode           *string                         `json:"barcode,omitempty"`
	Price             decimal.Decimal                 `json:"price" binding:"required"`
	CompareAtPrice    *decimal.Decimal                `json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal                `json:"cost_price,omitempty"`
	CurrencyCode      string                          `json:"currency_code"`
	WeightGrams       *int                            `json:"weight_grams,omitempty"`
	InventoryQuantity int                             `json:"inventory_quantity"`
	InventoryPolicy   string                          `json:"inventory_policy" binding:"omitempty,oneof=deny continue"`
	LowStockThreshold *int                            `json:"low_stock_threshold,omitempty"`
	OptionValues      []CreateVariantOptionRefInput   `json:"option_values"`
	Position          int                             `json:"position"`
}

type CreateVariantOptionRefInput struct {
	OptionName string `json:"option_name" binding:"required"`
	Value      string `json:"value" binding:"required"`
}

type CreateProductMediaInput struct {
	StorageKey string  `json:"storage_key" binding:"required"`
	URL        string  `json:"url" binding:"required"`
	Alt        *string `json:"alt,omitempty"`
	Position   int     `json:"position"`
	MediaType  string  `json:"media_type" binding:"omitempty,oneof=image video"`
}

type UpdateProductRequest struct {
	Handle            *string                      `json:"handle,omitempty"`
	Title             *string                      `json:"title,omitempty"`
	Description       *string                      `json:"description,omitempty"`
	Status            *string                      `json:"status,omitempty" binding:"omitempty,oneof=draft active archived"`
	Tags              *[]string                    `json:"tags,omitempty"`
	SEOTitle          *string                      `json:"seo_title,omitempty"`
	SEODescription    *string                      `json:"seo_description,omitempty"`
	PrimaryCategoryID *string                      `json:"primary_category_id,omitempty"`
	Options           *[]CreateProductOptionInput  `json:"options,omitempty"`
	Variants          *[]CreateProductVariantInput `json:"variants,omitempty"`
	Media             *[]CreateProductMediaInput   `json:"media,omitempty"`
	CategoryIDs       *[]string                    `json:"category_ids,omitempty"`
}

type CopyProductRequest struct {
	TargetStoreID string `json:"target_store_id" binding:"required,uuid"`
}

type ListProductsQuery struct {
	Status   string `form:"status" binding:"omitempty,oneof=draft active archived"`
	Search   string `form:"search"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

func (q *ListProductsQuery) Defaults() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
}
```

The `toService*Request` helpers translate these wire types into the M3 service layer's request types. They take the path params (`storeId`, `tenantId` from auth context, `productId` from path) as additional arguments.

```go
func toServiceCreateRequest(req CreateProductRequest, tenantID, storeID, createdBy string) product.CreateRequest {
	out := product.CreateRequest{
		TenantID:          tenantID,
		StoreID:           storeID,
		Handle:            req.Handle,
		Title:             req.Title,
		Description:       req.Description,
		Status:            req.Status,
		Tags:              req.Tags,
		SEOTitle:          req.SEOTitle,
		SEODescription:    req.SEODescription,
		PrimaryCategoryID: req.PrimaryCategoryID,
		CategoryIDs:       req.CategoryIDs,
	}
	if createdBy != "" {
		out.CreatedBy = &createdBy
	}
	for _, o := range req.Options {
		out.Options = append(out.Options, product.OptionInput{Name: o.Name, Values: o.Values})
	}
	for _, v := range req.Variants {
		ovs := make([]product.VariantOptionRef, 0, len(v.OptionValues))
		for _, ref := range v.OptionValues {
			ovs = append(ovs, product.VariantOptionRef{OptionName: ref.OptionName, Value: ref.Value})
		}
		out.Variants = append(out.Variants, product.VariantInput{
			SKU: v.SKU, Barcode: v.Barcode,
			Price: v.Price, CompareAtPrice: v.CompareAtPrice, CostPrice: v.CostPrice,
			CurrencyCode: v.CurrencyCode, WeightGrams: v.WeightGrams,
			InventoryQuantity: v.InventoryQuantity,
			InventoryPolicy:   v.InventoryPolicy,
			LowStockThreshold: v.LowStockThreshold,
			OptionValues:      ovs,
			Position:          v.Position,
		})
	}
	for _, m := range req.Media {
		mt := m.MediaType
		if mt == "" {
			mt = "image"
		}
		out.Media = append(out.Media, product.MediaInput{
			StorageKey: m.StorageKey, URL: m.URL, Alt: m.Alt, Position: m.Position, MediaType: mt,
		})
	}
	return out
}
```

Similar `toServiceUpdateRequest` (carefully handling the pointer-of-slice "patch" semantics) and `toServiceCopyRequest`.

**Tests** (`validation_test.go`):

1. `TestCreateProductRequest_BindingTags_RequireTitle` — bind a JSON body with empty title; assert gin's binder returns an error.
2. `TestCreateProductRequest_BindingTags_StatusOneOf` — status="invalid" → binding error.
3. `TestToServiceCreateRequest_PreservesAllFields` — build a populated wire request, call helper, assert every field landed on the service request.
4. `TestToServiceCreateRequest_DefaultsMediaType` — wire `MediaType: ""` → service `MediaType: "image"`.
5. `TestListProductsQuery_Defaults` — empty query, call `Defaults()`, assert `Page=1, PageSize=20`.

**Steps:** TDD rhythm. Commit: `feat(marketplace-api): add admin product validation + service-request mappers (M5a)`.

---

### Task 5: `internal/handlers/admin/products.go` — ProductHandler

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/products.go`

**Scope:** The handler struct + 6 methods. Each method follows the same shape:
1. Extract path params (`storeId`, `id` where applicable)
2. Read `user_id` and `tenant_id` from gin context
3. Bind request body (Create/Patch/Copy) or query (List)
4. Call service method
5. On error → `RespondErr(c, err, h.logger)`
6. On success → call `ToAdminProductResponse` and write 200/201/204

```go
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ProductHandler bundles the dependencies needed by the admin product
// endpoints. Construct via NewProductHandler in main.go and pass to
// RegisterAdmin.
type ProductHandler struct {
	svc      *product.Service
	catRepo  category.Repository
	logger   *slog.Logger
}

func NewProductHandler(svc *product.Service, catRepo category.Repository, logger *slog.Logger) *ProductHandler {
	return &ProductHandler{svc: svc, catRepo: catRepo, logger: logger}
}

// List handles GET /admin/stores/:storeId/products.
func (h *ProductHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	var q ListProductsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}
	q.Defaults()
	page, total, err := h.svc.List(c.Request.Context(), product.ListAdminQuery{
		StoreID: storeID, TenantID: tenantID,
		Status: q.Status, Search: q.Search,
		Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	resp := make([]AdminProductResponse, 0, len(page))
	for _, agg := range page {
		categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
		a := agg
		resp = append(resp, ToAdminProductResponse(&a, categories))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": resp,
		"meta": gin.H{
			"page": q.Page, "page_size": q.PageSize,
			"total": total, "total_pages": ceilDiv(total, int64(q.PageSize)),
		},
	})
}

// Get handles GET /admin/stores/:storeId/products/:id.
func (h *ProductHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	agg, err := h.svc.Get(c.Request.Context(), id, storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusOK, ToAdminProductResponse(agg, categories))
}

// Create handles POST /admin/stores/:storeId/products.
func (h *ProductHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceCreateRequest(req, tenantID, storeID, userID)
	agg, err := h.svc.Create(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusCreated, ToAdminProductResponse(agg, categories))
}

// Patch handles PATCH /admin/stores/:storeId/products/:id.
func (h *ProductHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	var req UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceUpdateRequest(req, id, tenantID, storeID, userID)
	agg, err := h.svc.Update(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, storeID, tenantID)
	c.JSON(http.StatusOK, ToAdminProductResponse(agg, categories))
}

// Delete handles DELETE /admin/stores/:storeId/products/:id.
func (h *ProductHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if err := h.svc.Delete(c.Request.Context(), id, storeID, tenantID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}

// Copy handles POST /admin/stores/:storeId/products/:id/copy.
func (h *ProductHandler) Copy(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	var req CopyProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := product.CopyRequest{
		SourceProductID: id,
		SourceTenantID:  tenantID,
		SourceStoreID:   storeID,
		TargetStoreID:   req.TargetStoreID,
	}
	if userID != "" {
		svcReq.CopiedBy = &userID
	}
	agg, err := h.svc.Copy(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	categories := h.resolveCategoryRefs(c, agg.CategoryLinks, agg.Product.StoreID, tenantID)
	c.JSON(http.StatusCreated, ToAdminProductResponse(agg, categories))
}

// resolveCategoryRefs hydrates the wire DTO category list with names + slugs.
// Reads from category repo. Errors are downgraded to empty slice + logged
// to avoid blocking the product response on a category lookup glitch.
func (h *ProductHandler) resolveCategoryRefs(c *gin.Context, links []product.ProductCategory, storeID, tenantID string) []AdminCategoryRef {
	if len(links) == 0 {
		return []AdminCategoryRef{}
	}
	out := make([]AdminCategoryRef, 0, len(links))
	for _, link := range links {
		cat, err := h.catRepo.GetByIDForStore(c.Request.Context(), link.CategoryID, storeID, tenantID)
		if err != nil {
			if !errors.Is(err, apperrors.ErrNotFound) && h.logger != nil {
				h.logger.Warn("resolve category ref", "category_id", link.CategoryID, "err", err)
			}
			continue
		}
		out = append(out, AdminCategoryRef{ID: cat.ID, Name: cat.Name, Slug: cat.Slug})
	}
	return out
}

func ceilDiv(a int64, b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
```

**Note:** Some methods reference `h.svc.List(...)` and `h.svc.Get(...)` — these are conveniences over `repo.ListAdmin` / `repo.GetByIDForStore`. The M3 service exposed Create/Update/Delete/Copy but NOT a top-level `List` and `Get`. Add them to `internal/product/service.go` as part of this task — they're 5-line wrappers that delegate to the repo:

```go
// List wraps repo.ListAdmin so handlers don't need a direct repo dependency.
func (s *Service) List(ctx context.Context, q ListAdminQuery) ([]Aggregate, int64, error) {
	return s.repo.ListAdmin(ctx, q)
}

// Get wraps repo.GetByIDForStore so handlers consume only the service.
func (s *Service) Get(ctx context.Context, id, storeID, tenantID string) (*Aggregate, error) {
	return s.repo.GetByIDForStore(ctx, id, storeID, tenantID)
}
```

Document in service.go why these wrappers exist (handler-facing convenience).

**No tests in this task** — the handler is exercised end-to-end by Task 8's API integration tests. Unit testing handlers in isolation duplicates the integration test work without adding signal.

**Steps:**

- [ ] **Step 1:** Add `Service.List` + `Service.Get` to `internal/product/service.go`
- [ ] **Step 2:** Implement `products.go`
- [ ] **Step 3:** `cd .../services/marketplace-api && go build ./... && go vet ./internal/handlers/admin/...`
- [ ] **Step 4:** Commit: `feat(marketplace-api): add admin product handler with 6 routes (M5a)`

---

### Task 6: `internal/handlers/admin/routes.go` — RegisterAdmin

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/routes.go`

**Scope:** A single function that takes a `*gin.RouterGroup` and a `Deps` struct (services + middlewares) and mounts the products subset.

```go
package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// Deps groups every dependency the admin route registrar needs. Constructed
// in main.go.
type Deps struct {
	ProductHandler   *ProductHandler
	StoresMiddleware gin.HandlerFunc // from stores.StoreMiddleware
	AuthzMiddleware  *authz.Middleware
	InternalSecret   string
}

// RegisterAdmin mounts the admin route group on the given router. The group
// is rooted at /api/v1/admin and the middleware chain is:
//
//   HeaderTrustAuth → :storeId (path param) → StoreMiddleware → RequireTenantRelation
//
// The auth middleware sets user_id + tenant_id; the store middleware
// validates store ownership and populates the store on the context;
// RequireTenantRelation runs the FGA check per the spec §13.1.1
// permission map.
func RegisterAdmin(router *gin.RouterGroup, deps Deps) {
	auth := auth.HeaderTrustAuth(deps.InternalSecret)

	storeRoute := router.Group("/admin/stores/:storeId", auth, deps.StoresMiddleware)
	{
		products := storeRoute.Group("/products")
		{
			products.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.List)
			products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Create)
			products.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.Get)
			products.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Patch)
			products.DELETE("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner), deps.ProductHandler.Delete)
			products.POST("/:id/copy", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Copy)
		}
	}

	// Categories, variants quick-PATCH, and media routes ship in M5b.
	_ = stores.ErrNotFound // imported for M5b future use
}
```

The permission map mounts here per spec §13.1.1:

| Route | Required role |
|---|---|
| `GET /products[/...]` | `staff` |
| `POST /products` | `admin` |
| `PATCH /products/:id` | `admin` |
| `DELETE /products/:id` | `owner` |
| `POST /products/:id/copy` | `admin` |

**Steps:** No tests here — this is route plumbing exercised by Task 8. Commit: `feat(marketplace-api): add admin route registrar with FGA-gated products subset (M5a)`.

---

### Task 7: `cmd/marketplace-api/main.go` + `pkg/config` — wire admin route group

**Files:**
- Modify: `services/marketplace-api/pkg/config/config.go` (add `InternalAuthSecret string` with `MARKETPLACE_INTERNAL_AUTH_SECRET`, `required:"false"`)
- Modify: `services/marketplace-api/pkg/config/config_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (construct services + handler, mount admin route group)

**Scope:**

`main.go` additions, after the FGA bootstrap block from M4 and before the http server starts:

1. Construct the M3 services that the handler needs:
   - `productRepo := product.NewRepository(conn)`
   - `categoryRepo := category.NewRepository(conn)`
   - `outboxRepo := outbox.NewRepository(conn)`
   - `storesRepo := stores.NewRepository(conn)`
   - `mediaUploader := media.NewFakeUploader()` ← stub for now; M5b swaps in real GCS
   - `productSvc := product.NewService(product.Config{...})`

2. Construct the StoreMiddleware:
   - `storeFlight := &singleflight.Group{}`
   - `storesClient := stubPlatformClient{}` ← M5a uses an in-line stub; M5b swaps in real client
   - `storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{Repo: storesRepo, Client: storesClient, Logger: log, Flight: storeFlight})`

3. Construct the handler:
   - `productHandler := admin.NewProductHandler(productSvc, categoryRepo, log)`

4. Mount the admin group on whichever Gin engine corresponds to MODE:
   - In `mode.Both`: mount on the merged router
   - In `mode.Admin`: mount on `engine`
   - In `mode.Storefront`: do NOT mount (storefront is M6)

5. The mount uses `RegisterAdmin(adminGroup, admin.Deps{...})`.

The `stubPlatformClient` is a 10-line struct that satisfies `stores.Client` and returns `stores.ErrPlatformUnavailable` always — fine for M5a because tests pre-seed the `stores` projection table directly via raw SQL, so the middleware never actually needs to call the platform client. Document this clearly: "M5b replaces this with a real HTTP client to platform-api."

**Steps:** Build, vet, commit. Commit: `feat(marketplace-api): wire admin route group + dependencies in main (M5a)`.

---

### Task 8: `internal/handlers/admin/products_integration_test.go` — full HTTP stack tests

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/products_integration_test.go`

**Scope:** API tests that drive HTTP via httptest against a router constructed exactly like production. Build tag `//go:build integration`. Skip if `TEST_DATABASE_URL` unset.

Test helpers:

```go
func setupTestRouter(t *testing.T, db *gorm.DB) (*gin.Engine, *media.FakeUploader, fixtures) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")

	fakeUploader := media.NewFakeUploader()
	productSvc := product.NewService(product.Config{
		DB:              db,
		Repo:            product.NewRepository(db),
		CategoryRepo:    category.NewRepository(db),
		OutboxRepo:      outbox.NewRepository(db),
		StoresRepo:      stores.NewRepository(db),
		Media:           fakeUploader,
		DefaultLocationID: "00000000-0000-0000-0000-000000000001",
	})

	// FGA fake — every test grants the test user `admin` on the test tenant.
	fakeFGA := authz.NewFakeClient()
	authzMW := authz.NewMiddleware(fakeFGA, nil)

	// Stores middleware with a no-op client (the projection is pre-seeded
	// before each test, so the client is never invoked).
	flight := &singleflight.Group{}
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   stores.NewRepository(db),
		Client: stubClient{},
		Logger: nil,
		Flight: flight,
	})

	productHandler := admin.NewProductHandler(productSvc, category.NewRepository(db), nil)
	admin.RegisterAdmin(api, admin.Deps{
		ProductHandler:   productHandler,
		StoresMiddleware: storeMW,
		AuthzMiddleware:  authzMW,
		InternalSecret:   "",
	})

	return r, fakeUploader, fixtures{fakeFGA: fakeFGA}
}
```

**Test cases** (each `func TestAPI_AdminProducts_...(t *testing.T)`):

1. `Create_HappyPath` — seed store + grant FGA admin role; POST /api/v1/admin/stores/:id/products with full body (1 option, 2 values, 2 variants, 1 media); register media key in fakeUploader; assert 201, body has `id`, `handle`, `variants[0].sku`, etc.
2. `Create_MissingTitle_Returns400_ValidationFailed` — empty title in body; assert 400, error code `validation_failed`.
3. `Create_NoAuthHeader_Returns401` — request without `X-User-Id`; assert 401 + `error=unauthorized`.
4. `Create_NoFGAGrant_Returns404` — auth headers set, but fakeFGA has no grant; assert 404 + `error=not_found`.
5. `Create_HandleCollision_Returns409_HandleTaken` — create twice with same handle; assert second returns 409, body has `error=handle_taken` and `details.suggested`.
6. `List_ReturnsEmptyPage_WhenNoProducts` — GET without any products; assert 200, `data: []`, `meta.total: 0`.
7. `List_PaginatesCorrectly` — create 25 products; GET with `?page_size=10&page=2`; assert 10 results, `meta.total: 25`, `meta.total_pages: 3`.
8. `List_FiltersByStatus` — create 3 draft + 2 active; GET `?status=active`; assert 2 results.
9. `Get_ExistingProduct_Returns200` — create then GET by id; assert full response shape.
10. `Get_NonExistent_Returns404_NotFound` — GET random uuid; assert 404 + `error=not_found`.
11. `Get_CrossTenant_Returns404` — create as tenant A, GET as tenant B; assert 404 (no leak).
12. `Patch_UpdatesTitle_Returns200_WithUpdatedTitle` — create, PATCH `{"title":"new title"}`, assert response has new title.
13. `Patch_NotFound_Returns404` — PATCH random uuid; 404.
14. `Delete_Returns204_NoBody` — create, DELETE, assert 204; subsequent GET returns 404.
15. `Copy_HappyPath_Returns201_NewProduct` — create source product in store A, seed store B, POST `/products/:id/copy {"target_store_id": "<B>"}`; assert 201, returned product has `copy_source_product_id` matching source.
16. `Copy_TargetSameAsSource_Returns400_TargetStoreInvalid` — target id == source store id; assert 400 + `error=target_store_invalid`.
17. `Authz_RequireOwnerOnDelete_StaffDenied_Returns404` — fakeFGA grants only `staff` (not `owner`); DELETE; assert 404.
18. `Authz_RequireAdminOnCreate_StaffDenied_Returns404` — fakeFGA grants only `staff`; POST; assert 404.

Each test follows the seed → request → assert pattern. Use `httptest.NewRecorder()` + `router.ServeHTTP(w, req)`.

**Steps:** TDD-ish (write the test, implement gaps in handlers if any, verify pass). One commit at the end. Commit: `test(marketplace-api): API integration tests for admin product lifecycle (M5a)`.

---

### Task 9: M5a verification + PR

- [ ] **Step 1:** Full test run

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go vet ./... && go vet -tags=integration ./... && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

All clean. API tests skip cleanly without `TEST_DATABASE_URL`.

- [ ] **Step 2:** Push the branch

```
git push -u origin feat/products-m5a-admin-products-http
```

- [ ] **Step 3:** Open PR

```
gh pr create --base main --head feat/products-m5a-admin-products-http --title "feat(marketplace-api): products M5a — admin HTTP surface (products lifecycle)" --body "$(cat <<'EOF'
## Summary

- Lands the admin HTTP surface for the **product lifecycle** — list, get, create, patch, delete, copy — wired through the full middleware chain (auth → tenant → store → authz → handler) per spec §3.2 + §14.7.
- Adds \`internal/auth\` (header-trust auth — Istio claim headers + optional internal-secret), \`internal/handlers/admin\` (DTOs, validation, error envelope renderer, ProductHandler, route registrar).
- Mounts the admin route group on the admin Gin engine in \`cmd/marketplace-api/main.go\`. The new \`MARKETPLACE_INTERNAL_AUTH_SECRET\` env var is optional; when empty, header-trust accepts any request that has the X-User-Id and X-Tenant-Id headers.
- Permission map per spec §13.1.1: GET=staff, POST/PATCH=admin, DELETE=owner, COPY=admin.

## What is NOT in this PR (deferred to M5b)

- Categories admin handlers (GET/POST/PATCH/DELETE)
- Variants quick-PATCH (PATCH /products/:id/variants/:variantId)
- Media POST/PATCH/DELETE handlers and the upload-url endpoint
- **Real GCS uploader** — M5a uses the M3 \`media.FakeUploader\` stub; tests register storage_keys directly
- **Real platform-api client** — M5a uses an in-line stub; \`StoreMiddleware\` is exercised against a pre-seeded \`stores\` projection table
- Helm chart env var additions

## Test plan

- [x] \`go vet ./...\` clean
- [x] \`go vet -tags=integration ./...\` clean
- [x] \`go build ./...\` clean
- [x] \`go test ./... -race\` green (unit tests for auth middleware, error renderer, DTO mapper, validation)
- [x] \`go test -tags integration ./... -race\` green when \`TEST_DATABASE_URL\` is set; clean skip otherwise
- [x] 18+ API integration tests covering happy path, validation failures, auth failures, FGA denials, cross-tenant 404, handle collision, copy, pagination, status filter

🤖 Subagent-driven execution under the superpowers/writing-plans → subagent-driven-development workflow.
EOF
)"
```

- [ ] **Step 4:** Wait for CI, merge.

---

## Exit criteria

- [ ] Every product lifecycle endpoint (List, Get, Create, Patch, Delete, Copy) responds correctly to a `curl` from the test harness
- [ ] Header-trust auth blocks requests with missing claim headers (401)
- [ ] FGA middleware blocks requests with insufficient role (404, no leak)
- [ ] StoreMiddleware blocks cross-tenant store access (404, no leak)
- [ ] Error envelope matches §13.4/§14.13 for every typed error code reachable through the HTTP layer
- [ ] DTO mapper does not leak any forbidden fields (compile-time guarantee + unit test on field names)
- [ ] All 18+ API integration tests green against real Postgres
- [ ] No changes to migrations, Helm chart, CI workflows, or `go.work`
- [ ] PR is open and CI is green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. Auth middleware + tests | 60 min |
| 2. Error envelope renderer + tests | 45 min |
| 3. DTO mapper + tests | 90 min |
| 4. Validation + service-request mappers | 60 min |
| 5. ProductHandler (6 methods) | 2 hours |
| 6. RegisterAdmin route registrar | 30 min |
| 7. main.go + config wiring | 60 min |
| 8. API integration tests (18 cases) | 2 hours |
| 9. Verification + PR | 30 min |
| **Total** | **~9 hours** |

Roughly the same wall time as M3. Designed to ship as one PR.
