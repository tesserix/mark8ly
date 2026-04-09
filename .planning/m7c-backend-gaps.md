# M7c Backend Gaps Catalog

Generated: 2026-04-10 (Task 1 Step 1)
Scope: services/marketplace-api aggregate PATCH readiness for M7c variants + rich media admin UI

> **All 7 gaps closed. Backend ready for M7c frontend work.**

## Summary

| # | Verification item | Status | Blocking |
|---|---|---|---|
| 1 | Aggregate PATCH round-trip | DONE | yes |
| 2 | Removed variants handling | DONE | yes |
| 3 | Dedicated media endpoints (create/delete/recrop) | DONE | yes |
| 4 | Signed URL endpoint | PARTIAL | no |
| 5 | `variant_id` on `product_media` | DONE (27b9e9d) | yes |
| 6 | `gcs_path_original` column | DONE (6afa464, 8cc3f3f) | yes |
| 7 | Backend 500-variant cap | DONE (2f26f7e) | yes |
| 8 | Recrop support | DONE | yes |

## Detail

### 1. Aggregate PATCH round-trip

**Status:** PARTIAL

**Evidence:**
- `services/marketplace-api/internal/handlers/admin/products.go:135` — `ProductHandler.Patch` is the PATCH `/admin/stores/:storeId/products/:id` entry point. It fans the wire DTO out to three sub-methods: `UpdateBasics`, `ReplaceMedia`, `UpdateCategoryLinks`.
- `services/marketplace-api/internal/handlers/admin/products.go:190-193` — explicit TODO comment: "Variants/Options — deferred to M5b... the handler silently accepts and ignores these fields."
- `services/marketplace-api/internal/handlers/admin/validation.go:226-236` — `UpdateProductRequest` wire DTO DOES declare `Options *[]CreateProductOptionInput` and `Variants *[]CreateProductVariantInput`, but they are bound from JSON and then ignored in the handler.
- `services/marketplace-api/internal/product/service.go:142-166` — `UpdateBasicsRequest` is scalar-only; a distinct `UpdateVariantsRequest` struct exists (lines 160-166) but **no `Service.UpdateVariants` method is implemented** anywhere in `service.go`, `service_copy.go`, `service_variant_patch.go`, or `service_single_media.go`.
- `services/marketplace-api/internal/product/service_variant_patch.go:42` — the only variant mutation endpoint is `UpdateVariantBasics`, which patches ONE variant row (narrow PATCH used by the separate `PATCH /products/:id/variants/:variantId` route at `routes.go:57`).

**Gap:** The aggregate PATCH silently drops `options` and `variants` arrays. There is no service method that accepts a full desired variant matrix, diffs against existing variants, and dispatches Add/Update/Remove. `UpdateVariantsRequest` exists as a type but has no implementation.

### 2. Removed variants handling

**Status:** MISSING

**Evidence:**
- Grep for `removed_variant_ids`, `RemovedVariantIDs`, `removedVariantIds` across the entire `services/marketplace-api` tree returns zero matches.
- `services/marketplace-api/internal/handlers/admin/validation.go:226-236` — `UpdateProductRequest` has no `RemovedVariantIDs` field.
- `services/marketplace-api/internal/product/service.go:160-166` — `UpdateVariantsRequest` type has `Options` and `Variants` but no removed-ids field.
- Migration `000001_products_initial.up.sql` declares `product_variants.deleted_at` (soft-delete column exists at the DB layer), but no service code paths write to it for individual variants (`Delete` at `service.go:421` soft-deletes the whole product).

**Gap (closed):** `UpdateAggregateRequest` now accepts `RemovedVariantIDs` and soft-deletes via the existing `ApplyVariantDiffInTx`, preserving order-history FKs. The handler wires `req.RemovedVariantIDs` through the wire DTO (`UpdateProductRequest.RemovedVariantIDs`) into the aggregate path. No hard-delete code path was added; orphaned IDs route through the same soft-delete as diffed-out variants. Covered by `TestIntegration_ProductService_UpdateAggregate_RemovedVariantIDsSoftDelete`.

### 3. Dedicated media endpoints

**Status:** PARTIAL

**Evidence:**
- `services/marketplace-api/internal/handlers/admin/routes.go:61-75` — media subgroup registered:
  - `POST /products/:id/media/upload-url` → `MediaHandler.UploadURL` (line 63-65)
  - `POST /products/:id/media` → `MediaHandler.Create` (line 66-68) ✅
  - `PATCH /products/:id/media/:mediaId` → `MediaHandler.Patch` (line 69-71)
  - `DELETE /products/:id/media/:mediaId` → `MediaHandler.Delete` (line 72-74) ✅
- `POST /products/:id/media/:mediaId/recrop` — **NOT registered anywhere.** Grep for `recrop` returns zero matches in routes, handlers, services, models, or migrations.

**Gap:** Create (POST) and Delete routes exist. Recrop route is entirely absent.

### 4. Signed URL endpoint

**Status:** PARTIAL

**Evidence:**
- `services/marketplace-api/internal/handlers/admin/routes.go:63-65` — route is `POST /products/:id/media/upload-url`, NOT `/media/signed-url` as the spec names it.
- `services/marketplace-api/internal/handlers/admin/media.go:41-70` — `UploadURL` handler calls `media.SignedURLGenerator.SignedUploadURL` and returns `{url, storage_key, expires_at}`. Returns 501 if uploader doesn't implement the signer interface (dev FakeUploader).
- `services/marketplace-api/internal/media/gcs.go` exists; `services/marketplace-api/internal/media/uploader.go` declares the `SignedURLGenerator` interface.

**Gap:** Functionally equivalent endpoint exists under a different path (`/upload-url` vs `/signed-url`). If the frontend spec requires the exact `/media/signed-url` path, add an alias; otherwise reuse existing.

### 5. `variant_id` on `product_media`

**Status:** PARTIAL

**Evidence:**
- `services/marketplace-api/migrations/000001_products_initial.up.sql:216` — column exists: `variant_id uuid REFERENCES product_variants(id) ON DELETE SET NULL`. Partial index at line 230: `product_media_variant_idx ON product_media (variant_id) WHERE variant_id IS NOT NULL`.
- `services/marketplace-api/internal/product/models.go:139` — GORM field `VariantID *string` present.
- `services/marketplace-api/internal/product/service_single_media.go:29, 63` — `AddMediaRequest` has `VariantID *string`; `AddMedia` writes it to the row.
- `services/marketplace-api/internal/handlers/admin/validation.go:64-70` — **wire-level `CreateMediaRequest` has NO `variant_id` field**, so the HTTP surface drops it before it reaches the service.
- `services/marketplace-api/internal/handlers/admin/validation.go:73-77` — `UpdateMediaWireRequest` (PATCH media) has only `Alt`, `Position`, `URL`. No variant reassignment path.
- `services/marketplace-api/internal/product/service.go:229-241` and `394-408` — bulk Create/ReplaceMedia paths DO plumb `VariantID` through, but only if callers supply it (no external caller does).

**Gap:** Column and model field exist and are plumbed through the service layer, but every wire DTO that writes media omits `variant_id`. No API path lets the frontend set or change a media row's variant association.

### 6. `gcs_path_original` column

**Status:** MISSING

**Evidence:**
- `services/marketplace-api/migrations/000001_products_initial.up.sql:213-228` — `product_media` table has only `url, storage_key, alt, position, media_type, width, height, bytes`. No `gcs_path_original`, `original_path`, or `source_gcs_path` column.
- `services/marketplace-api/internal/product/models.go:136-149` — GORM `Media` struct has no corresponding field.
- Grep for `gcs_path_original` / `original_path` / `source_gcs_path` in the whole `services/marketplace-api` tree returns zero matches.

**Gap:** Column does not exist; needs a new migration and GORM field, plus a backfill strategy for existing rows (use `storage_key` as the initial value).

### 7. Backend 500-variant cap

**Status:** MISSING

**Evidence:**
- Grep for `MaxVariants`, `max_variants`, `variant_cap`, `len(variants) >`, and the literal `500` in Go sources under `services/marketplace-api/internal/product/` returns zero matches related to a variant cap. The two `500` hits in `repository.go:7` and `repository_variants.go:4` are comments about file length.
- `services/marketplace-api/internal/product/matrix.go` (referenced from `service.go:185` as `ValidateMatrix`) — verified no cap via grep above.
- `services/marketplace-api/internal/product/service.go:184-187` — only validation is `ValidateMatrix(options, specs)`; no length guard.

**Gap:** No hardcoded or configurable cap on variant count anywhere in Create or Update paths.

### 8. Recrop support

**Status:** MISSING

**Evidence:**
- Grep for `recrop`, `Recrop`, `re-crop`, case-insensitive, across the entire `services/marketplace-api` tree returns zero matches.
- No handler, no service method, no repository method, no route, no migration column.

**Gap:** Complete absence. Needs a new handler + service method + (depending on design) either a new `gcs_path_original` column so re-cropping can reference the untouched source, or a GCS-side preserved object.

## Recommended Task 1 sub-tasks

Ordered by dependency so the frontend can unblock as early as possible.

1. **Add `gcs_path_original` column + backfill migration** (S)
   - Files: new `services/marketplace-api/migrations/000005_product_media_gcs_path_original.up.sql` (+ `.down.sql`); update `internal/product/models.go` `Media` struct.
   - Backfill: `UPDATE product_media SET gcs_path_original = storage_key WHERE gcs_path_original IS NULL`.
   - Test: `TestMedia_GcsPathOriginalBackfill` (migration-level + repo round-trip).

2. **Wire `variant_id` through media wire DTOs** (S)
   - Files: `internal/handlers/admin/validation.go` (add `VariantID *string` to `CreateMediaRequest` and `UpdateMediaWireRequest`; plumb through `toServiceAddMedia` and a new `toServiceUpdateMedia` field); `internal/product/service_single_media.go` (`UpdateMediaRequest.VariantID`, handle in `UpdateMedia`); repo `UpdateMediaInTx` accept `variant_id` in fields map.
   - Test: `TestMedia_VariantAssignment` — create media with `variant_id`, patch to reassign, patch to clear.

3. **Backend variant cap constant + enforcement** (S)
   - Files: `internal/product/matrix.go` (add `MaxVariants = 500`, enforce in `ValidateMatrix`); error through `apperrors`.
   - Test: `TestAggregatePatch_VariantCapExceeded`.

4. **`Service.UpdateAggregate` + `removed_variant_ids` handling** (L) — the core blocker
   - Files: new `internal/product/service_aggregate_update.go` implementing full-diff matrix update (adds, updates, removes-by-id using existing `deleted_at` soft-delete column); extend `internal/product/repository_variants.go` with `SoftDeleteVariantsInTx`; wire into `internal/handlers/admin/products.go` `Patch` (replace the TODO at lines 190-193); add `RemovedVariantIDs *[]string` to `validation.go` `UpdateProductRequest`.
   - Must reuse existing `buildOptions` / `buildVariants` helpers and preserve currency guards.
   - Tests: `TestAggregatePatch_FullAggregateRoundTrip`, `TestAggregatePatch_RemovedVariants`.

5. **Recrop endpoint + service method** (M)
   - Files: new handler method `MediaHandler.Recrop` in `internal/handlers/admin/media.go`; new `Service.RecropMedia` in a new `internal/product/service_media_recrop.go`; register `POST /products/:id/media/:mediaId/recrop` in `routes.go`; extend `media.Uploader` interface (or add a new `Recropper` interface) to perform the GCS-side crop against `gcs_path_original`, writing to a new `storage_key`.
   - Depends on sub-task 1 (`gcs_path_original` must exist first).
   - Test: `TestMedia_RecropRoundTrip`.

6. **(Optional) Signed-URL alias** (S)
   - If the frontend spec requires `/media/signed-url` as the literal path, register an alias route pointing to the existing `MediaHandler.UploadURL` at `routes.go:63`. Otherwise document the existing `/media/upload-url` and move on.

7. **Media lifecycle integration test sweep** (S) — captures task 2's wire changes plus the already-working create/delete path.
   - Test: `TestMediaEndpoints_Lifecycle`.

## Exit matrix (to be filled as gaps close)

| # | Spec §2.7 verification item | Test name | Status | Commit |
|---|---|---|---|---|
| 1 | PATCH aggregate round-trip | TestIntegration_ProductService_UpdateAggregate_FullRoundTrip | ✅ | (pending) |
| 2 | Removed variants handling | TestIntegration_ProductService_UpdateAggregate_RemovedVariantIDsSoftDelete | ✅ | (pending) |
| 3 | Media endpoints lifecycle (incl. recrop route) | TestAPI_AdminMedia_Recrop_ReturnsSignedUrlsPreservingOriginal | ✅ | (this commit) |
| 4 | variant_id on product_media | TestAPI_AdminMedia_Create_WithVariantID_Persists / TestAPI_AdminMedia_Patch_UpdatesVariantID | ✅ | 27b9e9d |
| 5 | gcs_path_original column + backfill | TestIntegration_Service_AddMedia_HappyPath | ✅ | 6afa464, 8cc3f3f |
| 6 | 500-variant backend cap | TestValidateMatrix_501Variants_Rejected / TestValidateMatrix_500Variants_Accepted | ✅ | 2f26f7e |
| 7 | Recrop round-trip | TestAPI_AdminMedia_Recrop_AfterCommit_KeepsOriginalPinned | ✅ | (this commit) |
