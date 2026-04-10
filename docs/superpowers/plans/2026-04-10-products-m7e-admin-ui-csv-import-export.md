# Products M7e — Admin UI: CSV Import/Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the CSV surface for products admin — synchronous streaming **export** and async resumable **import** — across both `services/marketplace-api` (Go) and `apps/admin` (Next.js). Export is a single streaming handler with no job state. Import runs as an **in-process goroutine worker** per marketplace-api pod: submit → GCS upload → content-hash dedupe → queue → worker goroutine → per-row parse/validate/insert → streamed error CSV → completion. The worker survives pod restarts via a `FOR UPDATE SKIP LOCKED` recovery scan on startup, and keeps Knative from scaling to zero mid-job via a conditional `minScale: 1` patch on the marketplace-api Knative Service.

**Architecture:** Export is a pure streaming handler reusing the existing product repository. Import introduces a new `internal/csvjob/` package: model + repository + service + parser + worker. One goroutine per job, gated by `pg_try_advisory_lock(hashtext(job_id::text))` so multiple pods never process the same job. Heartbeat every 5s, checkpoint `last_processed_row` every 10 rows, cancellation observed every batch. Error rows stream directly into a GCS **resumable upload** session — never buffered in memory — so a 50k-row import with 50k errors stays memory-bounded. Content hash (sha256 of the uploaded CSV) plus a unique partial index `(store_id, content_hash) WHERE status IN ('queued','running','paused')` guarantees that double-submits from network retries collapse to the same job id. Frontend is a three-page flow under `app/products/import/`: dropzone + client-side preview + column mapping → per-job status page polling every 2s via server action + `router.refresh()` → history list on the landing page.

**Tech Stack:** Go 1.26 / Gin / GORM / Postgres / `encoding/csv` (stdlib) / `cloud.google.com/go/storage` (resumable uploads) / `k8s.io/client-go` (Knative `minScale` patch) under `services/marketplace-api`. Next.js 16 (App Router + server actions), React 19, Tailwind v4, `@tesserix/web` v1.7.1 primitives, `@repo/ui` promoted components, `papaparse` (client-side preview only, route-scoped), Vitest + RTL, Playwright 1.59+, Paper · Ink · Moss design tokens.

**Design Authority:** `docs/superpowers/specs/2026-04-10-products-admin-ui-slice-2-design.md` §4 (all subsections — backend gate, export, import, worker, Knative mitigation, endpoints, frontend), §5.3 (testing strategy), §6 M7e (task sizing), §7 landmines 6, 7, 8, 9, 11, 12, 13, 14.

---

## Status

> **Pending.** All tasks open. Current branch: `feat/products-m7d-m7e`.

---

## Scope check

Adds `services/marketplace-api/internal/csvjob/` (models, repository, service, parser, worker), a new migration (`000007_csv_import_jobs.up.sql` — the next available slot after `000006_store_watermarks_orders`), new handlers under `services/marketplace-api/internal/product/csv_handlers.go` (or a sibling `internal/csvjob/handlers.go` — decided in Task 1), and a streaming export handler on the existing product handler surface. On the frontend: adds `apps/admin/app/products/import/` route group, `apps/admin/components/products/csv/*`, and `apps/admin/lib/api/csvImports.ts`. Extends `BulkActionsBar` (landed by M7d) with an "Export selected" button and the list page header with "Export all".

Spec sections authoritative for this milestone:
- Design spec §4 (all subsections — backend gate, export, import schema, worker pattern, Knative mitigation, endpoints, frontend layout, libraries)
- Design spec §5.3 (testing strategy — unit, integration, crash-recovery injection, idempotency, E2E)
- Design spec §6 M7e (task sizing — ~18 tasks)
- Design spec §7 landmines 6 (Knative scale-to-zero), 7 (orphan recovery race), 8 (advisory lock collisions), 9 (CSV encoding), 11 (papaparse bundle), 12 (50k row cap), 13 (content-hash idempotency), 14 (error CSV memory bound)
- `mark8ly/.impeccable.md` — Paper · Ink · Moss design context
- Marketplace spec §7.8 (role-based UX), §7.9 (a11y), §13.1.1 (permission map)

**Out of scope (deferred to later milestones):**
- Export format variants (XLSX, JSON) — CSV only
- Incremental/streaming import from a remote URL — multipart upload only
- Image-bundled CSV imports (CSV + zip of media)
- Inventory multi-location CSV
- Variant matrix in import CSV (first pass imports base product rows only; variants via the admin UI after M7c)
- Category tree creation from import (existing categories referenced by handle only)

---

## Decisions locked (from the spec — do NOT re-debate)

1. **Worker pattern:** in-process goroutine, **NOT** Pub/Sub. One goroutine per job; `pg_try_advisory_lock(hashtext(job_id::text))` prevents cross-pod double-processing.
2. **Knative mitigation:** **Option A** — conditional `minScale: 1` patched via the Kubernetes API when the first non-terminal job appears; patched back to `"0"` when the last terminal. Option B (keepalive HTTP) is the documented fallback only.
3. **Row cap:** 50k hard limit, enforced at upload (header-scan row count) **and** during processing (abort + mark `failed`).
4. **Error CSV:** streamed via GCS resumable upload, **never** buffered in memory. A 50k-row import with 50k errors must run with bounded memory.
5. **Orphan recovery:** `SELECT … FOR UPDATE SKIP LOCKED` on marketplace-api startup. Multiple concurrently-starting pods never compete on the same orphan row.
6. **Idempotency:** `(store_id, content_hash)` unique partial index on non-terminal statuses. Double-submit of the same CSV returns the existing job id.
7. **Frontend polling:** 2s interval via server action + `router.refresh()` (Next 16 pattern). Stops when `status ∈ {completed, failed, cancelled}`.
8. **papaparse:** client-side preview only, imported **only** on the `/products/import` route so the main list page bundle stays lean.
9. **CSV encoding:** UTF-8 enforced; BOM-less Windows-1252 rejected with a clear error. No silent mangling.
10. **Design system:** Paper · Ink · Moss tokens, Source Serif 4 display, Source Sans 3 body, `@tesserix/web` primitives first, `@repo/ui` promoted flat-file components second. No new hex values. No new dialogs except hard-delete confirm.
11. **Impeccable chain is a gate.** Task 0 verifies `mark8ly/.impeccable.md` exists. Task 17 runs the full chain (`frontend-design` → `critique` → `polish` → `arrange` → `typeset` → `audit` → `adapt`) with a `critique` score ≥ 7.5 threshold on the new import UI surfaces.

---

## File structure produced by M7e

### New backend files

```
services/marketplace-api/
  migrations/
    000007_csv_import_jobs.up.sql
    000007_csv_import_jobs.down.sql
  internal/csvjob/
    models.go                      CsvImportJob entity + status constants
    repository.go                  CRUD + FindOrphanedForUpdate + UpsertByContentHash
    repository_test.go
    service.go                     Submit / GetStatus / Cancel / Resume / List
    service_test.go
    parser.go                      CSV → ProductDraft + row validation
    parser_test.go                 table-driven
    worker.go                      goroutine + heartbeat + advisory lock + GCS resumable error upload
    worker_test.go                 crash-recovery injection + resume
    knative_minscale.go            k8s client-go patch helper (option A)
    knative_minscale_test.go
    handlers.go                    HTTP handlers: submit, list, status, cancel, errors.csv
  internal/product/
    export_handler.go              streaming CSV export
    export_handler_test.go
  cmd/marketplace-api/main.go      (modify — wire csvjob service + startup recovery scan + shutdown drain)
```

### New frontend files

```
apps/admin/
  lib/api/
    csvImports.ts                  typed client: submit, list, status, cancel, errors URL, export URLs
    csvImports.test.ts
  app/products/
    actions.ts                     (modify — extend with csv submit/cancel/poll server actions)
    import/
      page.tsx                     upload + preview + column mapping (client)
      [jobId]/
        page.tsx                   live status polling via server action + router.refresh
  components/products/csv/
    CsvUploadDropzone.tsx
    CsvPreviewTable.tsx            first 5 rows via papaparse (client-side)
    CsvColumnMapping.tsx           map CSV columns → product fields
    CsvImportHistory.tsx           history table (landing)
    CsvJobProgress.tsx             progress bar + row counters
    CsvErrorSummary.tsx            download error CSV link
  tests/e2e/
    products-csv-import.spec.ts    E2E 1
    products-csv-export.spec.ts    E2E 2
```

### New npm dependency (`apps/admin/package.json`)

- `papaparse@^5` + `@types/papaparse@^5` — client-side CSV preview parsing, route-scoped import under `app/products/import/` only.

### New Go dependencies (`services/marketplace-api/go.mod`)

- `k8s.io/client-go` + `k8s.io/apimachinery` (if not already present) — Knative Service PATCH for option A `minScale` flow. Verify first in Task 1; if this drags transitive weight that's unacceptable, fall back to option B (keepalive HTTP) and document.

---

## Landmines

1. **Knative scale-to-zero mid-import.** The naive "pin a local concurrency counter" mitigation does **not** work — Knative reads concurrency from the queue-proxy sidecar, not from in-process state. Ship option A (conditional `minScale: 1` patch) or, as documented fallback, option B (fake long-lived keepalive HTTP). Without one of these, jobs hang on idle clusters until the next external request.
2. **Orphan recovery race across pods.** Multiple concurrently-starting marketplace-api pods must never double-enqueue the same orphan row. Use `SELECT … FOR UPDATE SKIP LOCKED` in the startup recovery scan.
3. **Advisory lock hash collisions.** `hashtext(uuid::text)` → int64 has a non-zero (but negligible) collision risk at this volume. Document, do not design around.
4. **GCS resumable upload memory bounds.** The error CSV must be opened as a **resumable** session at job start and streamed row-by-row via a bounded writer. Never accumulate error rows in a slice. Finalize on completion/failure/cancel.
5. **papaparse bundle size.** Import only under `app/products/import/` (route-scoped). Do not pull into the list page or any shared layout.
6. **50k row cap.** Enforced twice — at upload (header-scan count) and during processing (abort + mark `failed`). UI surfaces the limit on the upload page.
7. **CSV encoding.** UTF-8 enforced. BOM-less Windows-1252 must be rejected with a clear typed error, not silently mangled. `encoding/csv` is encoding-agnostic; explicit validation lives in `parser.go`.
8. **Content-hash idempotency.** sha256 the **raw uploaded bytes** before GCS write. The unique partial index `(store_id, content_hash) WHERE status IN ('queued','running','paused')` catches double-submits from network retries. The second submit must return the **first** job id, never a 409.
9. **RBAC for `minScale` patch (option A).** marketplace-api's ServiceAccount needs `patch` on its own Knative `Service` resource. Task 7 documents and emits the Role + RoleBinding YAML. Without this, option A throws 403 at runtime.
10. **Shutdown drain.** On SIGTERM, the worker must mark in-flight jobs `paused` and release the advisory lock **before** exit, otherwise the next pod waits for the lock timeout.
11. **Heartbeat staleness window.** 60s threshold on `heartbeat_at < now() - 60s`. Tighter thresholds churn recovery scans; looser ones delay resume. 60s is the locked default.
12. **Knative `minScale` patch propagation.** Option A has a brief window (~3-10s) where the patch is accepted but the autoscaler hasn't acted. Don't assume the patch is instant; the worker must not rely on minScale being effective *before* the first heartbeat.
13. **Export streaming back-pressure.** `http.ResponseWriter` doesn't flush by default. Call `w.(http.Flusher).Flush()` after every N (e.g. 100) rows so large exports don't buffer the entire CSV in memory.
14. **Paper · Ink · Moss tokens only.** No new hex values. No terracotta/sage/cream legacy aliases in new code.

---

## Task decomposition

**18 tasks**, dependency-ordered. Task 0 and Task 1 are gates. Tasks 2–10 are backend, serial. Tasks 11–15 are frontend, sequential after Task 10 lands the HTTP contract. Tasks 16–17 are verification.

Legend: **R** = repository, **S** = service, **U** = unit/pure, **I** = integration (needs Postgres or GCS emulator), **C** = component (RTL), **E** = E2E (Playwright).

---

### Task 0: Impeccable design context check

**Files:** none (verification only)

**Scope:** Ensure `mark8ly/.impeccable.md` exists and is current before any UI code is written. This pins Paper · Ink · Moss design context for the `frontend-design` / `critique` / `polish` chain used in Task 17.

- [ ] **Step 1: Check for the file**

```bash
test -f mark8ly/.impeccable.md && echo "OK" || echo "MISSING"
```

Expected: `OK`. If `MISSING`, stop and run the `teach-impeccable` skill to generate it, then commit the result before continuing.

- [ ] **Step 2: Verify it mentions Paper · Ink · Moss**

```bash
grep -q "Paper" mark8ly/.impeccable.md && grep -q "Ink" mark8ly/.impeccable.md && grep -q "Moss" mark8ly/.impeccable.md && echo "OK" || echo "STALE"
```

Expected: `OK`. If `STALE`, re-run `teach-impeccable` and re-verify.

- [ ] **Step 3: Commit (only if regenerated)**

```bash
git add mark8ly/.impeccable.md
git commit -m "chore(impeccable): refresh design context for M7e"
```

---

### Task 1: Backend infrastructure verification gate

**Files (investigation only):**
- Read: `services/marketplace-api/cmd/marketplace-api/main.go`
- Read: `services/marketplace-api/internal/product/handlers.go`
- Read: `services/marketplace-api/internal/product/repository.go`
- Read: `services/marketplace-api/go.mod`
- Read: `services/marketplace-api/migrations/` (to confirm next slot)
- Read: `tesserix-infra/k8s/apps/marketplace/marketplace-api/` (RBAC, Knative Service annotations)

**Scope:** Close every infrastructure gap required by M7e before implementation begins. Spec §4.0 lists five verification items. Each gap becomes a numbered sub-task with its own failing test and commit. Do **not** start Task 2 until every row in the exit matrix is ✅.

- [ ] **Step 1: Run the existing marketplace-api suite to confirm the baseline is green**

```bash
cd services/marketplace-api
go test ./... -race
```

Expected: all green. If anything is red, stop — fix baseline before adding new surface area.

- [ ] **Step 2: Catalog verification items in `.planning/m7e-infra-gaps.md`**

Spec §4.0 verification list:

1. No existing `csvjob` / product export handler that conflicts with the new surface (`grep -r csvjob internal/` + `grep -r export.csv internal/product/`).
2. GCS bucket + signed-upload infra from M5b is reusable for raw CSV uploads (same bucket, separate prefix `csv-imports/:storeId/`).
3. Multipart upload size limit on the Gin engine supports ≤100 MB (50k rows × ~2 KB). Check `router.MaxMultipartMemory` and any reverse proxy limits (Istio VirtualService / Knative `timeoutSeconds`).
4. marketplace-api Knative `minScale` policy — confirm it can be raised to 1 from 0 dynamically. Inspect the Knative Service manifest under `tesserix-infra/k8s/apps/marketplace/marketplace-api/` and the service's current RBAC.
5. `go-shared/authz/middleware` can gate new routes with the same `fgaMw.Require(...)` pattern M5a uses.

Mark each with `supported` / `gap`.

- [ ] **Step 3: For each gap, write a failing test or failing manifest-diff, then fix, then commit**

Example for item 3 (multipart limit):

```go
// internal/csvjob/handlers_test.go
func TestSubmit_RejectsOver100MB(t *testing.T) {
    router := testrouter.New(t)
    body := &bytes.Buffer{}
    w := multipart.NewWriter(body)
    f, _ := w.CreateFormFile("file", "big.csv")
    io.CopyN(f, rand.Reader, 101*1024*1024)
    w.Close()
    req := httptest.NewRequest("POST", "/api/v1/admin/stores/s1/products/csv-imports", body)
    req.Header.Set("Content-Type", w.FormDataContentType())
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
```

Run, fail, implement minimal fix in `main.go` (`router.MaxMultipartMemory = 100 << 20`), pass, commit.

Example for item 4 (Knative `minScale` patch RBAC):

Write a `Role` + `RoleBinding` YAML under `tesserix-infra/k8s/apps/marketplace/marketplace-api/rbac-minscale.yaml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: marketplace-api-knative-patcher
  namespace: marketplace
rules:
  - apiGroups: ["serving.knative.dev"]
    resources: ["services"]
    resourceNames: ["marketplace-api"]
    verbs: ["get", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: marketplace-api-knative-patcher
  namespace: marketplace
subjects:
  - kind: ServiceAccount
    name: marketplace-api
    namespace: marketplace
roleRef:
  kind: Role
  name: marketplace-api-knative-patcher
  apiGroup: rbac.authorization.k8s.io
```

Commit manifest and reference it from the Kustomization.

- [ ] **Step 4: Fill the exit matrix before closing Task 1**

| # | Spec §4.0 verification item | Evidence / test name | Status | Commit |
|---|---|---|---|---|
| 1 | No existing `csvjob`/export handler conflict | grep output in `.planning/m7e-infra-gaps.md` | ⬜ | `_________` |
| 2 | GCS M5b bucket reusable under `csv-imports/:storeId/` prefix | `TestCsvJob_GcsPrefixResolves` | ⬜ | `_________` |
| 3 | Gin multipart limit supports ≤100 MB | `TestSubmit_RejectsOver100MB` | ⬜ | `_________` |
| 4 | Knative Service is patchable by marketplace-api SA (option A) | `rbac-minscale.yaml` merged + `TestKnativeMinScale_PatchSucceeds` (integration w/ kind cluster or mocked dynamic client) | ⬜ | `_________` |
| 5 | `go-shared/authz/middleware` gates new routes via `fgaMw.Require(...)` | `TestCsvHandlers_FGAGated` | ⬜ | `_________` |

Exit criteria: all 5 rows ✅, `.planning/m7e-infra-gaps.md` fully closed, `go test ./... -race` green, next migration slot confirmed as `000007`.

- [ ] **Step 5: Decide Knative mitigation**

Default = option A. If Task 1 surfaces that the transitive weight of `k8s.io/client-go` is unacceptable (binary size +30 MB, CVE surface, etc.), fall back to option B (keepalive HTTP) and rewrite Task 5 + Task 7 accordingly. Record the decision in `.planning/m7e-infra-gaps.md` and move on — do NOT re-debate mid-milestone.

---

### Task 2: Migration `000007_csv_import_jobs` + GORM model + repository + tests (R, I)

**Files:**
- Create: `services/marketplace-api/migrations/000007_csv_import_jobs.up.sql`
- Create: `services/marketplace-api/migrations/000007_csv_import_jobs.down.sql`
- Create: `services/marketplace-api/internal/csvjob/models.go`
- Create: `services/marketplace-api/internal/csvjob/repository.go`
- Create: `services/marketplace-api/internal/csvjob/repository_test.go`

**Scope:** Land the schema, GORM model, and repository layer first — everything downstream depends on the persistence contract. The migration includes the **unique partial index** for content-hash idempotency and the composite index for history listing. Repository tests run against real Postgres via the existing `testdb.Setup(t)` helper.

- [ ] **Step 1: Write the up migration**

```sql
-- services/marketplace-api/migrations/000007_csv_import_jobs.up.sql
CREATE TABLE csv_import_jobs (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id             uuid NOT NULL,
    user_id              text NOT NULL,
    gcs_path             text NOT NULL,
    content_hash         text NOT NULL,
    error_csv_gcs_path   text,
    status               text NOT NULL DEFAULT 'queued',
    total_rows           integer,
    last_processed_row   integer NOT NULL DEFAULT 0,
    success_count        integer NOT NULL DEFAULT 0,
    error_count          integer NOT NULL DEFAULT 0,
    heartbeat_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT csv_import_jobs_status_chk
        CHECK (status IN ('queued','running','paused','completed','failed','cancelled')),
    CONSTRAINT csv_import_jobs_progress_chk
        CHECK (total_rows IS NULL OR last_processed_row <= total_rows)
);

CREATE INDEX csv_import_jobs_store_status_created_idx
    ON csv_import_jobs (store_id, status, created_at DESC);

-- Retry idempotency: if the same CSV content is submitted twice while a prior
-- import is still in flight, the second submit collapses to the first job row.
CREATE UNIQUE INDEX csv_import_jobs_dedupe_idx
    ON csv_import_jobs (store_id, content_hash)
    WHERE status IN ('queued','running','paused');
```

- [ ] **Step 2: Write the down migration**

```sql
-- services/marketplace-api/migrations/000007_csv_import_jobs.down.sql
DROP INDEX IF EXISTS csv_import_jobs_dedupe_idx;
DROP INDEX IF EXISTS csv_import_jobs_store_status_created_idx;
DROP TABLE IF EXISTS csv_import_jobs;
```

- [ ] **Step 3: Apply migration locally and inspect**

```bash
cd services/marketplace-api
migrate -path migrations -database "$DATABASE_URL" up
psql "$DATABASE_URL" -c "\d csv_import_jobs"
```

Verify constraints and both indexes are present.

- [ ] **Step 4: Write the GORM model**

```go
// services/marketplace-api/internal/csvjob/models.go
package csvjob

import (
    "time"

    "github.com/google/uuid"
)

type Status string

const (
    StatusQueued    Status = "queued"
    StatusRunning   Status = "running"
    StatusPaused    Status = "paused"
    StatusCompleted Status = "completed"
    StatusFailed    Status = "failed"
    StatusCancelled Status = "cancelled"
)

// IsTerminal reports whether the job will not transition further without
// operator action. Used by main.go to decide when to patch Knative minScale
// back to 0.
func (s Status) IsTerminal() bool {
    switch s {
    case StatusCompleted, StatusFailed, StatusCancelled:
        return true
    }
    return false
}

type CsvImportJob struct {
    ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    StoreID           uuid.UUID  `gorm:"type:uuid;not null;index:csv_import_jobs_store_status_created_idx,priority:1"`
    UserID            string     `gorm:"type:text;not null"`
    GcsPath           string     `gorm:"type:text;not null"`
    ContentHash       string     `gorm:"type:text;not null"`
    ErrorCsvGcsPath   *string    `gorm:"type:text"`
    Status            Status     `gorm:"type:text;not null;default:queued;index:csv_import_jobs_store_status_created_idx,priority:2"`
    TotalRows         *int       `gorm:"type:integer"`
    LastProcessedRow  int        `gorm:"type:integer;not null;default:0"`
    SuccessCount      int        `gorm:"type:integer;not null;default:0"`
    ErrorCount        int        `gorm:"type:integer;not null;default:0"`
    HeartbeatAt       *time.Time `gorm:"type:timestamptz"`
    CreatedAt         time.Time  `gorm:"type:timestamptz;not null;default:now();index:csv_import_jobs_store_status_created_idx,priority:3,sort:desc"`
    UpdatedAt         time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

func (CsvImportJob) TableName() string { return "csv_import_jobs" }
```

- [ ] **Step 5: Write failing repository tests**

Create `repository_test.go` with: `TestCreate_Success`, `TestUpsertByContentHash_ReturnsExistingWhenInFlight`, `TestUpsertByContentHash_AllowsNewAfterTerminal`, `TestFindOrphanedForUpdate_SkipsLocked`, `TestHeartbeat`, `TestCheckpoint`, `TestMarkStatus`, `TestListByStore_Paginated`. Run — expect FAIL.

- [ ] **Step 6: Implement `repository.go`**

Key method — the FOR UPDATE SKIP LOCKED recovery scan:

```go
// services/marketplace-api/internal/csvjob/repository.go

// FindOrphanedForUpdate returns and row-locks jobs whose heartbeat is stale.
// The SKIP LOCKED clause guarantees that two concurrently-starting pods never
// compete on the same orphan row — the loser gets a different slice or an
// empty result and moves on.
func (r *Repository) FindOrphanedForUpdate(ctx context.Context, staleAfter time.Duration) ([]CsvImportJob, error) {
    var jobs []CsvImportJob
    err := r.db.WithContext(ctx).
        Raw(`
            SELECT *
            FROM csv_import_jobs
            WHERE status = 'running'
              AND heartbeat_at < now() - (? || ' seconds')::interval
            FOR UPDATE SKIP LOCKED
        `, int(staleAfter.Seconds())).
        Scan(&jobs).Error
    return jobs, err
}
```

And the content-hash dedupe upsert:

```go
// UpsertByContentHash attempts to insert a new queued job. If an in-flight job
// already exists for (store_id, content_hash), the unique partial index
// fires and we return the existing row instead. This collapses retries from
// network-flaked multipart uploads into a single job.
func (r *Repository) UpsertByContentHash(ctx context.Context, job *CsvImportJob) (*CsvImportJob, bool, error) {
    err := r.db.WithContext(ctx).
        Clauses(clause.OnConflict{DoNothing: true}).
        Create(job).Error
    if err != nil {
        return nil, false, err
    }
    // If insert was skipped, fetch the existing in-flight job.
    var existing CsvImportJob
    err = r.db.WithContext(ctx).
        Where("store_id = ? AND content_hash = ? AND status IN ?",
            job.StoreID, job.ContentHash,
            []Status{StatusQueued, StatusRunning, StatusPaused}).
        First(&existing).Error
    if err == nil && existing.ID != job.ID {
        return &existing, true, nil // deduped
    }
    if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, false, err
    }
    return job, false, nil // newly created
}
```

Run tests — expect PASS.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/migrations/000007_csv_import_jobs.*.sql services/marketplace-api/internal/csvjob/models.go services/marketplace-api/internal/csvjob/repository.go services/marketplace-api/internal/csvjob/repository_test.go
git commit -m "feat(marketplace-api): csv_import_jobs schema + repository with content-hash dedupe (M7e)"
```

---

### Task 3: `csvjob.Service` — Submit / GetStatus / Cancel / Resume / List (S, U)

**Files:**
- Create: `services/marketplace-api/internal/csvjob/service.go`
- Create: `services/marketplace-api/internal/csvjob/service_test.go`

**Scope:** The orchestration layer between HTTP handlers and the worker. `Submit` sha256s the raw upload bytes, writes to GCS under `csv-imports/:storeId/:jobId.csv`, and calls `repository.UpsertByContentHash`. `GetStatus` is a simple read. `Cancel` sets status to `cancelled` — the worker observes the flip on its next batch. `Resume` flips `paused` → `queued` and re-enqueues.

- [ ] **Step 1: Write failing service tests** — `TestSubmit_NewJob`, `TestSubmit_DedupesIdenticalContent`, `TestSubmit_AllowsNewAfterPriorCompleted`, `TestCancel_InFlight`, `TestCancel_AlreadyTerminalIsNoOp`, `TestResume_PausedJob`, `TestGetStatus_NotFound`, `TestList_Paginated`.

- [ ] **Step 2: Implement `service.go`**

Key method — content-hash computation + dedupe:

```go
// Submit computes sha256 over the raw upload, writes to GCS, and upserts
// a csv_import_jobs row gated by the (store_id, content_hash) unique
// partial index. A second submit of the same bytes while a prior job is
// still in flight collapses to the existing job id — no HTTP 409, no
// duplicate row.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*CsvImportJob, bool, error) {
    hash := sha256.Sum256(in.RawBytes)
    contentHash := hex.EncodeToString(hash[:])

    gcsPath := fmt.Sprintf("csv-imports/%s/%s.csv", in.StoreID, uuid.New())
    if err := s.gcs.Write(ctx, gcsPath, bytes.NewReader(in.RawBytes)); err != nil {
        return nil, false, fmt.Errorf("gcs write: %w", err)
    }

    job := &CsvImportJob{
        StoreID:     in.StoreID,
        UserID:      in.UserID,
        GcsPath:     gcsPath,
        ContentHash: contentHash,
        Status:      StatusQueued,
    }
    stored, deduped, err := s.repo.UpsertByContentHash(ctx, job)
    if err != nil {
        return nil, false, err
    }
    if !deduped {
        // Non-blocking: the worker loop will pick this up on its next tick.
        s.enqueue(stored.ID)
    }
    return stored, deduped, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/csvjob/service.go services/marketplace-api/internal/csvjob/service_test.go
git commit -m "feat(marketplace-api): csvjob.Service with Submit/GetStatus/Cancel/Resume (M7e)"
```

---

### Task 4: `parser.go` — CSV → ProductDraft + row-level validation (U)

**Files:**
- Create: `services/marketplace-api/internal/csvjob/parser.go`
- Create: `services/marketplace-api/internal/csvjob/parser_test.go`

**Scope:** Pure stateless CSV row → `ProductDraft` translation with row-level validation. Table-driven tests cover: valid row, missing required column, type coercion (price as string → decimal), malformed row (wrong column count), duplicate handle within the same file, BOM-less Windows-1252 rejection, empty file, header-only file. Returns either a `ProductDraft` or a typed `RowError` that carries `row_number`, `raw_line`, `error_message` — the exact shape streamed into the GCS error CSV by the worker.

- [ ] **Step 1: Write table-driven failing tests**

```go
// parser_test.go
func TestParseRow(t *testing.T) {
    cases := []struct {
        name    string
        header  []string
        row     []string
        want    *ProductDraft
        wantErr string
    }{
        {
            name:   "valid row",
            header: []string{"handle", "title", "status", "base_price"},
            row:    []string{"black-tee", "Black Tee", "draft", "19.99"},
            want: &ProductDraft{
                Handle: "black-tee", Title: "Black Tee",
                Status: "draft", BasePrice: "19.99",
            },
        },
        {
            name:    "missing required column",
            header:  []string{"handle", "status"},
            row:     []string{"x", "draft"},
            wantErr: "missing required column: title",
        },
        {
            name:    "malformed price",
            header:  []string{"handle", "title", "status", "base_price"},
            row:     []string{"x", "X", "draft", "not-a-number"},
            wantErr: "base_price: invalid decimal",
        },
        {
            name:    "wrong column count",
            header:  []string{"handle", "title", "status", "base_price"},
            row:     []string{"x", "X"},
            wantErr: "expected 4 columns, got 2",
        },
        // ...
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            p := NewParser(tc.header)
            got, err := p.ParseRow(1, tc.row)
            if tc.wantErr != "" {
                require.ErrorContains(t, err, tc.wantErr)
                return
            }
            require.NoError(t, err)
            require.Equal(t, tc.want, got)
        })
    }
}

func TestParser_RejectsBOMlessWindows1252(t *testing.T) {
    // 0xE9 is "é" in Windows-1252 but an invalid UTF-8 continuation byte.
    raw := []byte("handle,title,status,base_price\nblack-t\xE9e,Black,draft,19.99\n")
    _, err := ParseAll(bytes.NewReader(raw))
    require.ErrorIs(t, err, ErrNotUTF8)
}
```

- [ ] **Step 2: Implement `parser.go`**, then PASS, then commit.

```bash
git add services/marketplace-api/internal/csvjob/parser.go services/marketplace-api/internal/csvjob/parser_test.go
git commit -m "feat(marketplace-api): csvjob parser with row validation and UTF-8 enforcement (M7e)"
```

---

### Task 5: `worker.go` — goroutine + heartbeat + advisory lock + streamed error CSV (S, I)

**Files:**
- Create: `services/marketplace-api/internal/csvjob/worker.go`
- Create: `services/marketplace-api/internal/csvjob/worker_test.go`

**Scope:** The core worker loop. One goroutine per job. Acquires `pg_try_advisory_lock(hashtext(job_id::text))` — if the lock is held by another pod, exits cleanly. Opens a GCS resumable upload session for the error CSV **before** the first row is processed. Reads the source CSV streamed from GCS row-by-row, calls `parser.ParseRow`, on success inserts the product via the existing `product.Service.Create`, on error writes one line into the resumable error stream. Heartbeat goroutine refreshes `heartbeat_at` every 5s. Checkpoint `last_processed_row` every 10 rows. Cancellation observed at every batch boundary via a `status` SELECT.

- [ ] **Step 1: Write failing worker tests**

- [ ] **Step 2: Implement the advisory-lock guard**

```go
// worker.go

// tryAcquireJobLock attempts to grab a Postgres advisory lock keyed by the
// job id. The lock is bound to the session that took it and auto-released
// on session close, so a pod crash releases without manual cleanup.
//
// LANDMINE: hashtext(uuid::text) → int64 has a tiny (but non-zero) collision
// risk. Accepted at M7e volumes; do not design around it.
func (w *Worker) tryAcquireJobLock(ctx context.Context, sess *gorm.DB, jobID uuid.UUID) (bool, error) {
    var acquired bool
    err := sess.WithContext(ctx).
        Raw(`SELECT pg_try_advisory_lock(hashtext(?::text))`, jobID.String()).
        Scan(&acquired).Error
    return acquired, err
}

func (w *Worker) releaseJobLock(ctx context.Context, sess *gorm.DB, jobID uuid.UUID) error {
    return sess.WithContext(ctx).
        Exec(`SELECT pg_advisory_unlock(hashtext(?::text))`, jobID.String()).Error
}
```

- [ ] **Step 3: Streamed error CSV via GCS resumable upload**

```go
// errorSink wraps a GCS resumable upload writer so error rows are flushed
// row-by-row. LANDMINE (spec §7.14): the error CSV must NEVER be buffered in
// memory. A 50k-row import where every row errors must still run with a
// bounded memory footprint.
type errorSink struct {
    obj    *storage.ObjectHandle
    writer *storage.Writer
    csv    *csv.Writer
}

func newErrorSink(ctx context.Context, bucket *storage.BucketHandle, path string) (*errorSink, error) {
    obj := bucket.Object(path)
    w := obj.NewWriter(ctx) // resumable upload session
    w.ChunkSize = 256 << 10 // 256 KB — bounded
    csvw := csv.NewWriter(w)
    if err := csvw.Write([]string{"row_number", "raw_line", "error_message"}); err != nil {
        _ = w.Close()
        return nil, err
    }
    return &errorSink{obj: obj, writer: w, csv: csvw}, nil
}

func (s *errorSink) WriteRow(rowNum int, rawLine, errMsg string) error {
    if err := s.csv.Write([]string{strconv.Itoa(rowNum), rawLine, errMsg}); err != nil {
        return err
    }
    s.csv.Flush()
    return s.csv.Error()
}

func (s *errorSink) Close() error {
    s.csv.Flush()
    if err := s.csv.Error(); err != nil {
        _ = s.writer.Close()
        return err
    }
    return s.writer.Close()
}
```

- [ ] **Step 4: Worker main loop with crash-injection hook**

```go
// Run executes the job. The testCancelAt hook is a deliberate injection point
// for the crash-recovery integration test (Task 6): set it non-zero in tests
// to simulate a SIGKILL at a specific row.
type Worker struct {
    repo       *Repository
    gcs        *storage.BucketHandle
    productSvc ProductCreator
    db         *gorm.DB

    // Test-only: when > 0, the worker cancels its context after processing
    // this row number. Leave at 0 in production.
    testCancelAt int
}

func (w *Worker) Run(ctx context.Context, jobID uuid.UUID) error {
    sess := w.db.Session(&gorm.Session{NewDB: true})
    defer sess.Rollback()

    acquired, err := w.tryAcquireJobLock(ctx, sess, jobID)
    if err != nil {
        return fmt.Errorf("advisory lock: %w", err)
    }
    if !acquired {
        return nil // another pod owns this job
    }
    defer w.releaseJobLock(context.Background(), sess, jobID)

    job, err := w.repo.MarkRunning(ctx, jobID)
    if err != nil {
        return err
    }

    sink, err := newErrorSink(ctx, w.gcs, fmt.Sprintf("csv-imports/%s/%s-errors.csv", job.StoreID, jobID))
    if err != nil {
        return err
    }
    defer sink.Close()

    hbCtx, hbCancel := context.WithCancel(ctx)
    defer hbCancel()
    go w.heartbeatLoop(hbCtx, jobID, 5*time.Second)

    return w.processRows(ctx, job, sink)
}
```

- [ ] **Step 5: Run unit tests — PASS**, commit.

```bash
git add services/marketplace-api/internal/csvjob/worker.go services/marketplace-api/internal/csvjob/worker_test.go
git commit -m "feat(marketplace-api): csvjob worker with advisory lock and streamed error CSV (M7e)"
```

---

### Task 6: Crash-recovery integration test (I)

**Files:**
- Create: `services/marketplace-api/internal/csvjob/worker_recovery_test.go`

**Scope:** Prove that a mid-import crash recovers cleanly with no duplicate product rows. Uses the `testCancelAt` injection point landed in Task 5. Runs against real Postgres + GCS emulator via `testdb.Setup(t)`.

- [ ] **Step 1: Write the recovery test**

```go
// worker_recovery_test.go

// TestWorker_CrashRecovery_NoDuplicateRows proves spec §5.3:
//   submit 50-row job → cancel at row 25 → recovery scan marks paused →
//   re-enqueue → resume starts at row 25 → completes → exactly 50 products,
//   not 75 or 100.
//
// The per-row unique marker (handle = "row-<n>") makes duplicate detection
// trivial: a count query MUST return 50, not more.
func TestWorker_CrashRecovery_NoDuplicateRows(t *testing.T) {
    ctx, db, cleanup := testdb.Setup(t)
    defer cleanup()

    storeID := testdb.SeedStore(t, db)
    csvBytes := build50RowCSV(t) // row handles "row-1" ... "row-50"

    repo := csvjob.NewRepository(db)
    gcs := testgcs.Bucket(t)
    productSvc := product.NewService(product.NewRepository(db), nil, nil)
    svc := csvjob.NewService(repo, gcs)

    job, _, err := svc.Submit(ctx, csvjob.SubmitInput{
        StoreID: storeID, UserID: "tester", RawBytes: csvBytes,
    })
    require.NoError(t, err)

    // First run: crash at row 25.
    crashing := csvjob.NewWorker(repo, gcs, productSvc, db)
    crashing.SetTestCancelAt(25)
    crashCtx, crashCancel := context.WithCancel(ctx)
    _ = crashing.Run(crashCtx, job.ID)
    crashCancel()

    // Simulate staleness: push heartbeat_at into the past.
    require.NoError(t, db.Exec(
        `UPDATE csv_import_jobs SET heartbeat_at = now() - interval '2 minutes' WHERE id = ?`,
        job.ID,
    ).Error)

    // Recovery scan — marks the job paused, re-enqueues.
    recovered, err := repo.FindOrphanedForUpdate(ctx, 60*time.Second)
    require.NoError(t, err)
    require.Len(t, recovered, 1)
    require.NoError(t, repo.MarkPaused(ctx, recovered[0].ID))

    // Resume: fresh worker picks up at last_processed_row=25.
    resuming := csvjob.NewWorker(repo, gcs, productSvc, db)
    require.NoError(t, resuming.Run(ctx, job.ID))

    // Assertion: exactly 50 products with the expected handles.
    var count int64
    require.NoError(t, db.
        Model(&product.Product{}).
        Where("store_id = ? AND handle LIKE ?", storeID, "row-%").
        Count(&count).Error)
    require.Equal(t, int64(50), count, "expected 50 unique product rows, got %d", count)

    final, err := repo.GetByID(ctx, job.ID)
    require.NoError(t, err)
    require.Equal(t, csvjob.StatusCompleted, final.Status)
    require.Equal(t, 50, final.SuccessCount)
}
```

- [ ] **Step 2: Run, PASS, commit**

```bash
git add services/marketplace-api/internal/csvjob/worker_recovery_test.go
git commit -m "test(marketplace-api): csvjob crash-recovery integration test (M7e)"
```

---

### Task 7: Wire worker into `main.go` — startup recovery scan + shutdown drain + Knative minScale patch (S)

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Create: `services/marketplace-api/internal/csvjob/knative_minscale.go`
- Create: `services/marketplace-api/internal/csvjob/knative_minscale_test.go`
- Modify: `tesserix-infra/k8s/apps/marketplace/marketplace-api/rbac-minscale.yaml` (landed by Task 1)

**Scope:** On startup, run the `FOR UPDATE SKIP LOCKED` recovery scan; for each orphan mark `paused` then re-enqueue. Install a background loop that dequeues and runs workers. On SIGTERM, drain: stop accepting new work, mark in-flight jobs `paused`, release advisory locks, exit. When the first non-terminal job appears, PATCH the Knative Service `minScale` annotation to `"1"`; when the last terminal job completes, PATCH back to `"0"`.

- [ ] **Step 1: Write the Knative minScale patch helper (option A)**

```go
// internal/csvjob/knative_minscale.go

// SetMinScale PATCHes the marketplace-api Knative Service's
// autoscaling.knative.dev/minScale annotation. Used to keep the pod alive
// while a csv import job is in flight — Knative reads queue-proxy concurrency
// for autoscaling, and an idle HTTP server scales to zero regardless of
// in-process goroutines.
//
// LANDMINE: the patch is not instant. There is a 3–10s window between
// 200 OK and the autoscaler acting. Do not rely on minScale being effective
// before the first worker heartbeat.
func SetMinScale(ctx context.Context, client dynamic.Interface, namespace, name string, value int) error {
    patch := []byte(fmt.Sprintf(
        `{"spec":{"template":{"metadata":{"annotations":{"autoscaling.knative.dev/minScale":"%d"}}}}}`,
        value,
    ))
    gvr := schema.GroupVersionResource{
        Group: "serving.knative.dev", Version: "v1", Resource: "services",
    }
    _, err := client.Resource(gvr).
        Namespace(namespace).
        Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
    return err
}
```

- [ ] **Step 2: Wire startup recovery scan into `main.go`**

```go
// cmd/marketplace-api/main.go (excerpt)

// Recovery scan: reclaim orphaned jobs from crashed pods. SKIP LOCKED
// guarantees that N concurrently-starting pods never fight over the same row.
if err := csvjobSvc.RecoverOrphans(ctx, 60*time.Second); err != nil {
    log.WithError(err).Warn("csvjob recovery scan failed (non-fatal)")
}

// Start the worker loop (background goroutine).
go csvjobSvc.RunWorkerLoop(ctx)

// Shutdown drain.
srv := &http.Server{Handler: router, Addr: ":" + port}
go func() {
    <-ctx.Done()
    drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := csvjobSvc.Drain(drainCtx); err != nil {
        log.WithError(err).Error("csvjob drain error")
    }
    _ = srv.Shutdown(drainCtx)
}()
```

- [ ] **Step 3: Document the RBAC requirement**

Reference `rbac-minscale.yaml` from the marketplace-api Kustomization and add a README note under `services/marketplace-api/internal/csvjob/README.md`:

> The marketplace-api ServiceAccount requires `patch` permission on its own Knative Service resource for the CSV worker's `minScale` flow (option A). Without this RBAC, the worker logs a 403 and the pod may scale to zero mid-import.

- [ ] **Step 4: Run integration test against a fake dynamic client, commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go services/marketplace-api/internal/csvjob/knative_minscale.go services/marketplace-api/internal/csvjob/knative_minscale_test.go services/marketplace-api/internal/csvjob/README.md
git commit -m "feat(marketplace-api): wire csvjob worker loop + startup recovery + Knative minScale patch (M7e)"
```

---

### Task 8: HTTP handlers — submit, list, status, cancel, download error CSV (S)

**Files:**
- Create: `services/marketplace-api/internal/csvjob/handlers.go`
- Create: `services/marketplace-api/internal/csvjob/handlers_test.go`

**Scope:** Five endpoints, all FGA-gated. Submit reads multipart file, calls `service.Submit`. List paginates by `(store_id, status, created_at desc)`. Status is a single read. Cancel flips status. Errors.csv streams the GCS object back to the client with `Content-Type: text/csv`.

- [ ] **Step 1: Route table**

```go
// handlers.go
func RegisterRoutes(r *gin.RouterGroup, h *Handlers, fga authz.Middleware) {
    g := r.Group("/stores/:storeId/products/csv-imports")
    g.POST("",                fga.Require(authz.MPCanImportProducts), h.Submit)
    g.GET("",                 fga.Require(authz.MPCanViewProducts),   h.List)
    g.GET("/:jobId",          fga.Require(authz.MPCanViewProducts),   h.Status)
    g.POST("/:jobId/cancel",  fga.Require(authz.MPCanImportProducts), h.Cancel)
    g.GET("/:jobId/errors.csv", fga.Require(authz.MPCanViewProducts), h.DownloadErrors)
}
```

- [ ] **Step 2: Tests** — `TestSubmit_HappyPath`, `TestSubmit_Over50k_Rejected`, `TestSubmit_DedupedReturnsExistingJobId`, `TestList_Paginated`, `TestStatus_NotFound`, `TestCancel_FlipsStatus`, `TestDownloadErrors_StreamsFromGCS`.

- [ ] **Step 3: Implement, PASS, commit**

```bash
git add services/marketplace-api/internal/csvjob/handlers.go services/marketplace-api/internal/csvjob/handlers_test.go
git commit -m "feat(marketplace-api): csvjob HTTP handlers submit/list/status/cancel/errors (M7e)"
```

---

### Task 9: Streaming CSV export handler (S)

**Files:**
- Create: `services/marketplace-api/internal/product/export_handler.go`
- Create: `services/marketplace-api/internal/product/export_handler_test.go`

**Scope:** `GET /api/v1/admin/stores/:storeId/products/export.csv?ids=…&filters=…`. Streams CSV directly to `http.ResponseWriter` with periodic `Flush()` every 100 rows. Reuses the existing product repository's filter/listing path. Columns per spec §4.1: `id, handle, title, status, vendor, category, base_price, compare_at_price, sku, stock_total, variant_count, created_at`.

- [ ] **Step 1: Failing test**

```go
func TestExport_StreamsAllColumns(t *testing.T) {
    // seed 3 products, call handler, parse response body, assert headers + rows
}

func TestExport_FlushesPeriodically(t *testing.T) {
    // wrap ResponseWriter, assert Flush() called at least once for 250-row export
}
```

- [ ] **Step 2: Implement**

```go
// export_handler.go

// LANDMINE: http.ResponseWriter buffers by default. Call Flush() after every
// 100 rows so large exports don't accumulate the entire CSV in memory before
// the client sees any bytes.
func (h *Handler) ExportCSV(c *gin.Context) {
    storeID := c.Param("storeId")
    filters := parseFilters(c)

    c.Header("Content-Type", "text/csv; charset=utf-8")
    c.Header("Content-Disposition", fmt.Sprintf(
        `attachment; filename="products-%s-%s.csv"`,
        storeID, time.Now().UTC().Format("20060102-150405"),
    ))

    w := csv.NewWriter(c.Writer)
    _ = w.Write(exportColumns)
    w.Flush()

    flusher, _ := c.Writer.(http.Flusher)
    n := 0
    err := h.repo.Stream(c.Request.Context(), storeID, filters, func(p product.Product) error {
        if err := w.Write(productToRow(p)); err != nil {
            return err
        }
        n++
        if n%100 == 0 {
            w.Flush()
            if flusher != nil {
                flusher.Flush()
            }
        }
        return nil
    })
    w.Flush()
    if err != nil {
        log.WithError(err).Error("export stream error")
    }
}
```

- [ ] **Step 3: PASS, commit**

```bash
git add services/marketplace-api/internal/product/export_handler.go services/marketplace-api/internal/product/export_handler_test.go
git commit -m "feat(marketplace-api): streaming CSV export handler (M7e)"
```

---

### Task 10: FGA gates verified on every new endpoint (S)

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Modify: `services/marketplace-api/internal/csvjob/handlers.go` (if not already done in Task 8)

**Scope:** Route-level assertion that every new endpoint is wrapped with `fgaMw.Require(...)`. Add a dedicated table-driven test that hits every route without a valid FGA token and asserts 403. This is the gate against accidental unauthenticated surfaces.

- [ ] **Step 1: Test**

```go
func TestAllCsvRoutes_AreFGAGated(t *testing.T) {
    routes := []struct{ method, path string }{
        {"POST", "/api/v1/admin/stores/s1/products/csv-imports"},
        {"GET",  "/api/v1/admin/stores/s1/products/csv-imports"},
        {"GET",  "/api/v1/admin/stores/s1/products/csv-imports/j1"},
        {"POST", "/api/v1/admin/stores/s1/products/csv-imports/j1/cancel"},
        {"GET",  "/api/v1/admin/stores/s1/products/csv-imports/j1/errors.csv"},
        {"GET",  "/api/v1/admin/stores/s1/products/export.csv"},
    }
    for _, r := range routes {
        t.Run(r.method+" "+r.path, func(t *testing.T) {
            req := httptest.NewRequest(r.method, r.path, nil)
            rec := httptest.NewRecorder()
            router.ServeHTTP(rec, req)
            require.Equal(t, http.StatusForbidden, rec.Code)
        })
    }
}
```

- [ ] **Step 2: PASS, commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go services/marketplace-api/internal/csvjob/handlers.go
git commit -m "test(marketplace-api): assert FGA gating on every new csv endpoint (M7e)"
```

---

### Task 11: Frontend typed client `lib/api/csvImports.ts` (U)

**Files:**
- Create: `apps/admin/lib/api/csvImports.ts`
- Create: `apps/admin/lib/api/csvImports.test.ts`

**Scope:** Typed API client for all six backend endpoints (5 csv-imports + 1 export). Export is exposed as two URL-builder helpers (selected / all) because the browser triggers the download via an anchor `href`, not fetch. Submit wraps `FormData` upload. List/Status/Cancel use JSON. All responses are typed with zod schemas to catch contract drift early.

- [ ] **Step 1: Define the types**

```typescript
// apps/admin/lib/api/csvImports.ts
import { z } from "zod";

export const csvJobStatusSchema = z.enum([
  "queued", "running", "paused", "completed", "failed", "cancelled",
]);
export type CsvJobStatus = z.infer<typeof csvJobStatusSchema>;

export const csvJobSchema = z.object({
  id: z.string().uuid(),
  store_id: z.string().uuid(),
  user_id: z.string(),
  status: csvJobStatusSchema,
  total_rows: z.number().nullable(),
  last_processed_row: z.number(),
  success_count: z.number(),
  error_count: z.number(),
  error_csv_gcs_path: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type CsvJob = z.infer<typeof csvJobSchema>;

export interface SubmitResult { job: CsvJob; deduped: boolean; }

export async function submitCsvImport(storeId: string, file: File): Promise<SubmitResult> {
  const body = new FormData();
  body.set("file", file);
  const res = await fetch(
    `/api/v1/admin/stores/${storeId}/products/csv-imports`,
    { method: "POST", body },
  );
  if (!res.ok) throw new Error(await res.text());
  const json = await res.json();
  return { job: csvJobSchema.parse(json.job), deduped: Boolean(json.deduped) };
}

export function buildExportAllUrl(storeId: string, filters?: URLSearchParams): string {
  const qs = filters?.toString();
  return `/api/v1/admin/stores/${storeId}/products/export.csv${qs ? `?${qs}` : ""}`;
}

export function buildExportSelectedUrl(storeId: string, ids: string[]): string {
  const qs = new URLSearchParams();
  qs.set("ids", ids.join(","));
  return `/api/v1/admin/stores/${storeId}/products/export.csv?${qs.toString()}`;
}

// ...listCsvImports, getCsvImport, cancelCsvImport, buildErrorCsvUrl
```

- [ ] **Step 2: Tests with mocked fetch, PASS, commit**

```bash
git add apps/admin/lib/api/csvImports.ts apps/admin/lib/api/csvImports.test.ts
git commit -m "feat(admin): csvImports typed API client with zod schemas (M7e)"
```

---

### Task 12: `app/products/import/page.tsx` — upload + preview + column mapping (C)

**Files:**
- Create: `apps/admin/app/products/import/page.tsx`
- Create: `apps/admin/components/products/csv/CsvUploadDropzone.tsx`
- Create: `apps/admin/components/products/csv/CsvPreviewTable.tsx`
- Create: `apps/admin/components/products/csv/CsvColumnMapping.tsx`
- Modify: `apps/admin/app/products/actions.ts`

**Scope:** A client component page under `app/products/import/page.tsx` (single entry point — both upload and history land here). Imports `papaparse` **only** inside the client-component subtree so the list page bundle stays lean. Flow: dropzone → client-side parse of first 5 rows via papaparse → preview table → column mapping UI (CSV columns → product fields) → submit via server action → redirect to `/products/import/[jobId]`.

- [ ] **Step 1: Scaffold `CsvUploadDropzone.tsx`**

```tsx
// apps/admin/components/products/csv/CsvUploadDropzone.tsx
"use client";
import { useCallback, useState } from "react";
import { cn } from "@/lib/utils";

interface Props {
  onFileSelected: (file: File) => void;
  disabled?: boolean;
}

export function CsvUploadDropzone({ onFileSelected, disabled }: Props) {
  const [dragOver, setDragOver] = useState(false);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file && file.type === "text/csv") onFileSelected(file);
  }, [onFileSelected]);

  return (
    <div
      onDrop={onDrop}
      onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      className={cn(
        "border border-dashed border-[var(--ink-300)] rounded-md px-8 py-16",
        "bg-[var(--paper-200)] text-[var(--ink-700)]",
        "transition-colors",
        dragOver && "border-[var(--moss-700)] bg-[var(--paper-300)]",
        disabled && "opacity-50 pointer-events-none",
      )}
    >
      <p className="font-serif text-2xl text-[var(--ink-900)]">Drop CSV here</p>
      <p className="mt-2 text-sm">or click to browse. Maximum 50,000 rows, UTF-8 only.</p>
      <input
        type="file"
        accept=".csv,text/csv"
        className="sr-only"
        onChange={(e) => e.target.files?.[0] && onFileSelected(e.target.files[0])}
      />
    </div>
  );
}
```

- [ ] **Step 2: Scaffold `CsvPreviewTable.tsx`** — uses `papaparse` (dynamic import inside the component) to parse the first 5 rows only.

```tsx
// apps/admin/components/products/csv/CsvPreviewTable.tsx
"use client";
import { useEffect, useState } from "react";

interface Props { file: File; }

export function CsvPreviewTable({ file }: Props) {
  const [header, setHeader] = useState<string[]>([]);
  const [rows, setRows] = useState<string[][]>([]);

  useEffect(() => {
    // Route-scoped dynamic import — keeps papaparse out of the list page bundle.
    import("papaparse").then(({ default: Papa }) => {
      Papa.parse<string[]>(file, {
        preview: 6, // header + 5 data rows
        complete: (res) => {
          setHeader(res.data[0] ?? []);
          setRows(res.data.slice(1));
        },
      });
    });
  }, [file]);

  if (header.length === 0) return <p className="text-sm">Parsing preview…</p>;

  return (
    <table className="w-full text-sm">
      <thead className="border-b border-[var(--ink-200)]">
        <tr>
          {header.map((h) => (
            <th key={h} className="py-2 text-left font-serif text-[var(--ink-900)]">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => (
          <tr key={i} className="border-b border-[var(--ink-100)]">
            {row.map((cell, j) => (
              <td key={j} className="py-2 text-[var(--ink-700)]">{cell}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

- [ ] **Step 3: Scaffold `CsvColumnMapping.tsx`** — maps CSV columns to product fields (`handle`, `title`, `status`, `base_price`, `sku`, etc.). Uses `@tesserix/web` Select primitives.

- [ ] **Step 4: Wire `page.tsx`** — orchestrates the three steps, calls `submitCsvImportAction` server action on confirm, `redirect(`/products/import/${job.id}`)` on success.

- [ ] **Step 5: Tests with RTL + mock papaparse, commit**

```bash
git add apps/admin/app/products/import/page.tsx apps/admin/components/products/csv/CsvUploadDropzone.tsx apps/admin/components/products/csv/CsvPreviewTable.tsx apps/admin/components/products/csv/CsvColumnMapping.tsx apps/admin/app/products/actions.ts
git commit -m "feat(admin): csv import upload + preview + column mapping page (M7e)"
```

---

### Task 13: `app/products/import/[jobId]/page.tsx` — live status polling (C)

**Files:**
- Create: `apps/admin/app/products/import/[jobId]/page.tsx`
- Create: `apps/admin/components/products/csv/CsvJobProgress.tsx`

**Scope:** Server component that reads the job status server-side, renders `CsvJobProgress`, and triggers a client-side interval that calls a server action + `router.refresh()` every 2 seconds until status is terminal.

- [ ] **Step 1: Scaffold `CsvJobProgress.tsx`**

```tsx
// apps/admin/components/products/csv/CsvJobProgress.tsx
"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import type { CsvJob } from "@/lib/api/csvImports";

const TERMINAL = new Set(["completed", "failed", "cancelled"]);

interface Props { job: CsvJob; }

export function CsvJobProgress({ job }: Props) {
  const router = useRouter();

  useEffect(() => {
    if (TERMINAL.has(job.status)) return;
    const id = setInterval(() => router.refresh(), 2000);
    return () => clearInterval(id);
  }, [job.status, router]);

  const pct = job.total_rows
    ? Math.round((job.last_processed_row / job.total_rows) * 100)
    : 0;

  return (
    <section className="border-t border-[var(--ink-200)] pt-6">
      <h2 className="font-serif text-3xl text-[var(--ink-900)]">
        Importing {job.total_rows ?? "?"} rows
      </h2>
      <div className="mt-4 h-1 w-full bg-[var(--paper-300)]">
        <div
          className="h-full bg-[var(--moss-700)] transition-[width]"
          style={{ width: `${pct}%` }}
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
        />
      </div>
      <dl className="mt-6 grid grid-cols-3 gap-6 text-sm">
        <div>
          <dt className="text-[var(--ink-600)]">Processed</dt>
          <dd className="font-serif text-2xl">{job.last_processed_row}</dd>
        </div>
        <div>
          <dt className="text-[var(--ink-600)]">Imported</dt>
          <dd className="font-serif text-2xl text-[var(--moss-700)]">{job.success_count}</dd>
        </div>
        <div>
          <dt className="text-[var(--ink-600)]">Errors</dt>
          <dd className="font-serif text-2xl text-[var(--danger)]">{job.error_count}</dd>
        </div>
      </dl>
    </section>
  );
}
```

- [ ] **Step 2: Wire `page.tsx`** as a server component that fetches the job status via the typed client and renders `CsvJobProgress` + `CsvErrorSummary` (from Task 15).

- [ ] **Step 3: Commit**

```bash
git add apps/admin/app/products/import/[jobId]/page.tsx apps/admin/components/products/csv/CsvJobProgress.tsx
git commit -m "feat(admin): csv import status page with 2s polling via router.refresh (M7e)"
```

---

### Task 14: `CsvImportHistory.tsx` history table (C)

**Files:**
- Create: `apps/admin/components/products/csv/CsvImportHistory.tsx`
- Modify: `apps/admin/app/products/import/page.tsx` (inject history under the upload area)

**Scope:** A paginated list of past imports, Paper surface with hairline rules, Source Serif 4 for numerals (row counts), Source Sans 3 for labels. Each row links to `/products/import/[jobId]`. Statuses rendered with muted type — never pill badges (editorial, not dashboard).

- [ ] **Step 1: Scaffold**

```tsx
// apps/admin/components/products/csv/CsvImportHistory.tsx
import Link from "next/link";
import type { CsvJob } from "@/lib/api/csvImports";

interface Props { jobs: CsvJob[]; storeId: string; }

export function CsvImportHistory({ jobs, storeId }: Props) {
  if (jobs.length === 0) {
    return (
      <p className="mt-12 text-sm text-[var(--ink-600)]">
        No imports yet. Drop a CSV above to start.
      </p>
    );
  }
  return (
    <section className="mt-16">
      <h2 className="font-serif text-2xl text-[var(--ink-900)]">Recent imports</h2>
      <ul className="mt-6 divide-y divide-[var(--ink-100)]">
        {jobs.map((job) => (
          <li key={job.id} className="py-4">
            <Link
              href={`/products/import/${job.id}`}
              className="flex items-baseline justify-between gap-6 hover:text-[var(--moss-700)]"
            >
              <span className="font-serif text-lg">
                {new Date(job.created_at).toLocaleString()}
              </span>
              <span className="text-sm text-[var(--ink-600)]">
                {job.success_count} imported · {job.error_count} errors · {job.status}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/admin/components/products/csv/CsvImportHistory.tsx apps/admin/app/products/import/page.tsx
git commit -m "feat(admin): csv import history list on /products/import (M7e)"
```

---

### Task 15: `CsvErrorSummary.tsx` — download error CSV link (C)

**Files:**
- Create: `apps/admin/components/products/csv/CsvErrorSummary.tsx`
- Modify: `apps/admin/app/products/import/[jobId]/page.tsx` (embed below progress)

**Scope:** A single editorial block rendered when `error_count > 0` and the job is terminal. Provides a direct download link to `GET /csv-imports/:jobId/errors.csv`. No modal, no accordion — straight anchor with Moss hover.

- [ ] **Step 1: Scaffold**

```tsx
// apps/admin/components/products/csv/CsvErrorSummary.tsx
import type { CsvJob } from "@/lib/api/csvImports";
import { buildErrorCsvUrl } from "@/lib/api/csvImports";

interface Props { job: CsvJob; storeId: string; }

export function CsvErrorSummary({ job, storeId }: Props) {
  if (job.error_count === 0) return null;
  return (
    <section className="mt-12 border-t border-[var(--ink-200)] pt-6">
      <h3 className="font-serif text-xl text-[var(--ink-900)]">
        {job.error_count} rows could not be imported
      </h3>
      <p className="mt-2 text-sm text-[var(--ink-700)]">
        Each error row includes its original row number and the reason it was rejected.
        Fix and re-upload the corrected rows — successfully imported rows were not rolled back.
      </p>
      <a
        href={buildErrorCsvUrl(storeId, job.id)}
        className="mt-4 inline-block text-[var(--moss-700)] underline-offset-4 hover:underline"
      >
        Download error CSV
      </a>
    </section>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/admin/components/products/csv/CsvErrorSummary.tsx apps/admin/app/products/import/[jobId]/page.tsx
git commit -m "feat(admin): csv error summary with download link (M7e)"
```

---

### Task 16: Playwright E2E — import flow + export flow (E)

**Files:**
- Create: `apps/admin/tests/e2e/products-csv-import.spec.ts`
- Create: `apps/admin/tests/e2e/products-csv-export.spec.ts`

**Scope:** Two Playwright specs. Import: upload a 10-row CSV with 2 intentionally bad rows → assert the `[jobId]` page polls and shows progress → wait for terminal status → navigate to list → assert 8 new products → download error CSV → assert 2 rows with correct row numbers. Export: select 5 products on the list page → click "Export selected" in `BulkActionsBar` → assert downloaded file content matches expected columns and row count.

- [ ] **Step 1: Build fixture CSVs under `apps/admin/tests/e2e/fixtures/`**

- [ ] **Step 2: Write `products-csv-import.spec.ts`**

```typescript
import { test, expect } from "@playwright/test";
import path from "node:path";

test("import: 10-row CSV with 2 bad rows → 8 imported, 2 in error CSV", async ({ page }) => {
  await page.goto("/products/import");
  await page.locator('input[type="file"]').setInputFiles(
    path.join(__dirname, "fixtures", "import-10-rows-2-bad.csv"),
  );
  await page.getByRole("button", { name: /import/i }).click();

  // Poll until terminal.
  await expect(page.getByRole("progressbar")).toBeVisible();
  await expect(page.getByText(/completed/i)).toBeVisible({ timeout: 60_000 });

  await expect(page.getByText(/8 imported/i)).toBeVisible();
  await expect(page.getByText(/2 errors/i)).toBeVisible();

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("link", { name: /download error csv/i }).click(),
  ]);
  const content = await download.createReadStream().then(
    async (s) => { if (!s) return ""; let t = ""; for await (const c of s) t += c; return t; },
  );
  expect(content).toMatch(/row_number,raw_line,error_message/);
  expect(content.split("\n").filter((l) => l.trim()).length).toBe(3); // header + 2
});
```

- [ ] **Step 3: Write `products-csv-export.spec.ts`**

- [ ] **Step 4: Run both, commit**

```bash
git add apps/admin/tests/e2e/products-csv-import.spec.ts apps/admin/tests/e2e/products-csv-export.spec.ts apps/admin/tests/e2e/fixtures/
git commit -m "test(admin): e2e csv import + export flows (M7e)"
```

---

### Task 17: Impeccable chain pass + verification + PR

**Files:** no new code — quality gate + PR.

**Scope:** Run the full `frontend-design` → `critique` → `polish` → `arrange` → `typeset` → `audit` → `adapt` chain on the three new UI surfaces (`/products/import`, `/products/import/[jobId]`, the export trigger in `BulkActionsBar`). `critique` score must be ≥ 7.5/10 on each surface. Fix any CRITICAL/HIGH findings before closing the task.

- [ ] **Step 1: Run the chain on `/products/import`**

```bash
# (skill invocation documented in mark8ly/.impeccable.md)
```

- [ ] **Step 2: Record the critique score and the polish diff**

- [ ] **Step 3: Run the final verification checklist** (all items from spec §8):
  - `go test ./... -race -cover` — 80%+ on new csvjob files
  - `npm run test -- csv` — 80%+ on new frontend files
  - Both Playwright E2E specs green
  - No new `go vet` warnings
  - No new ESLint errors
  - Paper · Ink · Moss tokens only — `grep -r "#[0-9a-fA-F]\{6\}" apps/admin/components/products/csv/` empty
  - WCAG 2.1 AA: keyboard nav through the full import flow, visible moss focus ring, `prefers-reduced-motion` honored on progress bar
  - Skip link still first focusable on list page
  - No new dialogs (per §13.5)
  - `mark8ly/.impeccable.md` exists and current
  - No secrets committed

- [ ] **Step 4: Delete `.planning/m7e-infra-gaps.md`**

```bash
git rm .planning/m7e-infra-gaps.md
git commit -m "chore(planning): remove m7e-infra-gaps.md, all rows closed"
```

- [ ] **Step 5: Open the PR**

Paste the filled exit matrix from Task 1 and the critique scores from Task 17 into the PR description. Reference the design spec §4 sections. Request review.

**Exit:** 10-row CSV imports with 2 partial failures; error CSV downloadable with correct row numbers; export streams and matches current filter; crash mid-import recovers cleanly with no duplicate rows; double-submit of same CSV deduped by content hash; Knative minScale patch verified against kind cluster or mocked dynamic client; impeccable chain passed; 80% coverage on all new Go and TypeScript files.

---

## Verification checklist (milestone-level)

- [ ] All new Go files 80%+ coverage (`go test ./internal/csvjob/... -cover`)
- [ ] All new TS files 80%+ coverage (Vitest)
- [ ] Both Playwright E2E specs green
- [ ] No new `go vet` warnings
- [ ] No new ESLint errors
- [ ] Paper · Ink · Moss tokens only — no new hex values
- [ ] WCAG 2.1 AA on `/products/import` and `/products/import/[jobId]`
- [ ] `prefers-reduced-motion` honored on progress bar
- [ ] **No new dialogs** (per §13.5)
- [ ] Impeccable chain passed: `critique` ≥ 7.5, `polish` applied, `audit` green, `adapt` verified across viewports
- [ ] `mark8ly/.impeccable.md` exists and is current
- [ ] `.planning/m7e-infra-gaps.md` deleted
- [ ] RBAC `Role` + `RoleBinding` for Knative `minScale` patch merged to `tesserix-infra`
- [ ] Crash-recovery integration test (`TestWorker_CrashRecovery_NoDuplicateRows`) green
- [ ] Content-hash idempotency test (`TestSubmit_DedupesIdenticalContent`) green
- [ ] 50k row cap enforced at both upload and processing — test for each
- [ ] Error CSV memory bound verified — 50k error rows run with bounded memory
