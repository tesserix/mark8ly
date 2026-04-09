# Products Admin UI — Slice 2 Design (M7c, M7d, M7e)

> **Context:** M7a (list page) and M7b (simple product detail page) shipped on 2026-04-09. This design covers the next three milestones that complete the products admin UI: rich variants + media editing (M7c), copy-to-store + bulk actions (M7d), and CSV import/export (M7e).
>
> **Stack:** Next.js 16 (App Router + server components), React 19, Tailwind v4, `@tesserix/web` primitives, `@repo/ui` promoted components, RHF, Paper · Ink · Moss design tokens, Playwright 1.59+. Backend is Go 1.26 / Gin / GORM / Postgres (marketplace-api).
>
> **Authority:** Spec §7.2–7.10 (admin UI), §13.1.1 (permission map), §13.5 (UX corrections), §8 milestone table.

---

## 1. Scope and decomposition

Three plan files land under `docs/superpowers/plans/`:

1. **`2026-04-10-products-m7c-admin-ui-variants-media.md`** — options editor, full variant matrix (per-row price/sku/weight/stock + column bulk actions), rich media editor (GCS upload via M5b, drag-reorder, delete, set primary, alt text, per-variant image assignment, client-side crop/rotate, image replace, bulk upload progress). Extends the existing `ProductForm` from M7b — no new page route.
2. **`2026-04-10-products-m7d-admin-ui-copy-and-bulk.md`** — copy-to-store dialog (wires the stub from M7a) and bulk actions bar (archive, unarchive, publish, unpublish, delete, category assign, copy-to-store) driven by the checkbox UI M7a already renders.
3. **`2026-04-10-products-m7e-admin-ui-csv-import-export.md`** — streaming CSV export and async CSV import with a resumable in-process Go worker. Spans both marketplace-api (Go) and apps/admin (Next.js) unlike M7c/M7d which are frontend-only.

**Execution order:** M7c → M7d → M7e. M7d can run in parallel with M7e if two tracks are desired. M7c must land first because it extends `ProductForm` (M7d and M7e read that contract).

**Out of scope for slice 2:** inventory multi-location UI, cost_price gross-margin analytics, product tags/metafields editor UI, digital product / download UI, localized content editing, customer-facing UI of any kind.

---

## 2. M7c — variants + media

### 2.1 Where it lives

Extends `apps/admin/components/products/form/ProductForm.tsx` (from M7b) with three new tabbed sections: **Media**, **Options**, **Variants**. No new route. Reuses `ProductForm`'s existing RHF context and submit pipeline.

### 2.2 Component tree (new files under `apps/admin/components/products/`)

```
form/
  MediaTab.tsx              container
  OptionsTab.tsx
  VariantsTab.tsx
media/
  MediaGrid.tsx             drag-reorder grid (@dnd-kit/sortable)
  MediaCard.tsx             thumbnail + overflow menu
  MediaUploader.tsx         drag-drop + progress
  MediaCropDialog.tsx       react-easy-crop
  MediaAltTextInput.tsx
options/
  OptionsEditor.tsx         list of options
  OptionRow.tsx             name + values chips
variants/
  VariantMatrixTable.tsx    generated grid
  VariantRow.tsx
  VariantBulkBar.tsx        column bulk actions ("set all prices to X")
  VariantImagePicker.tsx    link media → variant
```

Pure helper (unit-testable, no React):

```
lib/products/generateVariants.ts
```

### 2.3 State model

`ProductForm` (M7b) owns the full draft as RHF state. Options, variants, and media are RHF field arrays. The variant matrix is **derived** from options on every options change via `generateVariants(options, existingVariants)` — a pure function that:

- builds a stable **variant key** by stringifying sorted option-value pairs: `"Color=Red|Size=M"`
- **preserves** existing variants whose keys match (keeps price/sku/stock/weight/variant_id)
- **adds** new variants with defaults from the base product (base price, empty sku, 0 stock)
- **drops orphans** into a `removed` bucket on the form state; they are deleted on save, not silently forgotten

Dirty tracking via RHF `isDirty` across all tabs.

### 2.4 Data flow

```
Options edit  → generateVariants() → merge into form.variants
Media upload  → POST signed-url → PUT blob to GCS → POST finalize → media row appended
Save          → single PATCH /products/:id with full aggregate
                (options + variants + media + categories + removed bucket)
```

### 2.5 Variant generation rules

- Key: sorted `option_name=value_name` pairs joined by `|`
- Matching: key equality preserves existing variant row (including server `id`)
- Cap: if option values produce >500 combinations, block with inline error ("Too many variants — reduce options or values")
- Min options: 0 (simple product, no variants)
- Removing an option value moves its variants to the `removed` bucket
- Renaming an option value is a rename in-place (keys update, rows preserve data)

### 2.6 Media upload flow

Signed URL pattern (reuses M5b backend):

```
POST /api/v1/admin/stores/:storeId/products/:id/media/signed-url
→ { upload_url, gcs_path, expires_at }

Client: PUT blob directly to upload_url

POST /api/v1/admin/stores/:storeId/products/:id/media
body: { gcs_path, alt, position, variant_id? }
→ { id, url, ... }
```

**Client-side crop** via `react-easy-crop` → canvas → blob → PUT. Crop is **editable on existing media** (not just upload) — reopening the crop dialog re-downloads the original blob (stored server-side under `gcs_path_original`) and re-uploads the cropped result as a new revision.

**Bulk upload progress:** `MediaUploader` tracks an array of `{ file, progress, status, error }` and renders a progress strip while uploads run. Uploads fan out with a concurrency cap of 3.

### 2.7 Backend verification gate

Before M7c task 2 begins, **Task 1 verifies** that the existing M5a `PATCH /api/v1/admin/stores/:storeId/products/:id` already accepts the full aggregate (options, variants, media, per-variant image refs). If gaps are found, backend fix tasks are inserted into M7c before frontend work proceeds. Likely gaps to check:

- Does `PATCH` handle adding/removing options and their values?
- Does `PATCH` handle the `removed` variant bucket (soft-delete vs hard-delete)?
- Is there a dedicated media endpoint (`POST /products/:id/media` + `DELETE /products/:id/media/:mediaId`) or does media live only on the aggregate?
- Is `variant_id` on a media row supported?
- Is `gcs_path_original` stored (needed for re-cropping)?

### 2.8 Libraries (new npm deps in `apps/admin`)

- `react-easy-crop` — crop/rotate dialog
- `@dnd-kit/core` + `@dnd-kit/sortable` — drag-reorder (already used elsewhere in marketplace-admin; reuse version)

---

## 3. M7d — copy-to-store + bulk actions

### 3.1 Copy-to-store dialog

**Component:** `apps/admin/components/products/CopyToStoreDialog.tsx` using `@tesserix/web` Dialog primitive.

**Trigger:** list overflow menu "Copy to store…" (the stubbed item from M7a) and detail page action menu (new).

**Flow:**
1. Read `serverSession.stores` — filter to stores where user role is `admin` or `owner`, exclude current store
2. Radio list of target stores
3. Toggle: "Also copy media" (default on)
4. Toggle: "Publish as draft in target" (default on, read-only)
5. Submit → server action → toast "Copied to {store}" with link

**Backend:** needs verification. Expected endpoint:

```
POST /api/v1/admin/stores/:sourceStoreId/products/:id/copy
body: { target_store_id, copy_media: bool }
→ { new_product_id, new_store_id }
```

If absent, M7d Task 1 adds a backend sub-task.

### 3.2 Bulk actions bar

**Component:** `apps/admin/components/products/BulkActionsBar.tsx` — sticky bottom bar, Paper surface, hairline top rule, slides up when ≥1 row selected on list page.

**Actions (role-gated per §13.1.1):**

| Action | Staff | Admin | Owner |
|---|---|---|---|
| Archive | — | ✓ | ✓ |
| Unarchive | — | ✓ | ✓ |
| Publish | — | ✓ | ✓ |
| Unpublish | — | ✓ | ✓ |
| Assign category | — | ✓ | ✓ |
| Copy to store | — | ✓ | ✓ |
| Delete | — | — | ✓ |

**Delete** opens the hard-delete confirm dialog (per §13.5 — the only dialog allowed outside hard destructive ops). All other actions are inline with optimistic updates and toast feedback.

**Selection state:** URL-backed via `?selected=id1,id2,…` so selection survives pagination. Capped at **200 ids** (soft limit; bulk endpoint enforces same). Lives in `useProductSelection()` hook.

**Backend:** needs verification. Expected endpoint:

```
POST /api/v1/admin/stores/:storeId/products/bulk
body: { action: "archive"|"unarchive"|..., product_ids: [...], params?: {...} }
→ { results: [{ id, status: "ok"|"error", error? }] }
```

Atomic per product row (not global transaction); returns per-id success/failure; FGA enforced per product id. If absent, M7d Task 1 adds a backend sub-task.

---

## 4. M7e — CSV import/export

### 4.1 Export (synchronous streaming)

**Endpoint:** `GET /api/v1/admin/stores/:storeId/products/export.csv?ids=…&filters=…`

Streams CSV response, no job, no progress. Content-Type `text/csv`, Content-Disposition attachment with timestamped filename.

**Columns:** `id, handle, title, status, vendor, category, base_price, compare_at_price, sku, stock_total, variant_count, created_at`

**Frontend trigger:** "Export selected" button in `BulkActionsBar`; "Export all" in list page header (respects current filters).

### 4.2 Import (async, resumable)

#### Backend additions (Go)

```
internal/csvjob/
  models.go         CsvImportJob entity
  repository.go     CRUD + find-orphaned-jobs
  service.go        Submit, GetStatus, Cancel, Resume
  worker.go         goroutine — parses GCS CSV row by row
  parser.go         CSV → ProductDraft + row-level validation
migrations/000004_csv_import_jobs.up.sql / .down.sql
```

#### `csv_import_jobs` schema

| Column | Type | Notes |
|---|---|---|
| id | uuid pk | |
| store_id | uuid not null | FK + FGA scope |
| user_id | text not null | from auth context |
| gcs_path | text not null | uploaded source CSV |
| error_csv_gcs_path | text null | written as errors accumulate |
| status | text not null | `queued`, `running`, `paused`, `completed`, `failed`, `cancelled` |
| total_rows | int | set after first full scan |
| last_processed_row | int not null default 0 | resume checkpoint |
| success_count | int not null default 0 | |
| error_count | int not null default 0 | |
| heartbeat_at | timestamptz null | worker liveness |
| created_at | timestamptz default now() | |
| updated_at | timestamptz default now() | |
| CHECK | status in allowed set | |
| CHECK | last_processed_row ≤ total_rows | |

Index: `(store_id, status, created_at desc)` for the history list.

#### Worker pattern

- **Single goroutine per job**, not per pod — `pg_try_advisory_lock(hash(job_id))` ensures only one pod processes a given job even across Knative replicas
- **Heartbeat** every 5s (`UPDATE csv_import_jobs SET heartbeat_at = now() WHERE id = $1`)
- **Checkpoint** `last_processed_row` every 10 rows (cheap; matches expected throughput)
- **Crash recovery:** on startup, `main.go` scans for `status='running' AND heartbeat_at < now() - 60s` → marks them `paused` and re-enqueues. Worker resumes from `last_processed_row`.
- **Cancellation:** service sets `status='cancelled'`, worker checks status every batch (10 rows) and aborts cleanly
- **Error rows:** written to a per-job error CSV in GCS with columns `row_number, raw_line, error_message` — uploaded incrementally via GCS resumable upload or buffered-then-flushed on completion (decision: buffered in memory up to 10k rows, then flushed; cap import at 50k rows)

#### HTTP endpoints (new, FGA-gated)

```
POST   /api/v1/admin/stores/:storeId/products/csv-imports          submit (multipart upload → GCS → queue)
GET    /api/v1/admin/stores/:storeId/products/csv-imports           list history (paginated)
GET    /api/v1/admin/stores/:storeId/products/csv-imports/:jobId    status
POST   /api/v1/admin/stores/:storeId/products/csv-imports/:jobId/cancel
GET    /api/v1/admin/stores/:storeId/products/csv-imports/:jobId/errors.csv
```

#### Frontend (Next.js)

```
apps/admin/app/products/import/
  page.tsx                   upload + preview + column mapping (client component)
  [jobId]/
    page.tsx                 live status (server action polling every 2s)
components/products/csv/
  CsvUploadDropzone.tsx
  CsvPreviewTable.tsx        first 5 rows via papaparse (client-side)
  CsvColumnMapping.tsx       map CSV columns → product fields
  CsvImportHistory.tsx       table of past jobs
  CsvJobProgress.tsx         progress bar + row counters
  CsvErrorSummary.tsx        link to download error CSV
lib/api/csvImports.ts        typed client for all endpoints
```

**Polling:** 2s interval via server action + `router.refresh()` (Next 16 pattern). Stops when `status ∈ {completed, failed, cancelled}`.

#### Libraries

- **Backend:** `encoding/csv` (stdlib). No new deps.
- **Frontend:** `papaparse` (client-side preview parsing only).

---

## 5. Testing strategy

All three plans follow the global TDD rule — tests written first, implementation second, verification last. 80% coverage on all new files.

### 5.1 M7c

- **Unit (Vitest in apps/admin):** `generateVariants()` — empty options, add option, remove option, rename value, swap order, preserve stock on matching keys, orphan handling, 500-combination cap (8+ cases)
- **Component (Vitest + RTL):** `OptionsEditor` add/remove/rename flows; `VariantMatrixTable` inline edit + column bulk apply; `MediaCard` overflow actions; `MediaCropDialog` open → crop → apply round-trip
- **Playwright E2E (1 test):** create product → add 2 options (Color, Size) with 2 values each → confirm 4 variants auto-generated → set all prices via column bulk → upload 2 images → crop one → assign one image to Red variant → save → reload page → assert full persistence

### 5.2 M7d

- **Unit:** `useProductSelection()` URL sync, 200-cap, cross-page persistence reducer
- **Component:** `BulkActionsBar` visibility by selection count; role-gated action rendering for staff/admin/owner; `CopyToStoreDialog` filters target stores by permission and excludes current store
- **Playwright E2E (1 test):** select 3 products on list page → click bulk archive → assert toast + row status updates → select 1 product → copy to another store → navigate to target store's list → assert product exists there
- **Backend (Go integration, if new bulk endpoint):** bulk archive atomicity, per-id partial-failure response shape, FGA denial per product id

### 5.3 M7e

- **Unit (Go):** `parser.go` — valid row, missing required column, type coercion, malformed row, duplicate handle (table-driven); `worker.go` resume logic with mocked repository; advisory-lock contention test
- **Integration (Go, real Postgres + GCS emulator):** submit 100-row job → worker processes → all rows landed → `last_processed_row` advanced → error CSV written for bad rows; crash-recovery test — kill worker mid-run, restart, assert resume from checkpoint with no duplicate rows created
- **Unit (frontend):** column mapping reducer; preview parser row count
- **Playwright E2E (1 test):** upload 10-row CSV with 2 intentionally bad rows → assert job page polls and shows progress → poll completes → assert 8 products created in list page → download error CSV → assert 2 error rows with row numbers
- **Export E2E:** trigger export of 5 selected products → assert downloaded CSV content matches expected columns and row count

---

## 6. Task sizing

### M7c (~14 tasks)

1. Backend verification — test current `PATCH /products/:id` aggregate handling; insert backend fix sub-tasks if gaps
2. `generateVariants()` pure function + 8 unit tests (TDD first)
3. `MediaUploader` + signed URL client
4. `MediaCropDialog` with `react-easy-crop`
5. `MediaCard` + `MediaGrid` with drag-reorder
6. `MediaTab` composition + alt text + per-variant image picker
7. `OptionsEditor` + `OptionRow`
8. `VariantMatrixTable` + `VariantRow` inline edit
9. `VariantBulkBar` column actions
10. `ProductForm` tab integration + dirty tracking merge
11. Server action extension (PATCH full aggregate)
12. Component tests (Vitest + RTL)
13. Playwright E2E
14. Verification + PR

**Exit:** merchant creates product with 2 options / 4 variants / 3 cropped images / 1 per-variant image, saves, reloads; everything persists; Playwright green; 80% coverage on new files.

### M7d (~10 tasks)

1. Backend verification — check for copy endpoint + bulk endpoint; insert backend sub-tasks if missing
2. `CopyToStoreDialog` + server action
3. `useProductSelection()` URL-backed hook with 200-cap
4. `BulkActionsBar` component + role-gated actions
5. Bulk archive/unarchive/publish/unpublish server actions
6. Bulk delete (owner only) + hard-delete confirm
7. Bulk category assign (reuse M7b picker)
8. Bulk copy-to-store
9. Component + E2E tests
10. Verification + PR

**Exit:** 3-row bulk archive works; copy-to-store lands in target store; selection survives pagination; FGA blocks unauthorized bulk ops.

### M7e (~16 tasks, mixed Go + Next.js)

1. Migration `000004_csv_import_jobs`
2. GORM model + repository + tests
3. `csvjob.Service` (Submit, GetStatus, Cancel, Resume) + tests
4. `parser.go` with validation + table-driven tests
5. `worker.go` goroutine + heartbeat + advisory lock
6. Crash-recovery integration test
7. Wire worker into `main.go` (startup scan + shutdown drain)
8. HTTP handlers: submit, list, status, cancel, download error CSV
9. Export handler (streaming CSV)
10. FGA gates on all new endpoints
11. Frontend: `lib/api/csvImports.ts` typed client
12. `app/products/import/page.tsx` upload + column mapping
13. `app/products/import/[jobId]/page.tsx` status polling
14. `CsvImportHistory` table
15. Playwright E2E (import + export)
16. Verification + PR

**Exit:** 100-row CSV imports with 2+ partial failures; error CSV downloadable with row numbers; export matches current filter; crash mid-import recovers cleanly with no duplicate rows.

---

## 7. Risks and landmines

1. **Backend PATCH aggregate gaps (M7c)** — highest risk. M7c Task 1 must verify; if full aggregate isn't supported, backend work is inserted into M7c before any frontend work. Do not assume.
2. **Variant combination explosion** — capped at 500 client-side; backend should enforce the same cap independently.
3. **GCS signed URL expiry** — set to 15 minutes; client retries on 403 by fetching a new signed URL.
4. **Crop re-editing requires storing originals** — `gcs_path_original` must be a separate object from the displayed variant. Verify backend stores both.
5. **Knative scale-to-zero + CSV worker** — if no in-flight HTTP requests and no pinned concurrency, the pod may scale to zero mid-job. Mitigation: the worker pins a local concurrency counter on `marketplace-api` (non-zero while a job runs), which keeps the Knative pod alive. Needs explicit code in `main.go` via a `ConcurrencyReporter` or similar.
6. **Advisory lock hash collisions** — `hash(uuid)` to int64 has a tiny collision risk. Acceptable for this volume; document it.
7. **CSV row encoding** — enforce UTF-8, reject BOM-less Windows-1252 with a clear error, don't silently mangle.
8. **Bulk delete FGA** — owner-only gate on frontend is cosmetic; backend MUST enforce per-product FGA check on each id and return partial failures for unauthorized ids, never a global 403.
9. **papaparse bundle size** — only import on the `/products/import` route to keep the main list page lean.
10. **50k-row import cap** — hard limit on the backend to prevent runaway memory. UI must show this limit in the upload dialog.

---

## 8. Verification checklist (all three milestones)

- [ ] All new Go files 80%+ coverage (`go test -cover`)
- [ ] All new TS files 80%+ coverage (Vitest)
- [ ] Playwright E2E green for each milestone
- [ ] No new `logrus` / `slog` warnings
- [ ] No new `go vet` warnings
- [ ] No new ESLint errors
- [ ] Paper · Ink · Moss tokens only — no new hex values
- [ ] WCAG 2.1 AA on all new surfaces (keyboard nav, focus ring, screen reader labels)
- [ ] `prefers-reduced-motion` honored on all animations
- [ ] Skip link still first focusable on products list page
- [ ] No secrets committed; `.env.example` updated if any new config

---

## 9. Execution order

**Recommended:** M7c → M7d → M7e, serial.

**Alternative:** M7c → (M7d ∥ M7e), last two in parallel on separate branches if two executors are available. M7d and M7e have no shared files and no conflicting backend changes.

M7c must land first because it extends `ProductForm` — M7d's bulk "Assign category" and M7e's CSV column mapping both depend on the final shape of the product aggregate after M7c.
