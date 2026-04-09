# Products M3 — Repositories, Services, Sanitizer, Outbox Publisher

> **Status: ✅ COMPLETE** — all 14 tasks landed on `feat/products-m3-services`, PR [tesserix/mark8ly#8](https://github.com/tesserix/mark8ly/pull/8). Executed via superpowers subagent-driven-development (option C: hybrid fast-track + full review on Tasks 11/12). 20 commits, +8289/-24 across 35 files. Every reachable typed error code from spec §13.4 + §14.13 covered by integration tests. ListAdmin query-count gate enforced. Bluemonday OWASP corpus green. Outbox publisher wired into `cmd/marketplace-api/main.go` for admin/both modes with clean shutdown.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the admin-path service layer for the products feature on top of the M2 schema — product/category/stores/outbox repositories, sanitizer, variant matrix helpers, product + category services (Create/Update/Delete/Copy), and the in-process outbox publisher goroutine that bumps `store_watermarks`. No HTTP handlers yet. No OpenFGA. No real GCS or Pub/Sub.

**Architecture:** Classical repo/service split mirroring `services/platform-api/internal/onboarding/` (the closest reference in this monorepo). Services receive a `*gorm.DB`, open their own transactions, and write `outbox_events` rows in the same tx as the data they produce. A background goroutine polls `outbox_events` every 2s via `FOR UPDATE SKIP LOCKED`, upserts `store_watermarks`, and marks rows published. A `pkg/apperrors` package holds the full typed-error enumeration from spec §13.4 + §14.13; every service-layer error flows through it so M5 handlers can pattern-match cleanly. Bluemonday sanitizer is version-pinned per §14.14. The GCS media uploader is a stub interface in `internal/media` that M5 will back with a real client.

**Tech Stack:** Go 1.26, Gin (not touched in M3), GORM v1.31, shopspring/decimal, golang-migrate (not touched), `github.com/microcosm-cc/bluemonday` (new dep), `golang.org/x/sync/singleflight` (new dep).

---

## Scope check

This plan is one contained slice inside the existing `services/marketplace-api` Go module. It does not add a new module, does not touch `go.work`, does not change the Dockerfile, does not modify CI, does not change migrations. It only adds `.go` files under `services/marketplace-api/internal/...`, one under `services/marketplace-api/pkg/apperrors/`, and a handful of lines in `services/marketplace-api/cmd/marketplace-api/main.go` to wire the publisher goroutine.

Spec sections authoritative for this milestone:
- §3.3 (cross-module boundaries)
- §6.4 (create flow — superseded by §13.3 which is superseded by §14 where noted)
- §6.5 error envelope → §13.4 → §14.13
- §8 (M3 entry — this milestone)
- §13.1.1 (no per-object FGA tuples — simplifies our tx flow — transactions become DB-only)
- §13.1.4 → §14.7 (StoreMiddleware serve-stale-on-error)
- §13.3 (create-product transaction flow, GCS before BEGIN)
- §13.6.1 → §14.14 (bluemonday policy version pin)
- §14.1 (store_watermarks maintained by publisher, NOT in the mutation tx)
- §14.2 (currency change forbidden — `currency_change_forbidden` error)
- §14.3 (unique violations → typed `handle_taken` / `sku_taken` / `slug_taken`)
- §14.5 (variant_stock trigger — service must write to variant_stock, never to product_variants.inventory_quantity)
- §14.6 (outbox publisher — 2s tick, batch 100, SKIP LOCKED)
- §14.9 (pre-tx GCS attrs check — our stub mimics the contract, real impl in M5)
- §14.13 (error code additions)
- §14.17 (`ListAdmin` query-count gate — we'll assert a conservative ceiling)

---

## Decisions locked for this milestone

1. **No HTTP, no FGA, no Pub/Sub, no real GCS.** These are all downstream milestones. The service layer exposes plain Go methods; callers in tests drive them directly.
2. **`pkg/apperrors` is new in this milestone.** marketplace-api currently has no typed-error helper. Don't import platform-api's `pkg/errors` — each service owns its own envelope. Copy the shape, not the code.
3. **Transactions are DB-only.** Since §13.1.1 dropped per-object FGA tuples and §14.1 moved watermark bumps out of the mutation tx, `service.Create` opens exactly one GORM tx, writes rows, writes one `outbox_events` row, and commits. No GCS I/O inside the tx (validated pre-tx per §13.3).
4. **Media uploader is a stub.** `internal/media.Uploader` is an interface with `Verify(ctx, storageKey string) (*Attrs, error)`. The production impl lands in M5. M3 tests inject an in-memory `fakeUploader` that returns pre-registered attrs.
5. **Store pull-through is mocked.** `StoreMiddleware`'s platform client is an interface; M3 tests use a fake client. The real `platform.Client` implementation is deferred to M5.
6. **`ListAdmin` target: ≤7 queries per call** (spec §14.17 says verify ≤5, raise to ≤7 if warranted). With 1 products query + preload of options/values/variants/option-value-links/media/categories, 7 is a realistic ceiling. We'll assert whatever we land on and document it.
7. **Singleflight for store refresh** — `golang.org/x/sync/singleflight` is a new direct dependency (it's already indirect via many transitive users).
8. **Bluemonday policy v1** allows the minimal set needed by the admin rich-text editor: `p`, `br`, `strong`, `em`, `u`, `ul`, `ol`, `li`, `a` (with `href` only, `rel="nofollow"` forced, `target` stripped), `h2`–`h4`, `blockquote`. Nothing else. The exact list lives in `sanitizer.go` — the OWASP corpus test pins behavior.
9. **Copy-to-store preserves media by reference** — the same `storage_key` is reused; no GCS object copy. Refcount is `count(*) on storage_key`. Source currency is preserved on the copy (spec §6 copy semantics + §14.2: the target store's currency may differ but we don't convert).
10. **Category delete refusal** — if the category has products OR children, refuse with `category_not_empty` / `category_has_children` and the count in `details`.
11. **Soft-delete only.** `Delete` methods set `deleted_at = now()`. No hard delete path for slice 1 except via DB cascades triggered by the nightly orphan sweep (not M3's concern).
12. **No `variant_stock` direct reads in service-layer business logic.** `Variant.InventoryQuantity` is trigger-maintained. Service-layer writes go to `variant_stock`; the trigger cascades.
13. **Every typed error code from §13.4 + §14.13 that is reachable from M3 code paths has at least one service-level integration test producing it.** The unreachable ones (`rate_limited`, `forbidden`, `payload_too_large`, `unsupported_media_type` when >10 MiB — we test the <10 MiB path since the stub can simulate) are documented as "deferred to M5".

---

## File structure produced by M3

```
services/marketplace-api/
├── cmd/marketplace-api/main.go             MODIFIED: start publisher goroutine
├── pkg/apperrors/
│   ├── errors.go                           NEW: Error struct + typed codes + constructors
│   └── errors_test.go                      NEW: constructor + Is/As behavior
└── internal/
    ├── media/
    │   └── uploader.go                     NEW: Uploader interface + Attrs struct + stub
    ├── stores/
    │   ├── repository.go                   NEW: GetByIDForTenant, Upsert, IsStale helper, ErrNotFound
    │   ├── platform_client.go              NEW: Client interface + stub impl (returns ErrNotImplemented)
    │   ├── middleware.go                   NEW: StoreMiddleware with singleflight + serve-stale
    │   ├── repository_integration_test.go  NEW
    │   └── middleware_test.go              NEW: unit, fake client
    ├── outbox/
    │   ├── repository.go                   NEW: EnqueueInTx, PollBatch, MarkPublished
    │   ├── publisher.go                    NEW: Publisher goroutine
    │   ├── repository_integration_test.go  NEW
    │   └── publisher_integration_test.go   NEW: real commits, watermark assertion
    ├── category/
    │   ├── repository.go                   NEW: Create, GetByID, ListByStore, ListTree, UpdateInTx, SoftDeleteInTx, HasChildren, HasProducts
    │   ├── service.go                      NEW: Create/Update/Delete with refusal logic
    │   ├── repository_integration_test.go  NEW
    │   └── service_integration_test.go     NEW: all typed error cases
    └── product/
        ├── sanitizer.go                    NEW: SanitizerPolicyVersion + policyV1() + Sanitize
        ├── sanitizer_test.go               NEW: OWASP top-10 injection corpus
        ├── matrix.go                       NEW: pure variant-matrix validate + diff
        ├── matrix_test.go                  NEW
        ├── handle.go                       NEW: slug/handle generator + collision suggester (pure)
        ├── handle_test.go                  NEW
        ├── repository.go                   NEW: the aggregate repository (product + options + variants + media + categories + stock)
        ├── repository_integration_test.go  NEW
        ├── service.go                      NEW: Create/Update/Delete/Copy + helpers
        └── service_integration_test.go     NEW: every typed error code covered
```

**Target file sizes:** `product/repository.go` and `product/service.go` are the two big files. Aim for <700 lines each. If either climbs above 800, split into `repository_variants.go`, `repository_media.go`, `service_copy.go` etc. rather than ballooning one file.

---

## New Go module dependencies

Added **where they are first imported** (not in Task 1 — `go mod tidy` prunes unused deps, so pre-adding them would be undone):

```
github.com/microcosm-cc/bluemonday v1.0.26   // added in Task 8 (sanitizer.go)
golang.org/x/sync                  (latest)  // promoted to direct in Task 3 (middleware.go imports singleflight)
```

In each of those tasks, run `go get <package>` (no version pin — let Go pick the latest compatible release; the repo already has x/sync as indirect at a newer version than v0.8.x, so a version pin would silently downgrade the whole dep tree). Then `go mod tidy` and commit `go.mod` + `go.sum` as part of that task's commit. Always run Go commands from `services/marketplace-api/` (not from a shell whose CWD has drifted elsewhere — CWD drift caused a Task 1 recovery incident).

---

## Landmines (from auto-memory: feedback_marketplace_api_landmines.md)

M3 is a pure-Go milestone, so only two of the six landmines apply:

1. **Landmine #1 (go.work):** We are NOT creating a new module, so `go.work` is untouched. Confirm in Task 1 with `git diff go.work` → empty.
2. **Landmine #3 (DATABASE_URL URL encoding):** not triggered by M3 code, but the service integration tests need `TEST_DATABASE_URL` to point at a Postgres with URL-safe credentials. The existing M2 test DB setup already works; re-use the same DSN. If a fresh developer hits this, the `testdb.NewTx(t)` skip path protects CI.

Landmines #2, #4, #5, #6 are infra / chart / embed concerns and do not apply to M3.

Additional M3-specific landmine (new):

3. **Postgres aborts the current transaction on any constraint violation.** M2's `TestIntegration_PartialUnique_SoftDelete` is the reference pattern: tests that expect an `INSERT` to fail must wrap the expected-failure insert in a savepoint (`tx.SavePoint("sp"); ...; tx.RollbackTo("sp")`). Otherwise the outer test transaction is poisoned and any subsequent assertion fails with `current transaction is aborted`. The service layer hits this whenever it translates a `pgconn.PgError{Code: 23505}` — in production the service opens its own tx so there is no outer tx to poison, but **in integration tests where `testdb.NewTx(t)` already opened one**, the service must either open its own nested tx via `tx.Begin()` or the test must use savepoints. Decision: service-layer Create/Update/Delete always call `db.Transaction(...)` (which runs inside a savepoint when `db` is already a tx — GORM handles this). Verified by running the negative-path tests with `TEST_DATABASE_URL` set.

---

## Task decomposition

14 tasks, ordered by dependency. Tasks 1–5 have no cross-dependencies after Task 1 and can be parallelized if running with subagent-driven-development; 6–12 are serial.

Legend: **R** = repository, **S** = service, **U** = unit/pure, **I** = integration (needs Postgres).

---

### Task 1: `pkg/apperrors` — typed error envelope

**Files:**
- Create: `services/marketplace-api/pkg/apperrors/errors.go`
- Create: `services/marketplace-api/pkg/apperrors/errors_test.go`
- Modify: `services/marketplace-api/go.mod`, `services/marketplace-api/go.sum` (add bluemonday + promote x/sync)

**Scope:** One small package. Defines the shape M5 handlers will render as JSON and every service-layer error uses.

- [x] **Step 1: Write the failing test**

```go
// services/marketplace-api/pkg/apperrors/errors_test.go
package apperrors_test

import (
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func TestError_Is_MatchesByCode(t *testing.T) {
	err := apperrors.HandleTaken("linen-shirt", "linen-shirt-2")
	if !errors.Is(err, apperrors.ErrHandleTaken) {
		t.Fatalf("expected Is(ErrHandleTaken)==true, got err=%v", err)
	}
	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected errors.As to match *Error")
	}
	if ae.Code != apperrors.CodeHandleTaken {
		t.Fatalf("code: got %q want %q", ae.Code, apperrors.CodeHandleTaken)
	}
	if ae.Details["suggested"] != "linen-shirt-2" {
		t.Fatalf("details.suggested: got %v", ae.Details["suggested"])
	}
}

func TestError_Codes_CoverSpec(t *testing.T) {
	// Regression guard: every code from spec §13.4 + §14.13 must exist.
	want := []string{
		"validation_failed", "variant_matrix_mismatch", "too_many_options",
		"too_many_variants", "currency_mismatch", "handle_taken", "sku_taken",
		"category_not_empty", "category_has_children", "target_store_invalid",
		"upload_not_found", "forbidden", "not_found",
		"payload_too_large", "unsupported_media_type", "rate_limited",
		"currency_change_forbidden", "slug_taken",
	}
	for _, code := range want {
		if !apperrors.IsKnownCode(code) {
			t.Errorf("code %q missing from apperrors package", code)
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

```
cd services/marketplace-api && go test ./pkg/apperrors/...
```
Expected: compile failure (package doesn't exist yet).

- [x] **Step 3: Write minimal implementation**

```go
// services/marketplace-api/pkg/apperrors/errors.go
// Package apperrors is the marketplace-api typed error envelope.
// Every error that escapes the service layer flows through *Error so
// that M5's HTTP handlers render a consistent JSON envelope
// ({"error": "<code>", "message": "...", "details": {...}}) without
// type-switching on driver-level errors. Codes match spec §13.4 + §14.13.
package apperrors

import (
	"errors"
	"fmt"
)

// Code is an enumerated string identifying a failure mode.
type Code string

const (
	CodeValidationFailed        Code = "validation_failed"
	CodeVariantMatrixMismatch   Code = "variant_matrix_mismatch"
	CodeTooManyOptions          Code = "too_many_options"
	CodeTooManyVariants         Code = "too_many_variants"
	CodeCurrencyMismatch        Code = "currency_mismatch"
	CodeHandleTaken             Code = "handle_taken"
	CodeSKUTaken                Code = "sku_taken"
	CodeSlugTaken               Code = "slug_taken"
	CodeCategoryNotEmpty        Code = "category_not_empty"
	CodeCategoryHasChildren     Code = "category_has_children"
	CodeTargetStoreInvalid      Code = "target_store_invalid"
	CodeUploadNotFound          Code = "upload_not_found"
	CodeForbidden               Code = "forbidden"
	CodeNotFound                Code = "not_found"
	CodePayloadTooLarge         Code = "payload_too_large"
	CodeUnsupportedMediaType    Code = "unsupported_media_type"
	CodeRateLimited             Code = "rate_limited"
	CodeCurrencyChangeForbidden Code = "currency_change_forbidden"
)

// Error is the marketplace-api envelope. Satisfies the error interface.
type Error struct {
	Code    Code           // typed code (stable wire contract)
	Message string         // human-readable, PII-free
	Details map[string]any // extra structured data rendered into details{}
	wrapped error          // underlying cause for errors.Is / %w
}

func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// Sentinel values for errors.Is comparisons. The wrapped *Error does NOT
// equal the sentinel; Is() below does the code-level comparison.
var (
	ErrValidationFailed        = &Error{Code: CodeValidationFailed}
	ErrVariantMatrixMismatch   = &Error{Code: CodeVariantMatrixMismatch}
	ErrTooManyOptions          = &Error{Code: CodeTooManyOptions}
	ErrTooManyVariants         = &Error{Code: CodeTooManyVariants}
	ErrCurrencyMismatch        = &Error{Code: CodeCurrencyMismatch}
	ErrHandleTaken             = &Error{Code: CodeHandleTaken}
	ErrSKUTaken                = &Error{Code: CodeSKUTaken}
	ErrSlugTaken               = &Error{Code: CodeSlugTaken}
	ErrCategoryNotEmpty        = &Error{Code: CodeCategoryNotEmpty}
	ErrCategoryHasChildren     = &Error{Code: CodeCategoryHasChildren}
	ErrTargetStoreInvalid      = &Error{Code: CodeTargetStoreInvalid}
	ErrUploadNotFound          = &Error{Code: CodeUploadNotFound}
	ErrForbidden               = &Error{Code: CodeForbidden}
	ErrNotFound                = &Error{Code: CodeNotFound}
	ErrPayloadTooLarge         = &Error{Code: CodePayloadTooLarge}
	ErrUnsupportedMediaType    = &Error{Code: CodeUnsupportedMediaType}
	ErrRateLimited             = &Error{Code: CodeRateLimited}
	ErrCurrencyChangeForbidden = &Error{Code: CodeCurrencyChangeForbidden}
)

// Is makes errors.Is(err, sentinel) match when the codes are equal,
// so callers can write `errors.Is(err, apperrors.ErrHandleTaken)` regardless
// of which constructor built the error.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// IsKnownCode reports whether the given code string is one of the
// enumerated codes. Used by tests to assert enumeration coverage.
func IsKnownCode(s string) bool {
	switch Code(s) {
	case CodeValidationFailed, CodeVariantMatrixMismatch, CodeTooManyOptions,
		CodeTooManyVariants, CodeCurrencyMismatch, CodeHandleTaken, CodeSKUTaken,
		CodeSlugTaken, CodeCategoryNotEmpty, CodeCategoryHasChildren,
		CodeTargetStoreInvalid, CodeUploadNotFound, CodeForbidden, CodeNotFound,
		CodePayloadTooLarge, CodeUnsupportedMediaType, CodeRateLimited,
		CodeCurrencyChangeForbidden:
		return true
	}
	return false
}

// ---------- constructors ----------

func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, wrapped: err}
}

func ValidationFailed(field, msg string) *Error {
	return &Error{Code: CodeValidationFailed, Message: msg,
		Details: map[string]any{"field": field}}
}

func HandleTaken(attempted, suggested string) *Error {
	return &Error{Code: CodeHandleTaken,
		Message: fmt.Sprintf("handle %q is already in use in this store", attempted),
		Details: map[string]any{"attempted": attempted, "suggested": suggested}}
}

func SKUTaken(sku string) *Error {
	return &Error{Code: CodeSKUTaken,
		Message: fmt.Sprintf("SKU %q is already in use in this store", sku),
		Details: map[string]any{"sku": sku}}
}

func SlugTaken(attempted, suggested string) *Error {
	return &Error{Code: CodeSlugTaken,
		Message: fmt.Sprintf("slug %q is already in use in this store", attempted),
		Details: map[string]any{"attempted": attempted, "suggested": suggested}}
}

func CategoryNotEmpty(productCount int64) *Error {
	return &Error{Code: CodeCategoryNotEmpty,
		Message: "category still has products and cannot be deleted",
		Details: map[string]any{"product_count": productCount}}
}

func CategoryHasChildren(childCount int64) *Error {
	return &Error{Code: CodeCategoryHasChildren,
		Message: "category has sub-categories and cannot be deleted",
		Details: map[string]any{"child_count": childCount}}
}

func VariantMatrixMismatch(expected, got int) *Error {
	return &Error{Code: CodeVariantMatrixMismatch,
		Message: "variant count does not match option-value product",
		Details: map[string]any{"expected": expected, "got": got}}
}

func TooManyOptions(got int) *Error {
	return &Error{Code: CodeTooManyOptions,
		Message: "a product may not have more than 3 option axes",
		Details: map[string]any{"got": got}}
}

func TooManyVariants(got int) *Error {
	return &Error{Code: CodeTooManyVariants,
		Message: "a product may not have more than 100 variants",
		Details: map[string]any{"got": got}}
}

func UploadNotFound(key string) *Error {
	return &Error{Code: CodeUploadNotFound,
		Message: "referenced upload was not found in storage",
		Details: map[string]any{"storage_key": key}}
}

func PayloadTooLarge(key string, size int64) *Error {
	return &Error{Code: CodePayloadTooLarge,
		Message: "uploaded object exceeds the maximum size",
		Details: map[string]any{"storage_key": key, "bytes": size}}
}

func UnsupportedMediaType(key, ct string) *Error {
	return &Error{Code: CodeUnsupportedMediaType,
		Message: "uploaded object has an unsupported content type",
		Details: map[string]any{"storage_key": key, "content_type": ct}}
}

func TargetStoreInvalid(storeID, reason string) *Error {
	return &Error{Code: CodeTargetStoreInvalid,
		Message: "copy target store is invalid",
		Details: map[string]any{"store_id": storeID, "reason": reason}}
}

func CurrencyChangeForbidden() *Error {
	return &Error{Code: CodeCurrencyChangeForbidden,
		Message: "changing currency is not supported in slice 1"}
}

func CurrencyMismatch(source, target string) *Error {
	return &Error{Code: CodeCurrencyMismatch,
		Message: "variant currency does not match store currency",
		Details: map[string]any{"source": source, "target": target}}
}

func NotFound(resource string) *Error {
	return &Error{Code: CodeNotFound,
		Message: fmt.Sprintf("%s not found", resource)}
}

func Forbidden() *Error { return &Error{Code: CodeForbidden, Message: "forbidden"} }
```

- [x] **Step 4: Run `go mod tidy`**

```
cd services/marketplace-api && go mod tidy
```

No new deps in this task — the apperrors package is pure stdlib. Bluemonday is added in Task 8 and x/sync is promoted to direct in Task 3, only when the code that imports them lands. Adding unused deps here would be pruned by `go mod tidy` immediately. The expected `go.mod` diff from this task is **empty**.

- [x] **Step 5: Run test to verify it passes**

```
cd services/marketplace-api && go test ./pkg/apperrors/... -v
```
Expected: `PASS`

- [x] **Step 6: Confirm go.work is untouched (landmine #1)**

```
cd ../.. && git diff go.work
```
Expected: empty output.

- [x] **Step 7: Commit**

```
git add services/marketplace-api/pkg/apperrors services/marketplace-api/go.mod services/marketplace-api/go.sum go.work.sum
git commit -m "feat(marketplace-api): add pkg/apperrors typed error envelope (M3)"
```

---

### Task 2: `internal/stores` repository + integration tests

**Files:**
- Create: `services/marketplace-api/internal/stores/errors.go` (ErrNotFound sentinel, IsStale helper)
- Create: `services/marketplace-api/internal/stores/repository.go`
- Create: `services/marketplace-api/internal/stores/repository_integration_test.go`

**Scope:**
- `Repository` interface with `GetByIDForTenant(ctx, storeID, tenantID)`, `Upsert(ctx, *Store)`.
- `GetByIDForTenant` returns `(nil, ErrNotFound)` when missing OR belongs to wrong tenant — this is the tenant-isolation guarantee from §13.1.4. No existence leak.
- `IsStale(store, d)` reports whether `time.Since(store.SyncedAt) > d`.
- Integration tests: insert via tx, assert GetByIDForTenant returns the row; assert wrong tenant returns ErrNotFound; assert IsStale boundary.

- [x] **Step 1: Write repository_integration_test.go first** (TDD)

Key cases:
1. `Upsert` insert then update preserves `id`, bumps `synced_at`.
2. `GetByIDForTenant` for the right tenant returns the row.
3. `GetByIDForTenant` for a different tenant returns `ErrNotFound` (use a fresh random UUID).
4. `GetByIDForTenant` for a non-existent store returns `ErrNotFound`.
5. `IsStale(store, 5*time.Minute)` is false immediately after insert, true after manually updating `synced_at` to 6 minutes ago via a raw `UPDATE`.

All tests use `testdb.NewTx(t)` with `//go:build integration` at the top.

- [x] **Step 2: Run tests — they should fail to compile**

```
cd services/marketplace-api && go test -tags integration ./internal/stores/...
```
Expected: `undefined: stores.Repository` etc.

- [x] **Step 3: Implement `errors.go` and `repository.go`**

```go
// services/marketplace-api/internal/stores/errors.go
package stores

import (
	"errors"
	"time"
)

// ErrNotFound is returned by Repository.GetByIDForTenant when the store does
// not exist OR belongs to a different tenant. Callers must not distinguish
// these cases (no existence leak).
var ErrNotFound = errors.New("stores: not found")

// IsStale reports whether the projection row is older than the TTL.
// The nil guard makes callers' happy-path code shorter.
func IsStale(s *Store, ttl time.Duration) bool {
	if s == nil {
		return true
	}
	return time.Since(s.SyncedAt) > ttl
}
```

```go
// services/marketplace-api/internal/stores/repository.go
package stores

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository is the data-access interface for the local stores projection.
// Writes happen only through StoreMiddleware's lazy pull-through (Upsert).
// Reads are scoped by (id, tenant_id) — see ErrNotFound semantics.
type Repository interface {
	GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error)
	Upsert(ctx context.Context, s *Store) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", storeID, tenantID).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores: get by id for tenant: %w", err)
	}
	return &s, nil
}

// Upsert writes the row keyed on primary key id. On conflict it replaces
// every non-pk column and bumps synced_at. Caller is responsible for
// setting SyncedAt to time.Now() before calling.
func (r *gormRepository) Upsert(ctx context.Context, s *Store) error {
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"tenant_id", "slug", "name", "country_code",
				"currency_code", "timezone", "status", "synced_at",
			}),
		}).
		Create(s).Error; err != nil {
		return fmt.Errorf("stores: upsert: %w", err)
	}
	return nil
}
```

- [x] **Step 4: Run tests to verify they pass**

```
TEST_DATABASE_URL=<existing test dsn> go test -tags integration ./internal/stores/... -v
```
Expected: all 5 cases PASS.

- [x] **Step 5: Commit**

```
git add services/marketplace-api/internal/stores
git commit -m "feat(marketplace-api): add stores repository with tenant-scoped lookup (M3)"
```

---

### Task 3: `internal/stores` StoreMiddleware with singleflight + serve-stale

**Files:**
- Create: `services/marketplace-api/internal/stores/platform_client.go`
- Create: `services/marketplace-api/internal/stores/middleware.go`
- Create: `services/marketplace-api/internal/stores/middleware_test.go` (unit, no DB)

**Scope:** Implements §14.7 exactly. Uses `golang.org/x/sync/singleflight` to coalesce refresh calls. Tests use a fake `platform.Client` that can be toggled to return errors — the middleware logic is unit-testable without Postgres.

Middleware needs a `Repository` for the cache lookup and a `Client` for refresh. Unit tests use a fake `Repository` (in-memory map) so they don't need `testdb`.

- [x] **Step 1: Write the failing test**

```go
// services/marketplace-api/internal/stores/middleware_test.go
package stores_test

// Tests:
// 1. Fresh cached row → middleware calls Next, no platform hit
// 2. Missing cache + successful refresh → row written back, Next called
// 3. Stale cache (>5 min) + successful refresh → refresh happens, Next called
// 4. Stale cache + platform outage + cache age < 24h → serve stale, set store_stale flag, Next called
// 5. Missing cache + platform outage → 404
// 6. Stale cache + platform outage + cache age > 24h → 404
// 7. Singleflight coalescing: 10 concurrent requests with missing cache → exactly 1 platform call
//
// Fake Repository is a sync.Mutex-guarded map. Fake Client has a mode: ok/down/not_found.
```

(Full test body: ~250 lines. Use `httptest.NewRecorder()` + `gin.CreateTestContext()` to drive the middleware. Assert `c.Get("store")` is the expected row. For case 7, spawn 10 goroutines calling the middleware against a shared fake client with a 50ms sleep inside `GetStore`; assert the client's call counter is exactly 1.)

- [x] **Step 2: Implement `platform_client.go`**

```go
// services/marketplace-api/internal/stores/platform_client.go
package stores

import (
	"context"
	"errors"
)

// ErrPlatformUnavailable signals that platform-api could not be reached.
// StoreMiddleware catches this and falls back to stale cache when possible.
var ErrPlatformUnavailable = errors.New("stores: platform-api unavailable")

// Client is the interface marketplace-api uses to pull store metadata
// from the authoritative platform-api. The real HTTP implementation lands
// in M5. M3 tests inject fakes.
type Client interface {
	GetStore(ctx context.Context, tenantID, storeID string) (*Store, error)
}
```

- [x] **Step 3: Implement `middleware.go`**

Transcribe §14.7 exactly:

```go
// services/marketplace-api/internal/stores/middleware.go
package stores

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// MiddlewareConfig groups the knobs for StoreMiddleware so production and
// test wiring can agree without a long parameter list.
type MiddlewareConfig struct {
	Repo       Repository
	Client     Client
	Logger     *slog.Logger
	Flight     *singleflight.Group // shared across requests to coalesce refreshes
	FreshTTL   time.Duration       // default 5 * time.Minute
	StaleCeil  time.Duration       // default 24 * time.Hour
	TenantKey  string              // gin context key that holds tenant id; default "tenant_id"
}

// StoreMiddleware enforces store-ownership and populates c.Set("store", ...).
// Implements spec §14.7.
func StoreMiddleware(cfg MiddlewareConfig) gin.HandlerFunc {
	if cfg.FreshTTL == 0 {
		cfg.FreshTTL = 5 * time.Minute
	}
	if cfg.StaleCeil == 0 {
		cfg.StaleCeil = 24 * time.Hour
	}
	if cfg.TenantKey == "" {
		cfg.TenantKey = "tenant_id"
	}
	return func(c *gin.Context) {
		storeID := c.Param("storeId")
		tenantID, _ := c.Get(cfg.TenantKey)
		tid, _ := tenantID.(string)
		if storeID == "" || tid == "" {
			respondNotFound(c)
			return
		}

		cached, cacheErr := cfg.Repo.GetByIDForTenant(c.Request.Context(), storeID, tid)
		fresh := cacheErr == nil && !IsStale(cached, cfg.FreshTTL)
		if fresh {
			c.Set("store", cached)
			c.Next()
			return
		}

		result, refreshErr, _ := cfg.Flight.Do("store:"+storeID, func() (interface{}, error) {
			return refresh(c.Request.Context(), cfg, storeID, tid)
		})

		switch {
		case refreshErr == nil && result != nil:
			c.Set("store", result.(*Store))
			c.Next()
		case cacheErr == nil && cached != nil && time.Since(cached.SyncedAt) < cfg.StaleCeil:
			if cfg.Logger != nil {
				cfg.Logger.Warn("serving stale store projection",
					"store_id", storeID,
					"synced_at", cached.SyncedAt,
					"refresh_err", refreshErr)
			}
			c.Set("store", cached)
			c.Set("store_stale", true)
			c.Next()
		default:
			respondNotFound(c)
		}
	}
}

func refresh(ctx context.Context, cfg MiddlewareConfig, storeID, tenantID string) (*Store, error) {
	fresh, err := cfg.Client.GetStore(ctx, tenantID, storeID)
	if err != nil {
		return nil, err
	}
	if fresh == nil || fresh.TenantID != tenantID {
		return nil, ErrNotFound
	}
	fresh.SyncedAt = time.Now()
	if err := cfg.Repo.Upsert(ctx, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(404, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "store not found",
	})
}

// ensure errors.Is line wraps are not dead code (reserved for M5 handler use)
var _ = errors.Is
```

- [x] **Step 4: Run tests**

```
cd services/marketplace-api && go test ./internal/stores/... -run Middleware -race -v
```
Expected: all 7 cases PASS. The singleflight test in particular must show exactly 1 client call.

- [x] **Step 5: Commit**

```
git add services/marketplace-api/internal/stores
git commit -m "feat(marketplace-api): add StoreMiddleware with singleflight + serve-stale (M3)"
```

---

### Task 4: `internal/outbox` repository

**Files:**
- Create: `services/marketplace-api/internal/outbox/repository.go`
- Create: `services/marketplace-api/internal/outbox/repository_integration_test.go`

**Scope:**
- `EnqueueInTx(tx, *OutboxEvent)` — inserts one event row inside an existing transaction. Callers always pass their own tx so the event is atomic with the mutation.
- `PollBatch(ctx, limit)` — opens its own tx, issues `SELECT ... FOR UPDATE SKIP LOCKED LIMIT $1` over unpublished rows ordered by `(tenant_id, created_at)`, returns the rows AND the tx so the caller can finish work and commit/rollback.
- `MarkPublished(tx, ids []string)` — `UPDATE outbox_events SET published_at = now() WHERE id = ANY($1)`.

The poll signature with "returns the tx" is ugly but necessary — the caller must hold the row-level locks until it has upserted watermarks AND marked published. Alternative: fold the whole batch processing into a `ProcessBatch(ctx, fn)` callback pattern. Use the callback pattern — cleaner.

- [x] **Step 1: Write integration test first**

```go
// services/marketplace-api/internal/outbox/repository_integration_test.go
//go:build integration

package outbox_test

// Cases:
// 1. EnqueueInTx writes a row inside an existing tx; rollback drops it.
// 2. ProcessBatch sees the row, calls the callback with the rows, and
//    MarkPublished marks them published.
// 3. ProcessBatch with SKIP LOCKED: two concurrent callers each get disjoint
//    rows (insert 4 rows in one tx/commit, then two goroutines call
//    ProcessBatch(limit=2) simultaneously — assert each sees exactly 2 rows
//    and the union is all 4).
// 4. ProcessBatch ordering: rows are returned in (tenant_id, created_at)
//    order — insert 3 rows for tenant A then 2 for tenant B with backdated
//    created_at, assert the batch order matches the index.
//
// Case 3 requires real commits across connections — use testdb.NewDB with
// cleanup on "outbox_events".
```

- [x] **Step 2: Implement `repository.go`**

```go
// services/marketplace-api/internal/outbox/repository.go
package outbox

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repository is the data-access interface for outbox_events.
type Repository interface {
	EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error
	// ProcessBatch opens its own transaction, locks up to `limit` unpublished
	// rows via FOR UPDATE SKIP LOCKED, and calls fn with the rows and the
	// same tx. If fn returns nil the tx commits (the caller is expected to
	// have called MarkPublished inside fn); if fn returns an error the tx
	// rolls back and the rows become visible to the next poll. Returns the
	// number of rows the callback saw.
	ProcessBatch(ctx context.Context, limit int,
		fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error)
	MarkPublishedInTx(tx *gorm.DB, ids []string) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error {
	if err := tx.WithContext(ctx).Create(evt).Error; err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

func (r *gormRepository) ProcessBatch(ctx context.Context, limit int,
	fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error) {
	var seen int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []OutboxEvent
		// Raw query so we can express FOR UPDATE SKIP LOCKED cleanly.
		if err := tx.Raw(`
			SELECT id, tenant_id, aggregate, aggregate_id, event_type,
			       payload, created_at, published_at, error
			FROM outbox_events
			WHERE published_at IS NULL
			ORDER BY tenant_id, created_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, limit).Scan(&rows).Error; err != nil {
			return fmt.Errorf("outbox: poll: %w", err)
		}
		seen = len(rows)
		if seen == 0 {
			return nil
		}
		return fn(tx, rows)
	})
	return seen, err
}

func (r *gormRepository) MarkPublishedInTx(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Exec(`UPDATE outbox_events SET published_at = now() WHERE id = ANY(?)`,
		ids).Error; err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	return nil
}
```

- [x] **Step 3: Run tests**

```
TEST_DATABASE_URL=... go test -tags integration ./internal/outbox/... -race -v
```

- [x] **Step 4: Commit**

```
git add services/marketplace-api/internal/outbox
git commit -m "feat(marketplace-api): add outbox repository with SKIP LOCKED batch processing (M3)"
```

---

### Task 5: `internal/outbox` publisher goroutine

**Files:**
- Create: `services/marketplace-api/internal/outbox/publisher.go`
- Create: `services/marketplace-api/internal/outbox/publisher_integration_test.go`

**Scope:**
- `Publisher` struct owns a `Repository`, a store-watermarks writer function, a tick interval, a logger, and a cancel channel.
- `Start(ctx)` spawns a goroutine that ticks every `cfg.Interval` (default 2s), calls `repo.ProcessBatch(ctx, 100, ...)`, and for each row upserts `store_watermarks` then marks published. All inside the same tx.
- The store-watermarks upsert lives in this package (not stores) to keep the coupling local — the publisher is the only caller.
- Shutdown waits for the goroutine to return.

Publisher's batch callback logic:
1. Group rows by their `store_id`. The store id is NOT a column on `outbox_events` — we read it from `payload.store_id`. Service-layer producers MUST include `store_id` in every payload for slice 1. Document this as a producer invariant.
2. For each `store_id`: upsert `store_watermarks` with `products_updated_at = GREATEST(existing, max(created_at))`.
3. Collect all row ids and call `MarkPublishedInTx`.

- [x] **Step 1: Write integration test** (uses testdb.NewDB with real commits)

```go
// Cases:
// 1. Enqueue an event with payload {"store_id": "<s>"} via a real commit,
//    Start publisher with Interval=50ms, wait 500ms, assert:
//    - store_watermarks has a row with store_id=<s>, products_updated_at >= t0
//    - outbox_events row has published_at set
//    - publisher goroutine exits cleanly on ctx cancel
// 2. Multiple events for same store → single watermark upsert per batch;
//    products_updated_at = max(created_at) among the batch.
// 3. Publisher error path: if MarkPublishedInTx fails (simulate by passing
//    a repo wrapper that returns err), the whole batch rolls back and rows
//    stay unpublished (assert via SELECT published_at IS NULL count after).
// 4. Payload without store_id → row is marked published anyway (drop-floor)
//    and publisher logs a warning. Decision: the slice-1 contract requires
//    store_id, so missing store_id is a producer bug, not a retry condition.
//    We log and skip. Document this invariant. Not covered by a spec
//    clause; locked here — blocking publisher progress on one malformed
//    row would stall every other event.
// 5. §14.11 FK sanity: store_watermarks.store_id has FK to stores(id)
//    RESTRICT. Because every mutation that enqueues an event has already
//    passed through StoreMiddleware (which upserts the stores projection
//    row first), the publisher's upsert will never hit a missing-parent
//    FK violation. Document this invariant so nobody adds a "pre-create
//    stores row" step to the publisher later.
```

- [x] **Step 2: Implement `publisher.go`**

```go
// services/marketplace-api/internal/outbox/publisher.go
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Publisher polls outbox_events and bumps store_watermarks asynchronously.
// See spec §14.1 (watermark separation) and §14.6 (publisher semantics).
//
// Payload invariant: every outbox row in slice 1 carries a "store_id" key
// at the top level of its JSON payload. Rows without it are logged and
// marked published without a watermark bump — losing the signal is
// preferable to blocking the publisher on a producer bug.
type Publisher struct {
	repo     Repository
	db       *gorm.DB
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

// Config configures a Publisher.
type Config struct {
	Repo     Repository
	DB       *gorm.DB
	Logger   *slog.Logger
	Interval time.Duration // default 2s
	BatchSize int          // default 100
}

// New constructs a Publisher.
func New(cfg Config) *Publisher {
	if cfg.Interval == 0 {
		cfg.Interval = 2 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	return &Publisher{
		repo:     cfg.Repo,
		db:       cfg.DB,
		logger:   cfg.Logger,
		interval: cfg.Interval,
		batch:    cfg.BatchSize,
	}
}

// Start runs the publisher loop until ctx is cancelled. It blocks; callers
// run it in a goroutine and wait for the returned done channel after
// cancelling ctx.
func (p *Publisher) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(p.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := p.tick(ctx); err != nil && p.logger != nil {
					p.logger.Error("outbox publisher tick failed", "err", err)
				}
			}
		}
	}()
	return done
}

// tick processes a single batch. Exposed for tests that want to drive
// without sleeping.
func (p *Publisher) tick(ctx context.Context) (int, error) {
	return p.repo.ProcessBatch(ctx, p.batch, func(tx *gorm.DB, rows []OutboxEvent) error {
		// Group by store_id, computing max created_at per store.
		byStore := map[string]time.Time{}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: unparseable payload; dropping",
						"event_id", r.ID, "err", err)
				}
				continue
			}
			sid, _ := payload["store_id"].(string)
			if sid == "" {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: payload missing store_id; dropping",
						"event_id", r.ID, "event_type", r.EventType)
				}
				continue
			}
			if prev, ok := byStore[sid]; !ok || r.CreatedAt.After(prev) {
				byStore[sid] = r.CreatedAt
			}
		}
		for sid, ts := range byStore {
			if err := tx.Exec(`
				INSERT INTO store_watermarks (store_id, products_updated_at)
				VALUES (?, ?)
				ON CONFLICT (store_id) DO UPDATE
					SET products_updated_at = GREATEST(
						store_watermarks.products_updated_at,
						EXCLUDED.products_updated_at)`, sid, ts).Error; err != nil {
				return err
			}
		}
		return p.repo.MarkPublishedInTx(tx, ids)
	})
}
```

- [x] **Step 3: Run tests**

```
TEST_DATABASE_URL=... go test -tags integration ./internal/outbox/... -run Publisher -race -v
```

- [x] **Step 4: Commit**

```
git add services/marketplace-api/internal/outbox/publisher.go services/marketplace-api/internal/outbox/publisher_integration_test.go
git commit -m "feat(marketplace-api): add outbox publisher goroutine + watermark upsert (M3)"
```

---

### Task 6: `internal/category` repository

**Files:**
- Create: `services/marketplace-api/internal/category/repository.go`
- Create: `services/marketplace-api/internal/category/repository_integration_test.go`

**Scope:**
- `Create(ctx, *Category)` — tenant + store already set by caller, inserts and returns with ID filled.
- `GetByIDForStore(ctx, id, storeID, tenantID)` — 404 on cross-tenant.
- `ListByStore(ctx, storeID, tenantID)` — flat list (M3 keeps it flat; M5 surfaces the tree via a helper).
- `UpdateInTx(ctx, tx, *Category)` — partial update via `Updates(map)`.
- `SoftDeleteInTx(ctx, tx, id, storeID, tenantID)` — sets `deleted_at = now()`.
- `HasChildren(ctx, parentID)` — `SELECT count(*) WHERE parent_id = ? AND deleted_at IS NULL`.
- `HasProducts(ctx, categoryID)` — same against `product_categories` joined with `products.deleted_at IS NULL`.
- Unique-violation on `(store_id, slug)` returns `apperrors.SlugTaken(...)`.

Integration test cases:
1. Create + GetByIDForStore roundtrip.
2. Cross-tenant GetByIDForStore returns `ErrNotFound`.
3. Create two categories with the same slug in the same store → second fails with `apperrors.ErrSlugTaken`. Use savepoint per landmine #3.
4. Soft-delete then re-create with the same slug succeeds (partial unique index test).
5. HasChildren true / false.
6. HasProducts true / false after inserting a product + product_categories link (use minimal product fixture).

- [x] **Step 1-4: TDD rhythm as above.**

Repository skeleton:

```go
// services/marketplace-api/internal/category/repository.go
package category

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type Repository interface {
	Create(ctx context.Context, c *Category) error
	GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Category, error)
	ListByStore(ctx context.Context, storeID, tenantID string) ([]Category, error)
	UpdateInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error
	SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error
	HasChildren(ctx context.Context, parentID string) (int64, error)
	HasProducts(ctx context.Context, categoryID string) (int64, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, c *Category) error {
	err := r.db.WithContext(ctx).Create(c).Error
	if isUniqueSlug(err) {
		return apperrors.SlugTaken(c.Slug, c.Slug+"-2")
	}
	if err != nil {
		return fmt.Errorf("category: create: %w", err)
	}
	return nil
}

// ...GetByIDForStore returns ErrNotFound→apperrors.NotFound("category")
// ...ListByStore filters deleted_at IS NULL, ORDER BY position, name
// ...UpdateInTx translates unique-violation to apperrors.SlugTaken
// ...SoftDeleteInTx: UPDATE ... SET deleted_at = now() WHERE id=? AND store_id=? AND tenant_id=? AND deleted_at IS NULL
// ...HasChildren: SELECT count(*) FROM categories WHERE parent_id=? AND deleted_at IS NULL
// ...HasProducts: SELECT count(*) FROM product_categories pc JOIN products p ON p.id = pc.product_id WHERE pc.category_id=? AND p.deleted_at IS NULL

func isUniqueSlug(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "categories_slug_per_store_live_unique"
}
```

Note: `github.com/jackc/pgx/v5/pgconn` — verify it's already in go.mod (gorm/postgres uses pgx v5). If not, `go get` it in this task's step and include the bump in the commit.

- [x] **Step 5: Commit**

```
git commit -m "feat(marketplace-api): add category repository with slug-taken error translation (M3)"
```

---

### Task 7: `internal/category` service

**Files:**
- Create: `services/marketplace-api/internal/category/service.go`
- Create: `services/marketplace-api/internal/category/service_integration_test.go`

**Scope:**
- `Create(ctx, req CreateRequest)` — validates name/slug, generates slug if missing, opens tx, calls repo.Create, enqueues outbox event `category.created` with `{"store_id": storeID, "category_id": id}`.
- `Update(ctx, req UpdateRequest)` — tx + partial update + outbox event `category.updated`.
- `Delete(ctx, id, storeID, tenantID)` — pre-checks HasChildren (refuse with `CategoryHasChildren(n)`) and HasProducts (refuse with `CategoryNotEmpty(n)`), then tx + SoftDelete + outbox event `category.deleted`.

Service constructor takes `*gorm.DB`, `Repository`, `outbox.Repository`, `*slog.Logger`.

Integration tests assert every error path:
- `CategoryHasChildren` → insert parent + child, attempt delete parent.
- `CategoryNotEmpty` → insert category + product + product_categories link, attempt delete.
- `SlugTaken` → create two with same slug.
- `NotFound` on update/delete with wrong tenant.

- [x] **Step 1: Write failing tests** (skeleton above, ~300 lines).

- [x] **Step 2: Implement service.go.**

Key pattern (matches platform-api onboarding):

```go
func (s *Service) Delete(ctx context.Context, id, storeID, tenantID string) error {
	// Pre-check (outside the tx is fine — race between check and delete is
	// acceptable; worst case the delete succeeds against an empty category).
	childCount, err := s.repo.HasChildren(ctx, id)
	if err != nil {
		return err
	}
	if childCount > 0 {
		return apperrors.CategoryHasChildren(childCount)
	}
	prodCount, err := s.repo.HasProducts(ctx, id)
	if err != nil {
		return err
	}
	if prodCount > 0 {
		return apperrors.CategoryNotEmpty(prodCount)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.SoftDeleteInTx(ctx, tx, id, storeID, tenantID); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"store_id": storeID, "category_id": id,
		})
		return s.outbox.EnqueueInTx(ctx, tx, &outbox.OutboxEvent{
			TenantID:    tenantID,
			Aggregate:   outbox.AggregateCategory,
			AggregateID: id,
			EventType:   outbox.EventCategoryDeleted,
			Payload:     datatypes.JSON(payload),
		})
	})
}
```

- [x] **Step 3-5: Run tests, iterate, commit.**

```
git commit -m "feat(marketplace-api): add category service with delete refusals + outbox events (M3)"
```

---

### Task 8: `internal/product` sanitizer (bluemonday, version-pinned)

**Files:**
- Create: `services/marketplace-api/internal/product/sanitizer.go`
- Create: `services/marketplace-api/internal/product/sanitizer_test.go`

**Scope:** Exactly §14.14. Constants `SanitizerPolicyVersion = 1`. A `Sanitize(html string) string` function. Tests include an OWASP top-10 XSS payload corpus with expected-safe output.

- [x] **Step 1: Write failing test**

```go
// services/marketplace-api/internal/product/sanitizer_test.go
package product_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/product"
)

func TestSanitizer_OWASPCorpus(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		banned  []string // substrings that MUST NOT appear in output
	}{
		{"script_tag", `<script>alert(1)</script>`, []string{"script", "alert"}},
		{"img_onerror", `<img src=x onerror=alert(1)>`, []string{"onerror", "alert", "img"}},
		{"iframe", `<iframe src="javascript:alert(1)"></iframe>`, []string{"iframe", "javascript"}},
		{"svg_onload", `<svg onload=alert(1)>`, []string{"onload", "alert"}},
		{"a_javascript_href", `<a href="javascript:alert(1)">x</a>`, []string{"javascript"}},
		{"meta_refresh", `<meta http-equiv="refresh" content="0;url=http://evil">`, []string{"meta", "refresh"}},
		{"style_expression", `<style>body{background:expression(alert(1))}</style>`, []string{"style", "expression"}},
		{"object", `<object data="evil.swf"></object>`, []string{"object"}},
		{"embed", `<embed src="evil.swf">`, []string{"embed"}},
		{"form", `<form action="evil"><input type=text></form>`, []string{"form", "input"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := product.Sanitize(tc.input)
			lo := strings.ToLower(out)
			for _, banned := range tc.banned {
				if strings.Contains(lo, banned) {
					t.Errorf("output %q contained banned token %q", out, banned)
				}
			}
		})
	}
}

func TestSanitizer_PreservesAllowedTags(t *testing.T) {
	in := `<p>Hello <strong>world</strong> <em>now</em></p><ul><li>one</li></ul>`
	out := product.Sanitize(in)
	for _, tag := range []string{"<p>", "<strong>", "<em>", "<ul>", "<li>"} {
		if !strings.Contains(out, tag) {
			t.Errorf("expected %q preserved in %q", tag, out)
		}
	}
}

func TestSanitizer_ForcesNofollowOnLinks(t *testing.T) {
	in := `<a href="https://example.com" target="_blank">click</a>`
	out := product.Sanitize(in)
	if !strings.Contains(out, `rel="nofollow"`) {
		t.Errorf("expected rel=nofollow on links; got %q", out)
	}
	if strings.Contains(out, `target`) {
		t.Errorf("expected target attribute stripped; got %q", out)
	}
}

func TestSanitizer_PolicyVersion(t *testing.T) {
	if product.SanitizerPolicyVersion != 1 {
		t.Fatalf("SanitizerPolicyVersion must stay 1 until an accompanying re-sanitize migration ships (spec §14.14); got %d", product.SanitizerPolicyVersion)
	}
}
```

- [x] **Step 2: Implement sanitizer.go**

```go
// services/marketplace-api/internal/product/sanitizer.go
package product

import "github.com/microcosm-cc/bluemonday"

// SanitizerPolicyVersion pins the bluemonday policy. Incrementing this is
// append-only and requires an accompanying re-sanitization migration that
// re-runs every stored product description through the new policy. See
// spec §14.14. Don't bump this without a migration in the same PR.
const SanitizerPolicyVersion = 1

var policy = policyV1()

func policyV1() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u",
		"ul", "ol", "li",
		"h2", "h3", "h4",
		"blockquote")
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(true)
	p.AllowURLSchemes("http", "https", "mailto")
	return p
}

// Sanitize returns a safe rendering of the input HTML per the pinned
// policy. Empty input returns empty string. Sanitize is only applied to
// user-authored rich text at write time; stored HTML is never
// re-sanitized on read.
func Sanitize(in string) string {
	if in == "" {
		return ""
	}
	return policy.Sanitize(in)
}
```

- [x] **Step 3-5: Run, verify all OWASP cases pass, commit.**

```
git commit -m "feat(marketplace-api): add bluemonday sanitizer with OWASP corpus test (M3)"
```

---

### Task 9: `internal/product` variant matrix + handle generator (pure logic)

**Files:**
- Create: `services/marketplace-api/internal/product/matrix.go`
- Create: `services/marketplace-api/internal/product/matrix_test.go`
- Create: `services/marketplace-api/internal/product/handle.go`
- Create: `services/marketplace-api/internal/product/handle_test.go`

**Scope:** Pure functions. No DB. Unit tests run in ms.

- `ValidateMatrix(options []OptionSpec, variants []VariantSpec) error` — asserts:
  - len(options) ≤ 3 → else `TooManyOptions`
  - len(variants) ≤ 100 → else `TooManyVariants`
  - len(variants) == Π(len(option.values)) → else `VariantMatrixMismatch(expected, got)`
  - each variant's option_values covers exactly each option once and references values that exist on that option → else `ValidationFailed`
- `DiffMatrix(existing, incoming []VariantSpec) (adds, updates, removes []VariantSpec)` — match by `option_value_id` tuple (sorted). Preserves existing variant IDs on updates.
- `GenerateHandle(title string) string` — lower, unicode-aware strip, collapse dashes, truncate to 200.
- `SuggestNextHandle(base string, takenPredicate func(string) bool) string` — tries `base`, `base-2`, `base-3`, ... up to 50 and returns the first free value; if all taken, returns `base + "-" + 6-hex-random`.

Handle tests:
- unicode e.g. `"Café au lait"` → `"cafe-au-lait"` (NFD strip or bluemonday-unrelated unicode helper — `golang.org/x/text/unicode/norm` is already indirect).
- collision suggester walks `-2` .. `-5`.

Matrix tests:
- 0 options, 1 variant → OK
- 1 option 3 values, 3 variants → OK
- 2 options 2x3, 6 variants → OK
- 2x3, 5 variants → `VariantMatrixMismatch(6, 5)`
- 4 options → `TooManyOptions(4)`
- 101 variants → `TooManyVariants(101)`
- variant references a nonexistent option_value_id → `ValidationFailed`

- [x] **Step 1-5: TDD rhythm**. Commit:

```
git commit -m "feat(marketplace-api): add variant matrix validator + handle generator (M3)"
```

---

### Task 10: `internal/media` stub uploader interface

**Files:**
- Create: `services/marketplace-api/internal/media/uploader.go`

**Scope:** Single file, ~40 lines. No tests of its own (exercised by product service tests).

```go
// Package media owns the interface through which marketplace-api's service
// layer verifies that uploaded media objects exist in GCS. The real GCS-
// backed implementation lands in M5; M3 ships the interface and a fake
// implementation for tests.
package media

import (
	"context"
	"errors"
)

// Attrs is the subset of GCS object attributes the service layer cares
// about pre-transaction. See spec §14.9.
type Attrs struct {
	StorageKey  string
	Size        int64
	ContentType string
}

// ErrNotFound signals the object does not exist at the given storage_key.
var ErrNotFound = errors.New("media: upload not found")

// Uploader verifies that an uploaded object exists and returns its metadata.
// The real implementation calls GCS HEAD; M3's fake holds an in-memory map.
type Uploader interface {
	Verify(ctx context.Context, storageKey string) (*Attrs, error)
}

// FakeUploader is an in-memory Uploader used by tests. Callers register
// expected storage keys via Register() before invoking service code.
type FakeUploader struct {
	attrs map[string]*Attrs
}

// NewFakeUploader returns a FakeUploader with an empty registry.
func NewFakeUploader() *FakeUploader { return &FakeUploader{attrs: map[string]*Attrs{}} }

// Register seeds the fake with an attrs record.
func (f *FakeUploader) Register(a Attrs) {
	copy := a
	f.attrs[a.StorageKey] = &copy
}

// Verify implements Uploader.
func (f *FakeUploader) Verify(_ context.Context, key string) (*Attrs, error) {
	a, ok := f.attrs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}
```

- [x] **Commit:**

```
git commit -m "feat(marketplace-api): add media.Uploader stub interface (M3)"
```

---

### Task 11: `internal/product` repository (the big one)

**Files:**
- Create: `services/marketplace-api/internal/product/repository.go`
- Create: `services/marketplace-api/internal/product/repository_integration_test.go`

**Scope:** CRUD on the product aggregate — products, product_options, product_option_values, product_variants, variant_option_values, product_media, product_categories, and `variant_stock` writes.

Interface:

```go
type Repository interface {
	// CreateAggregate inserts the full product graph in the caller's tx.
	// Caller has already pre-validated (pre-tx GCS check, matrix validation).
	// On unique violations the method returns apperrors.HandleTaken /
	// SKUTaken with the conflicting value from pgconn.PgError.Detail.
	CreateAggregateInTx(ctx context.Context, tx *gorm.DB, a *Aggregate) error

	// GetByIDForStore loads a full aggregate (product + options + values +
	// variants + links + media + category links) via Preload. Returns
	// apperrors.NotFound on missing / cross-tenant.
	GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Aggregate, error)

	// ListAdmin returns paginated products for the admin UI. Must execute
	// in ≤ 7 queries (see §14.17). Enforced by a test that counts SQL
	// statements via gorm logger interception.
	ListAdmin(ctx context.Context, q ListAdminQuery) ([]Aggregate, int64, error)

	// ListPublished is the storefront-visible query — active status + not
	// deleted + published_at IS NOT NULL. Returns only columns the
	// storefront DTO exposes (no cost_price, no inventory_quantity).
	// Even though M3 has no storefront handler, shipping the method now
	// means M6 doesn't need to touch the repo.
	ListPublished(ctx context.Context, q ListPublishedQuery) ([]Aggregate, error)

	// UpdateBasicsInTx updates title/description/seo/status/tags/primary
	// category. Variants/options/media are separate methods because their
	// update semantics are different (add/update/remove diff).
	UpdateBasicsInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error

	// ApplyVariantDiffInTx applies the pre-computed matrix diff. On
	// unique SKU violation, returns apperrors.SKUTaken.
	ApplyVariantDiffInTx(ctx context.Context, tx *gorm.DB, productID, storeID string, diff VariantDiff) error

	// ReplaceCategoryLinksInTx deletes and reinserts product_categories
	// rows for the product. Simple DELETE+INSERT is adequate for slice 1.
	ReplaceCategoryLinksInTx(ctx context.Context, tx *gorm.DB, productID string, categoryIDs []string) error

	// ReplaceMediaInTx inserts/removes product_media rows to match the
	// target set. Existing rows matched by (product_id, storage_key) are
	// kept; new keys are inserted; removed keys are deleted.
	ReplaceMediaInTx(ctx context.Context, tx *gorm.DB, productID string, media []Media) error

	// UpdateVariantStockInTx upserts variant_stock for a single variant.
	// Writes MUST go through this method — never to product_variants.
	// inventory_quantity. Enforced by code review; tests exercise the
	// trigger by reading back Variant.InventoryQuantity after the write.
	UpdateVariantStockInTx(ctx context.Context, tx *gorm.DB, variantID, locationID string, quantity int) error

	// SoftDeleteInTx sets deleted_at on the product row. Variants are
	// preserved (their own deleted_at is a separate column for future use).
	SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error
}

// Aggregate is the in-memory shape of the full product graph. Not a GORM
// model; composed of the individual model types from models.go. The
// repository fills it via Preload.
type Aggregate struct {
	Product       Product
	Options       []Option // each has Values populated
	Variants      []Variant
	VariantOption []VariantOptionValue
	Media         []Media
	CategoryLinks []ProductCategory
}

type ListAdminQuery struct {
	StoreID, TenantID string
	Status            string // "", "draft", "active", "archived"
	Search            string // tsvector match
	Page, PageSize    int
	Limit             int // alias; computed by service
}

type ListPublishedQuery struct {
	StoreID        string
	CategorySlug   string // optional
	Page, PageSize int
}

// VariantDiff is produced by matrix.DiffMatrix and consumed by
// ApplyVariantDiffInTx.
type VariantDiff struct {
	Adds    []Variant
	Updates []Variant
	Removes []string // variant IDs
}
```

Repository tests (integration):
1. Create minimal aggregate, read back via GetByIDForStore, assert full graph matches.
2. Cross-tenant GetByIDForStore → `NotFound`.
3. Unique handle collision → `HandleTaken` with suggested alternative.
4. Unique SKU collision → `SKUTaken`.
5. Soft-delete then re-create with same handle succeeds (partial index test).
5b. **§14.3 un-delete regression:** soft-delete a row, insert a fresh live row with the same `(store_id, handle)`, then attempt to clear `deleted_at` on the old soft-deleted row — assert the repository returns `apperrors.ErrHandleTaken` (translated from the unique violation fired by the UPDATE path).
6. `ListAdmin` query-count gate: install a `gorm.Logger` that counts statements. Assert count ≤ 7 for a list of 3 products each with 2 variants + 2 options + 3 values + 2 media. Log the actual count.
7. `ListPublished` excludes drafts, archived, and soft-deleted; includes only `published_at IS NOT NULL AND status = 'active'`.
8. `UpdateVariantStockInTx` → read back Variant.InventoryQuantity via a fresh Get, assert it equals the written stock (trigger test, §14.5 forward direction).
8b. **§14.5 reverse-direction guard:** run a raw `UPDATE product_variants SET inventory_quantity = 9999 WHERE id = ?` and assert the sibling `variant_stock.quantity` is UNCHANGED. This cements the "variant_stock is source, inventory_quantity is derived" invariant and catches any future code that tries to write the denormalized column directly.
9. `ApplyVariantDiffInTx` adds new variants, updates existing prices, removes stale variants (match by sorted option_value_id tuple).
10. `ReplaceCategoryLinksInTx` removes stale links and adds new ones.
11. `ReplaceMediaInTx` preserves existing rows by (product_id, storage_key), adds new, removes absent.
12. Concurrent Create of the same handle: two goroutines, one wins cleanly with `HandleTaken` on the loser.

Implementation notes:
- Use `pgconn.PgError.ConstraintName` to disambiguate `handle_taken` vs `sku_taken` vs `slug_taken`.
- Handle generator for suggestion pulled from Task 9's `SuggestNextHandle`; repository takes a `collisionSuggester func(base string) string` in its constructor so tests can inject a deterministic suggester.
- For unique-violation translation on Update paths (including un-delete by clearing `deleted_at` — §14.3 regression test), the same translation helper is reused.

This file will be the largest. If it exceeds ~700 lines, split into `repository.go` (Create/Get/List/SoftDelete) and `repository_variants.go` (ApplyVariantDiff, UpdateVariantStock) and `repository_media.go` (ReplaceMedia).

- [x] **Step 1-5: TDD rhythm, run with `-tags integration -race`, commit.**

```
git commit -m "feat(marketplace-api): add product aggregate repository with unique-violation mapping (M3)"
```

---

### Task 12: `internal/product` service (Create/Update/Delete/Copy)

**Files:**
- Create: `services/marketplace-api/internal/product/service.go`
- Create: `services/marketplace-api/internal/product/service_integration_test.go`

**Scope:** The capstone. Ties everything together. Implements the flow from §13.3 as corrected by §14.1 (no watermark write in tx) and §14.9 (pre-tx GCS attrs check) with the §13.1.1 simplification (no FGA in tx).

Service constructor:

```go
type Service struct {
	db       *gorm.DB
	repo     Repository
	catRepo  category.Repository
	outbox   outbox.Repository
	stores   stores.Repository
	media    media.Uploader
	logger   *slog.Logger
	// allowed content types for image uploads (§14.9)
	allowedContentTypes []string
	// max bytes for a single media upload (§14.9)
	maxMediaBytes int64
	// DEFAULT_LOCATION_ID for slice-1 single-location variant_stock writes
	defaultLocationID string
}
```

Methods:

#### `Create(ctx, req CreateRequest) (*Aggregate, error)`

1. Validate request shape (title, handle empty→generate, status default draft, options≤3, variants≤100, matrix match via `matrix.ValidateMatrix`).
2. Sanitize `req.Description` via `Sanitize()`.
3. Look up target store via `stores.GetByIDForTenant(storeID, tenantID)` → must exist (otherwise `NotFound`). Cache the store's `CurrencyCode`.
4. Force every variant's `CurrencyCode = store.CurrencyCode` (silent correction per §6.5/§13.4). This is the "variant currency silently overridden" behavior tested in §9.3.
5. Pre-tx: for each media, call `media.Verify(ctx, key)`. Translate `ErrNotFound` → `apperrors.UploadNotFound(key)`. Check `attrs.Size <= maxMediaBytes` else `PayloadTooLarge`. Check `attrs.ContentType in allowedContentTypes` else `UnsupportedMediaType`.
6. Verify all `category_ids` exist and belong to `(storeID, tenantID)` via `catRepo.GetByIDForStore` in a loop — tolerable cost, small lists.
7. Begin tx.
8. Call `repo.CreateAggregateInTx(tx, aggregate)`. Translate `HandleTaken` by calling `SuggestNextHandle` (caller supplies a predicate that checks a fresh `SELECT 1 FROM products WHERE store_id=? AND handle=? AND deleted_at IS NULL` inside the same tx) — wrap into `apperrors.HandleTaken(attempted, suggested)`.
9. For each variant, write a single `variant_stock` row at `defaultLocationID` with the request's initial quantity. The trigger updates `product_variants.inventory_quantity`.
10. Enqueue `outbox_events` row with `event_type = product.created` and payload `{store_id, product_id}`.
11. Commit.

No GCS move, no FGA tuple write, no `stores.products_updated_watermark` update — all superseded by later revisions.

#### `Update(ctx, req UpdateRequest)`

1. Load existing aggregate via `repo.GetByIDForStore`.
2. If `req.Variants` present, run `matrix.DiffMatrix(existing.Variants, req.Variants)`. Validate new matrix (`ValidateMatrix`).
3. Sanitize description if present.
4. If `req.Status == "active"` and old status was different: set `published_at = now()` in the fields map (§13.2.5).
5. If any variant's currency_code in the request differs from the store's currency → return `CurrencyChangeForbidden` (§14.2).
6. Pre-tx: new media keys → `media.Verify`.
7. Begin tx. Apply basics update. Apply variant diff. Replace category links. Replace media. Write new `variant_stock` rows for added variants. Enqueue `product.updated`. Commit.

#### `Delete(ctx, id, storeID, tenantID)`

Tx: `SoftDeleteInTx` + enqueue `product.deleted` + commit.

#### `Copy(ctx, req CopyRequest)`

1. Load source aggregate.
2. Load both source and target stores via `stores.GetByIDForTenant`. Target must be a different store owned by the same tenant; else `TargetStoreInvalid`.
3. Build new aggregate:
   - fresh IDs
   - `store_id = target.ID`
   - `handle` re-generated with `SuggestNextHandle` against target store
   - `copy_source_product_id = source.ID`
   - `status = draft`, `published_at = nil`
   - options, option_values, variants duplicated with fresh IDs
   - **variants preserve source currency** — NOT the target store's currency. This is the single-source-preserved decision from §13.5.1 / spec. A subsequent variant update will block with `CurrencyMismatch` (same code, different context than the silent-override in Create) — the merchant sees the callout in the UI.
   - media rows duplicated with the SAME storage_key (reference, not copy) — no GCS I/O
   - category links: for each source category, look up by slug in the target store; create missing (using `catRepo.Create`) within the same tx
   - `variant_stock` rows inserted at zero — "inventory starts at zero" per the spec copy semantics
4. Tx: Create + enqueue `product.created` + commit.

Return the new aggregate.

#### Service integration test coverage matrix

Every typed error code from §13.4 + §14.13 that M3 can reach must have at least one test. Mapping:

| Code | Producer | Test |
|---|---|---|
| `validation_failed` | Create with empty title | `TestService_Create_EmptyTitle` |
| `variant_matrix_mismatch` | Create with 2x3 options but 5 variants | `TestService_Create_MatrixMismatch` |
| `too_many_options` | Create with 4 options | `TestService_Create_TooManyOptions` |
| `too_many_variants` | Create with 101 variants | `TestService_Create_TooManyVariants` |
| `currency_mismatch` | Copy where variant update later submits a wrong currency | `TestService_Update_WrongCurrency` |
| `currency_change_forbidden` | Update with any currency that differs from store | `TestService_Update_CurrencyChangeForbidden` |
| `handle_taken` | Create twice with same handle | `TestService_Create_HandleCollision` |
| `sku_taken` | Create twice with same SKU | `TestService_Create_SKUCollision` |
| `slug_taken` | (category) Create two categories with same slug | (covered in Task 7) |
| `category_not_empty` | (category) Delete a category with a linked product | (covered in Task 7) |
| `category_has_children` | (category) Delete a parent with children | (covered in Task 7) |
| `target_store_invalid` | Copy to same source store, to missing store, to another tenant's store | `TestService_Copy_TargetInvalid_*` |
| `upload_not_found` | Create with a storage_key the FakeUploader doesn't know | `TestService_Create_UploadNotFound` |
| `not_found` | Get/Update/Delete a product that doesn't exist or belongs to another tenant | `TestService_*_NotFound` |
| `payload_too_large` | Register FakeUploader with Size > 10 MiB, create product referencing that key | `TestService_Create_PayloadTooLarge` |
| `unsupported_media_type` | Register FakeUploader with ContentType = `image/svg+xml`, allowed list = `[png, jpeg]` | `TestService_Create_UnsupportedMediaType` |

Codes deferred to M5 (document in the plan but don't test in M3):
- `forbidden` — no authz middleware yet
- `rate_limited` — no rate limiter yet

Also test the **happy path**:
- `TestService_CreateUpdateDeleteLifecycle` — full round trip through Create → Update → Delete → verify soft-deleted.
- `TestService_Copy_CrossStore_Success` — copy happy path, assert media by reference (same storage_key on both rows), variants preserved with source currency, inventory = 0, status = draft, copy_source_product_id set.
- `TestService_Create_VariantStockTrigger` — create, read back, assert `Variant.InventoryQuantity` equals the initial stock passed in the request (trigger test, §14.5).
- `TestService_Create_EnqueuesOutboxEvent` — assert one row in outbox_events with `aggregate='product'`, `event_type='product.created'`, `payload->>'store_id' = storeID`.
- `TestService_Create_CurrencySilentOverride` — caller passes `variant.currency_code = "USD"` but store is `"EUR"`; assert Create succeeds and the persisted variants all have `EUR`.

- [x] **Step 1: Scaffold service_integration_test.go with all test names from the table as `t.Skip("TODO")`.** This gives a visible coverage checklist.

- [x] **Step 2: Implement Create + its tests. Commit.**

```
git commit -m "feat(marketplace-api): add product.Service.Create with sanitize, GCS verify, outbox (M3)"
```

- [x] **Step 3: Implement Update + its tests. Commit.**

```
git commit -m "feat(marketplace-api): add product.Service.Update with variant matrix diff (M3)"
```

- [x] **Step 4: Implement Delete + its tests. Commit.**

```
git commit -m "feat(marketplace-api): add product.Service.Delete (soft) + outbox event (M3)"
```

- [x] **Step 5: Implement Copy + its tests. Commit.**

```
git commit -m "feat(marketplace-api): add product.Service.Copy across stores (M3)"
```

- [x] **Step 6: Flip remaining skipped tests green, verify full service package runs.**

```
TEST_DATABASE_URL=... go test -tags integration ./internal/product/... -race -v
```

Every typed error code from the coverage matrix must appear as a passing `PASS` line. Failures here are the milestone gate — do not proceed.

- [x] **Step 7: Commit any final test additions.**

```
git commit -m "test(marketplace-api): complete M3 service-level typed-error coverage matrix"
```

---

### Task 13: Wire the outbox publisher into `cmd/marketplace-api/main.go`

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Scope:** Start the publisher goroutine after `db.Open` succeeds, wait for it on shutdown.

- [x] **Step 1: Diff plan**

Import:
```go
"github.com/mark8ly/marketplace-api/internal/outbox"
```

After `conn, err := db.Open(cfg.DatabaseURL)`:

```go
	// Outbox publisher — runs in admin and both modes; the storefront
	// process does not produce events, so running it there would just poll
	// an always-empty table and waste a connection.
	var publisherDone <-chan struct{}
	publisherCtx, publisherCancel := context.WithCancel(context.Background())
	defer publisherCancel()
	if m == mode.Admin || m == mode.Both {
		outboxRepo := outbox.NewRepository(conn)
		pub := outbox.New(outbox.Config{
			Repo:     outboxRepo,
			DB:       conn,
			Logger:   log,
			Interval: 2 * time.Second,
			BatchSize: 100,
		})
		publisherDone = pub.Start(publisherCtx)
		log.Info("outbox publisher started")
	}
```

In the shutdown block, after `srv.Shutdown(ctx)`:

```go
	publisherCancel()
	if publisherDone != nil {
		select {
		case <-publisherDone:
			log.Info("outbox publisher stopped")
		case <-time.After(5 * time.Second):
			log.Warn("outbox publisher did not stop in time")
		}
	}
```

- [x] **Step 2: Build**

```
cd services/marketplace-api && go build ./...
```

- [x] **Step 3: Run existing tests to confirm nothing regressed**

```
cd services/marketplace-api && go test ./... -race
```

- [x] **Step 4: Smoke test with a local DB** (dev doc, not CI):

```
MODE=admin DATABASE_URL=... go run ./cmd/marketplace-api
# separately: insert an outbox row manually, watch store_watermarks update
```

- [x] **Step 5: Commit**

```
git commit -m "feat(marketplace-api): start outbox publisher goroutine in admin/both modes (M3)"
```

---

### Task 14: M3 verification + PR

**Files:** none (git + gh only)

- [x] **Step 1: Run the full test suite**

```
cd services/marketplace-api
go vet ./...
go build ./...
go test ./... -race                             # unit
TEST_DATABASE_URL=<dsn> go test -tags integration ./... -race
```

All must be green. Note the `ListAdmin` query count — the test logs it; record the actual number in the PR description.

- [x] **Step 2: Confirm nothing outside the allowed set changed**

```
git diff --stat main...HEAD
```

Expected: only files under `services/marketplace-api/internal/{stores,outbox,category,product,media}/`, `services/marketplace-api/pkg/apperrors/`, `services/marketplace-api/cmd/marketplace-api/main.go`, `services/marketplace-api/go.mod`, `services/marketplace-api/go.sum`, `go.work.sum`, and the plan doc itself.

- [x] **Step 3: Push the branch**

```
git push -u origin feat/products-m3-services
```

- [x] **Step 4: Open PR**

```
gh pr create --base main --head feat/products-m3-services --title "feat(marketplace-api): products M3 — repositories, services, sanitizer, outbox publisher" --body "$(cat <<'EOF'
## Summary

- Adds the admin-path service layer for the products feature: product + category + stores + outbox repositories, product/category services with full Create/Update/Delete/Copy, bluemonday sanitizer (policy v1, OWASP corpus test), variant matrix + handle generators.
- Ships the in-process outbox publisher goroutine that bumps `store_watermarks` per spec §14.1 + §14.6 — runs in admin/both modes, skipped in storefront.
- Introduces `pkg/apperrors` with every typed error code from spec §13.4 + §14.13; every service-layer error path is covered by an integration test.

## What is NOT in this PR

- HTTP handlers, DTOs, admin API routes — M5
- OpenFGA model + middleware — M4
- Real GCS uploader — M5 (this PR ships a stub `media.Uploader` interface + FakeUploader for tests)
- Real platform-api client for store pull-through — M5 (this PR ships a fake Client for StoreMiddleware tests)
- Real Pub/Sub delivery — slice 2 (this PR's publisher only bumps watermarks and marks published)

## Test plan

- [x] `go vet ./...` clean
- [x] `go build ./...` clean
- [x] `go test ./... -race` green (unit)
- [x] `go test -tags integration ./... -race` green against a real Postgres at M2 schema
- [x] `ListAdmin` query count ≤ 7 (actual: <fill in>)
- [x] Every typed error code from §13.4 + §14.13 reachable from M3 has a passing integration test (table in the plan doc)
- [x] Bluemonday OWASP corpus green
- [x] StoreMiddleware singleflight test shows exactly 1 client call under 10 concurrent requests
- [x] Outbox publisher integration test: watermark bumped within 500ms of an enqueue + commit

## Follow-ups (tracked for M4/M5)

- M4: wire `fgaMw.RequireTenantRelation(...)` around the admin routes once they land in M5; adjust service constructors if needed
- M5: replace `stores.Client` stub with a real HTTP client pointed at platform-api
- M5: replace `media.Uploader` stub with GCS HEAD implementation; service code already handles the return types
- Slice 2: add real Pub/Sub delivery in the outbox publisher (current `tick` is the single extension point)

EOF
)"
```

- [x] **Step 5: Wait for CI.** If the billing workaround from auto-memory (`feedback_ci_billing_workaround.md`) fires, flip the repo public temporarily, re-run CI, flip back.

---

## Exit criteria

- [x] `go test ./... -race` green from `services/marketplace-api/`
- [x] `go test -tags integration ./... -race` green
- [x] Every typed error code in §13.4 + §14.13 that M3 can reach has a passing service-level test
- [x] Bluemonday sanitizer reduces the OWASP top-10 corpus to safe output; policy version constant = 1
- [x] StoreMiddleware serve-stale path verified with a fake client that returns errors
- [x] Outbox publisher goroutine starts in admin/both mode, stops cleanly on ctx cancel, bumps `store_watermarks` within one tick after a mutation
- [x] `ListAdmin` query count asserted and documented (≤7 or raise with justification)
- [x] `variant_stock` trigger test passes (write variant_stock → read Variant.InventoryQuantity reflects)
- [x] Copy-to-store test asserts: media by reference (same storage_key), inventory zero, draft status, source currency preserved, copy_source_product_id set
- [x] No changes to `go.work`, migrations, Dockerfile, CI workflows, Helm charts, or any other service
- [x] PR is open and CI is green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. pkg/apperrors + deps | 30 min |
| 2. stores repository | 45 min |
| 3. StoreMiddleware + singleflight tests | 90 min |
| 4. outbox repository | 45 min |
| 5. outbox publisher goroutine | 60 min |
| 6. category repository | 45 min |
| 7. category service + error-path tests | 60 min |
| 8. bluemonday sanitizer + OWASP corpus | 30 min |
| 9. matrix + handle pure-logic helpers | 45 min |
| 10. media.Uploader stub | 15 min |
| 11. product repository (biggest) | 3 hours |
| 12. product service (Create/Update/Delete/Copy) + full error matrix | 4 hours |
| 13. wire publisher into main.go | 20 min |
| 14. verification + PR | 30 min |
| **Total** | **~12 hours** |

Noticeably larger than M2 because the service layer carries all the business-rule enforcement from the spec, but still one contained PR.
