# Products Admin UI — Slice 2 Design (M7c, M7d, M7e)

> **Context:** M7a (list page) and M7b (simple product detail page) shipped on 2026-04-09. This design covers the next three milestones that complete the products admin UI: rich variants + media editing (M7c), copy-to-store + bulk actions (M7d), and CSV import/export (M7e).
>
> **Stack:** Next.js 16 (App Router + server components), React 19, Tailwind v4, `@tesserix/web` primitives, `@repo/ui` promoted components, RHF, Paper · Ink · Moss design tokens, Playwright 1.59+. Backend is Go 1.26 / Gin / GORM / Postgres (marketplace-api).
>
> **Authority:** Spec §7.2–7.10 (admin UI), §13.1.1 (permission map), §13.5 (UX corrections), §8 milestone table.
>
> **Design process for all UI work in this slice:** every new screen and every new composition MUST flow through the impeccable / frontend-design skill chain before merge. Use `frontend-design` to generate initial UI, `critique` for a UX scoring pass, `polish` + `arrange` + `typeset` for a pre-merge quality gate, `audit` for a11y/perf/token compliance, and `adapt` for responsive verification. `teach-impeccable` must have been run once in this repo to pin the Paper · Ink · Moss design context; **verify `mark8ly/.impeccable.md` exists as M7c Task 0** before any UI code is written. No merchant-facing component in M7c/M7d/M7e ships without a `critique` score ≥ 7.5/10 and a `polish` pass on the final PR.

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
- Cap: if option values produce >500 combinations, block with inline error ("Too many variants — reduce options or values"). **Backend must mirror this cap independently** — explicit M7c Task 1 deliverable, not a "verify if it's there" check.
- **Option reorder stability:** sorting happens inside the key builder, so `"Color=Red|Size=M"` and `"Size=M|Color=Red"` collapse to the same key. Explicit unit test case covers reordering options in the UI without orphaning any variants.
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

**Signed URL expiry** is 60 minutes to comfortably cover concurrent multi-file bulk uploads. Client still handles 403 on PUT by fetching a fresh URL and retrying once — this retry path has an explicit test case.

**Client-side crop** via `react-easy-crop` → canvas → blob → PUT. Crop is **editable on existing media** — reopening the crop dialog re-downloads the *original* blob (stored server-side under a dedicated `gcs_path_original` column on `product_media`) and re-uploads the cropped result as a new revision. **Storing the original is a backend feature**, not an assumption: M7c Task 1 treats `gcs_path_original` as a required deliverable. If the existing media table lacks it, a migration + backfill + API contract update is added to M7c before any crop work begins.

**Bulk upload progress:** `MediaUploader` tracks an array of `{ file, progress, status, error }` and renders a progress strip while uploads run. Uploads fan out with a concurrency cap of 3.

### 2.7 Backend verification gate (M7c Task 1)

Before any M7c frontend work, Task 1 verifies and closes gaps on the marketplace-api contract. This is a **required gate**: if anything below is missing, the fix is scoped into M7c as a backend sub-task that blocks frontend work.

1. `PATCH /api/v1/admin/stores/:storeId/products/:id` accepts the full aggregate (add/remove options, add/remove option values, variant matrix with `id` or `null`, media rows with optional `variant_id`, and a `removed_variant_ids` array).
2. Soft-delete vs hard-delete behavior for removed variants is documented and testable.
3. Dedicated media endpoints exist: `POST /products/:id/media`, `DELETE /products/:id/media/:mediaId`, `POST /products/:id/media/:mediaId/recrop`.
4. `variant_id` column on `product_media` exists and is writable through the API.
5. `gcs_path_original` column on `product_media` exists and is populated on initial upload. Backfill for existing rows: set equal to `gcs_path` so legacy images are "already cropped to themselves."
6. Backend enforces the 500-variant cap independently (HTTP 422 with a typed error).
7. `recrop` endpoint accepts `{ crop_box: { x, y, w, h }, rotation }`, re-uploads the cropped blob, and returns the updated media row pointing at a new `gcs_path` while preserving `gcs_path_original`.

### 2.8 Libraries (new npm deps in `apps/admin`)

- `react-easy-crop` — crop/rotate dialog
- `@dnd-kit/core` + `@dnd-kit/sortable` — drag-reorder. **Verification item:** check if marketplace-admin already pins these. If yes, reuse the pinned version. If no, add at latest stable — no other app in the monorepo currently uses it, so there's no alignment pressure. Do not assume it's already there.

---

## 3. M7d — copy-to-store + bulk actions

### 3.1 Copy-to-store dialog

**Component:** `apps/admin/components/products/CopyToStoreDialog.tsx` using `@tesserix/web` Dialog primitive.

**Trigger:** list overflow menu "Copy to store…" (the stubbed item from M7a) and detail page action menu (new).

**Flow:**
1. Read `serverSession.stores` — filter to stores where user role is `admin` or `owner`, exclude current store
2. Radio list of target stores
3. Toggle: "Also copy media" (default on)
4. Static info row: "Copied products are published as drafts in the target store." (not a toggle — fixed behavior, shown as editorial copy so users understand the outcome)
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

**Selection state:** `useProductSelection()` hook. Canonical store is `sessionStorage` keyed by `store_id` (survives reloads, cross-tab scoped). A short URL hash `#sel={count}` mirrors selection *count* only, not ids — this keeps URLs short, avoids Istio header length limits, and still makes "you have N selected" bookmarkable. Hard cap **100 ids** (reduced from 200 after the URL-length review flagged proxy limits); bulk endpoint enforces the same cap.

**Backend:** needs verification. Expected endpoint:

```
POST /api/v1/admin/stores/:storeId/products/bulk
body: { action: "archive"|"unarchive"|..., product_ids: [...], params?: {...} }
→ { results: [{ id, status: "ok"|"error", error? }] }
```

Atomic per product row (not global transaction); returns per-id success/failure; **FGA enforced per product id for every action** — not just delete. Unauthorized ids come back as `status: "error"` entries; the bar never returns a global 403. If absent, M7d Task 1 adds a backend sub-task.

---

## 4. M7e — CSV import/export

### 4.0 Backend verification gate (M7e Task 0)

Before M7e Task 1 begins, Task 0 verifies and closes gaps on marketplace-api infrastructure:

1. No existing `csvjob` / `export` handler conflicts with the new surface.
2. GCS bucket + signed-upload infra from M5b is reusable for raw CSV uploads (same bucket, separate prefix `csv-imports/:storeId/`).
3. multipart upload size limit on the Gin engine supports ≤100 MB (50k rows × ~2 KB).
4. marketplace-api Knative `minScale` policy — confirm it can be raised to 1 from 0 dynamically (see §4.2 Knative mitigation).
5. `go-shared/authz/middleware` can gate new routes with the same pattern M5a uses.

If any gap exists, backend fix sub-tasks are inserted into M7e before Task 1 (migration).

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
| content_hash | text not null | sha256 of the uploaded CSV (idempotency) |
| error_csv_gcs_path | text null | streamed via GCS resumable upload |
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

**Indexes:**
- `(store_id, status, created_at desc)` — history list
- `(store_id, content_hash)` unique partial WHERE `status IN ('queued','running','paused')` — **retry idempotency**: if the same CSV content is submitted twice while an import is still in flight, the second submit returns the existing job id instead of creating a duplicate

#### Worker pattern

- **Single goroutine per job**, not per pod — `pg_try_advisory_lock(hashtext(job_id::text))` ensures only one pod processes a given job even across Knative replicas. Hash collision risk is called out as a landmine (§7).
- **Heartbeat** every 5s (`UPDATE csv_import_jobs SET heartbeat_at = now() WHERE id = $1`)
- **Checkpoint** `last_processed_row` every 10 rows
- **Crash recovery:** on startup, `main.go` runs a recovery scan using `SELECT … FOR UPDATE SKIP LOCKED` so multiple concurrently-starting pods never compete on the same orphan row. Rows matching `status='running' AND heartbeat_at < now() - 60s` are moved to `paused`, released, and re-enqueued. The scan itself is idempotent and safe to run on every pod start.
- **Cancellation:** service sets `status='cancelled'`, worker checks status every batch (10 rows) and aborts cleanly.
- **Error rows (streamed, not buffered):** the worker opens a GCS resumable upload session at job start and streams `row_number, raw_line, error_message` lines into it as errors occur. No in-memory error buffer. Memory stays bounded regardless of error count. The resumable session is finalized on job completion/failure/cancel.
- **Import cap:** 50k rows hard limit, enforced both at upload (header row count scan) and during processing (early abort + job marked `failed` if exceeded mid-scan). UI shows the limit on the upload page.

#### Knative scale-to-zero mitigation (critical)

The naive "pin a local counter" approach does not work — Knative's autoscaler reads concurrency from the queue-proxy sidecar, not from in-process state. Two options, both documented; **option A is the default, option B is the fallback if A proves operationally awkward:**

**Option A — conditional `minScale: 1` via the Kubernetes API.** When the `csvjob.Service` transitions the first job to `running`, it PATCHes the Knative Service annotation `autoscaling.knative.dev/minScale: "1"`. When the last non-terminal job completes, it PATCHes it back to `"0"`. marketplace-api's ServiceAccount needs `patch` on its own Knative Service resource — this is a small RBAC addition documented in M7e Task 7. Advantage: zero risk of losing in-flight work. Disadvantage: requires RBAC + the service can briefly scale up to 1 even when idle during patch propagation.

**Option B — fake long-lived in-flight request.** Worker opens a self-directed HTTP request to `/internal/csv-keepalive` that streams an empty body at 1 byte/sec until the job is done. The queue-proxy counts this as in-flight concurrency and keeps the pod alive. Simpler, no RBAC, but fragile and introduces a self-loop.

The landmine is real: without one of these, Knative will scale to zero mid-import on idle clusters and the job will be paused until the next external request arrives. Document whichever option ships.

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
- **Frontend:** `papaparse` (client-side preview parsing only, route-scoped import).

---

## 5. Testing strategy

All three plans follow the global TDD rule — tests written first, implementation second, verification last. 80% coverage on all new files.

### 5.1 M7c

- **Unit (Vitest in apps/admin):** `generateVariants()` — empty options, add option, remove option, rename value, **reorder options (key sorting)**, preserve stock on matching keys, orphan handling, 500-combination cap (≥9 cases)
- **Component (Vitest + RTL):** `OptionsEditor` add/remove/rename flows; `VariantMatrixTable` inline edit + column bulk apply; `MediaCard` overflow actions; `MediaCropDialog` open → crop → apply round-trip; `MediaUploader` 403 retry with fresh signed URL
- **Playwright E2E — two tests** (split from the original one):
  1. **Variants flow** — create product → add 2 options (Color, Size) with 2 values each → confirm 4 variants auto-generated → set all prices via column bulk → set one variant's sku inline → save → reload → assert full persistence
  2. **Media flow** — open existing product → upload 2 images via dropzone → crop one → assign it to the Red variant → drag-reorder → set alt text → save → reload → assert everything persists, then re-open crop on the cropped image and assert the original is re-downloadable

### 5.2 M7d

- **Unit:** `useProductSelection()` sessionStorage sync, 100-cap, cross-page persistence, multi-tab isolation
- **Component:** `BulkActionsBar` visibility by selection count; role-gated action rendering for staff/admin/owner; `CopyToStoreDialog` filters target stores by permission and excludes current store
- **Playwright E2E (1 test):** select 3 products on list page → click bulk archive → assert toast + row status updates → select 1 product → copy to another store → navigate to target store's list → assert product exists there
- **Backend (Go integration, if new bulk endpoint):** bulk archive atomicity, per-id partial-failure response shape, **per-id FGA denial for every action** (not just delete)

### 5.3 M7e

- **Unit (Go):** `parser.go` — valid row, missing required column, type coercion, malformed row, duplicate handle (table-driven); `worker.go` resume logic with mocked repository; advisory-lock contention test
- **Integration (Go, real Postgres + GCS emulator):** submit 100-row job → worker processes → all rows landed → `last_processed_row` advanced → error CSV streamed to GCS for bad rows
- **Crash-recovery integration test — concrete mechanism:** the test uses a context-cancel injection point in the worker's row loop (`if testCancelAt == currentRow { cancel() }`) to simulate a crash mid-import. Test flow: submit 50-row job → set `testCancelAt=25` → run worker → assert goroutine exits → run recovery scan → assert job moved to `paused` → re-enqueue → run worker again → assert resume starts at row 25, completes to 50, **no duplicate product rows in DB** (verified by count query on a per-row unique marker in the test CSV).
- **Content-hash idempotency test:** submit the same CSV twice back-to-back → assert the second call returns the first job's id without creating a second row.
- **Unit (frontend):** column mapping reducer; preview parser row count
- **Playwright E2E (1 test):** upload 10-row CSV with 2 intentionally bad rows → assert job page polls and shows progress → poll completes → assert 8 products created in list page → download error CSV → assert 2 error rows with row numbers
- **Export E2E:** trigger export of 5 selected products → assert downloaded CSV content matches expected columns and row count

---

## 6. Task sizing

> **Note:** M7e tasks 2–5 (model/repo/service/parser/worker) each pair a test task with an implementation task under TDD. Real task count during execution will expand to ~20. Flagged so executors size windows accordingly.

### M7c (~15 tasks)

0. Verify `mark8ly/.impeccable.md` exists; if not, run `teach-impeccable` once and commit
1. Backend verification gate — §2.7 items 1–7, insert backend fix sub-tasks if any gap, including the `gcs_path_original` column + recrop endpoint + 500-variant backend cap
2. `generateVariants()` pure function + 9 unit tests (TDD first, includes option-reorder case)
3. `MediaUploader` + signed URL client + 403 retry path
4. `MediaCropDialog` with `react-easy-crop` + re-crop round-trip
5. `MediaCard` + `MediaGrid` with drag-reorder
6. `MediaTab` composition + alt text + per-variant image picker
7. `OptionsEditor` + `OptionRow`
8. `VariantMatrixTable` + `VariantRow` inline edit
9. `VariantBulkBar` column actions
10. `ProductForm` tab integration + dirty tracking merge
11. Server action extension (PATCH full aggregate)
12. Component tests (Vitest + RTL)
13. Playwright E2E — variants flow + media flow (two tests)
14. Impeccable pass — `critique` → `polish` → `arrange` → `typeset` → `audit` → `adapt`; critique score ≥ 7.5
15. Verification + PR

**Exit:** merchant creates product with 2 options / 4 variants / 3 cropped images / 1 per-variant image, saves, reloads; everything persists; both Playwright tests green; 80% coverage on new files; impeccable chain passed.

### M7d (~11 tasks)

1. Backend verification — copy endpoint + bulk endpoint + per-id FGA on every action; insert backend sub-tasks if missing
2. `CopyToStoreDialog` + server action
3. `useProductSelection()` sessionStorage-backed hook with 100-cap
4. `BulkActionsBar` component + role-gated actions
5. Bulk archive/unarchive/publish/unpublish server actions
6. Bulk delete (owner only) + hard-delete confirm
7. Bulk category assign (reuse M7b picker)
8. Bulk copy-to-store
9. Component + E2E tests
10. Impeccable pass — `critique` → `polish` → `audit` → `adapt`; critique score ≥ 7.5
11. Verification + PR

**Exit:** 3-row bulk archive works; copy-to-store lands in target store; selection survives pagination and reload; per-id FGA enforced on every bulk action; impeccable chain passed.

### M7e (~18 tasks, mixed Go + Next.js)

0. Backend infra verification gate — §4.0 items 1–5
1. Migration `000004_csv_import_jobs` (includes `content_hash` column + unique partial index)
2. GORM model + repository + tests
3. `csvjob.Service` (Submit with content-hash dedupe, GetStatus, Cancel, Resume) + tests
4. `parser.go` with validation + table-driven tests
5. `worker.go` goroutine + heartbeat + advisory lock + streamed error CSV via GCS resumable upload
6. Crash-recovery integration test (context-cancel injection)
7. Wire worker into `main.go` — startup `FOR UPDATE SKIP LOCKED` recovery scan + shutdown drain + Knative minScale patch flow (option A)
8. HTTP handlers: submit, list, status, cancel, download error CSV
9. Export handler (streaming CSV)
10. FGA gates on all new endpoints
11. Frontend: `lib/api/csvImports.ts` typed client
12. `app/products/import/page.tsx` upload + column mapping
13. `app/products/import/[jobId]/page.tsx` status polling
14. `CsvImportHistory` table
15. Playwright E2E (import + export)
16. Impeccable pass on new UI surfaces — `critique` → `polish` → `audit` → `adapt`; critique score ≥ 7.5
17. Verification + PR

**Exit:** 100-row CSV imports with 2+ partial failures; error CSV downloadable with row numbers; export matches current filter; crash mid-import recovers cleanly with no duplicate rows; double-submit of same CSV is deduped by content hash; impeccable chain passed.

---

## 7. Risks and landmines

1. **Backend PATCH aggregate gaps (M7c)** — highest risk. M7c Task 1 must verify and close before any frontend work. Do not assume.
2. **Variant combination explosion** — capped at 500 client-side AND backend. Both caps are Task 1 deliverables.
3. **Variant key reorder stability** — swap two options in the UI and the keys must still match. Sorted key builder handles it; §5.1 has an explicit test case.
4. **GCS signed URL expiry** — 60 minutes. Client still retries once on 403 by fetching a fresh URL; the retry path has a test.
5. **Crop re-editing requires `gcs_path_original`** — treated as a M7c Task 1 backend deliverable, not an assumption.
6. **Knative scale-to-zero + CSV worker** — the naive concurrency-counter mitigation does NOT work. Use conditional `minScale: 1` patching (§4.2 option A) or the fake long-lived request (option B). Without one of these, jobs will hang on idle clusters. Not optional.
7. **Orphan recovery race across pods** — multiple concurrently-starting pods must not double-enqueue the same orphan. Use `FOR UPDATE SKIP LOCKED` in the recovery scan.
8. **Advisory lock hash collisions** — `hashtext(uuid::text)` to int64 has a tiny collision risk. Acceptable at this volume; document it.
9. **CSV row encoding** — enforce UTF-8, reject BOM-less Windows-1252 with a clear error, don't silently mangle.
10. **Bulk FGA per-id for ALL actions** — owner-only frontend gates are cosmetic. Backend MUST enforce per-product FGA on every bulk action (archive, unarchive, publish, unpublish, category-assign, delete, copy), not just delete. Unauthorized ids come back as partial failures.
11. **papaparse bundle size** — only imported on the `/products/import` route to keep the main list page lean.
12. **50k-row import hard cap** — enforced both at upload and during processing. UI shows the limit in the upload dialog.
13. **CSV import retry idempotency** — network drop during multipart upload could submit the same CSV twice. Dedupe via `(store_id, content_hash)` unique partial index on non-terminal statuses. Second submit returns the first job id.
14. **Error CSV memory bound** — stream via GCS resumable upload, never buffer in memory. A 50k-row import with 50k errors must still run with a bounded memory footprint.
15. **Selection URL length** — solved by storing selection ids in `sessionStorage` and keeping the URL to a count-only hash. Cap reduced from 200 to 100 to stay safely inside Istio header limits even if someone swaps to URL-backed selection later.
16. **`@dnd-kit` version alignment** — check marketplace-admin's existing lock file before installing; alignment concern even though no other mark8ly app uses it.

---

## 8. Verification checklist (all three milestones)

- [ ] All new Go files 80%+ coverage (`go test -cover`)
- [ ] All new TS files 80%+ coverage (Vitest)
- [ ] Playwright E2E green for each milestone (M7c has two tests)
- [ ] No new `logrus` / `slog` warnings
- [ ] No new `go vet` warnings
- [ ] No new ESLint errors
- [ ] Paper · Ink · Moss tokens only — no new hex values
- [ ] WCAG 2.1 AA on all new surfaces (keyboard nav, focus ring, screen reader labels)
- [ ] `prefers-reduced-motion` honored on all animations
- [ ] Skip link still first focusable on products list page
- [ ] **No new dialogs except hard-delete confirm** (per §13.5)
- [ ] Impeccable chain run on every milestone: `critique` score ≥ 7.5, `polish` pass applied, `audit` green, `adapt` verified across mobile/tablet/desktop
- [ ] `mark8ly/.impeccable.md` exists and is current
- [ ] No secrets committed; `.env.example` updated if any new config

---

## 9. Execution order

**Recommended:** M7c → M7d → M7e, serial.

**Alternative:** M7c → (M7d ∥ M7e), last two in parallel on separate branches if two executors are available. M7d and M7e have no shared files and no conflicting backend changes.

M7c must land first because it extends `ProductForm` — M7d's bulk "Assign category" and M7e's CSV column mapping both depend on the final shape of the product aggregate after M7c.
