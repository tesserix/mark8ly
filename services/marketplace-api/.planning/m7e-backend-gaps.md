# M7e Backend Gaps Catalog

| # | Verification item | Status | Blocking |
|---|---|---|---|
| 1 | No csvjob/export handler conflicts | CLOSED | no |
| 2 | GCS bucket + signed-upload infra reusable for CSV uploads | CLOSED | no |
| 3 | Multipart upload size limit on Gin engine supports <=100 MB | CLOSED | no |
| 4 | Knative minScale RBAC for marketplace-api ServiceAccount | DEFERRED | no — infra task, not Go code |
| 5 | go-shared/authz middleware can gate new routes | CLOSED | no |
| 6 | Next available migration slot | CLOSED (000007) | no |
| 7 | Repository method for streaming CSV export | CLOSED | no |
| 8 | GCS resumable upload support in internal/media | CLOSED | no — ErrorCSVWriter interface in csvjob, GCS impl deferred to CI |

## Detail

### 1. No csvjob/export handler conflicts
**Status:** CLEAR
**Evidence:** `internal/csvjob/` directory does not exist. Grep for `csvjob`, `csv_job`, `CsvJob`, `ExportHandler` across `internal/` returned zero hits in handler or route files. The two grep matches (`admin/validation_test.go`, `admin/media.go`) reference the word "export" only in the context of Go symbol exports, not CSV export functionality. `internal/handlers/admin/routes.go` has no `/export` or `/import` routes.
**Gap:** None.

### 2. GCS bucket + signed-upload infra reusable for CSV uploads
**Status:** SUPPORTED
**Evidence:** `internal/media/gcs.go:49` — `NewGCSUploader(client, bucketName)` constructs a bucket handle. `cmd/marketplace-api/main.go:126-138` — real GCS client is created when `MARKETPLACE_GCS_BUCKET` is non-empty. `gcs.go:80-93` — `SignedUploadURL` generates V4 signed PUT URLs. The same bucket handle can serve a `csv-imports/:storeId/` prefix with no changes to the existing uploader wiring. The `*storage.Client` is already available in the admin init block (`main.go:128`).
**Gap:** The csvjob package will need its own writer (not the media uploader) for error CSV streaming, but the GCS client and bucket config are reusable as-is. New code will construct a separate `*storage.Writer` from the same `*storage.Client` — no conflict.

### 3. Multipart upload size limit on Gin engine supports <=100 MB
**Status:** MISSING
**Evidence:** Grep for `MaxMultipartMemory` across the entire `services/marketplace-api/` tree returned zero matches. `pkg/httpserver/server.go` constructs Gin engines via `gin.New()` without setting `r.MaxMultipartMemory`. Gin's default is 32 MB (`gin.defaultMultipartMemory = 32 << 20`), which is below the 100 MB requirement.
**Gap:** Must set `r.MaxMultipartMemory = 100 << 20` on the admin engine before CSV import routes are mounted. This is a one-line change in `pkg/httpserver/server.go` or `cmd/marketplace-api/main.go`.

### 4. Knative minScale RBAC for marketplace-api ServiceAccount
**Status:** MISSING
**Evidence:** `go.mod` has no `k8s.io/client-go` or `k8s.io/apimachinery` dependency. Grep for `k8s.io` and `client-go` in go.mod returned nothing. No `internal/csvjob/knative_minscale.go` exists. No k8s RBAC manifests were found in this repo. The plan (line 118) explicitly calls out verifying whether `k8s.io/client-go` transitive weight is acceptable.
**Gap:** Two sub-gaps:
  (a) Add `k8s.io/client-go` + `k8s.io/apimachinery` to go.mod (or fall back to option B keepalive HTTP if dependency weight is unacceptable).
  (b) Emit Role + RoleBinding YAML granting marketplace-api's ServiceAccount `patch` on its own Knative `services.serving.knative.dev` resource. This is an infra task (k8s manifests), not a Go code task.

### 5. go-shared/authz middleware can gate new routes
**Status:** SUPPORTED
**Evidence:** `internal/authz/middleware.go:34` — `RequireTenantRelation(role Role) gin.HandlerFunc` is the gating mechanism. `internal/handlers/admin/routes.go` uses it on every route group (products, categories, orders, returns, abandoned-carts). `internal/authz/orders_roles.go` demonstrates the pattern: define role constants in a domain-specific file, reference them at route registration. New CSV import/export routes can follow the identical pattern by adding constants like `CsvExportRole = RoleStaff` and `CsvImportRole = RoleAdmin` in a new `csv_roles.go`.
**Gap:** None — pattern is established and extensible.

### 6. Next available migration slot
**Status:** 000007
**Evidence:** `migrations/` contains `000001` through `000006`. Highest is `000006_store_watermarks_orders.{up,down}.sql`. The plan confirms `000007_csv_import_jobs.up.sql` as the target filename.
**Gap:** None.

### 7. Repository method for streaming CSV export
**Status:** MISSING
**Evidence:** `internal/product/repository.go:41-57` — the `Repository` interface has `ListAdmin` (paginated, returns `[]Aggregate, int64, error`) and `ListPublished` (also paginated). Neither supports cursor-based iteration or streaming. `ListAdmin` loads all preloaded associations (options, values, variants, media) per page — suitable for UI pagination but not for streaming 50k rows to a CSV writer without excessive memory.
**Gap:** Need a new method like `StreamForExport(ctx, storeID, tenantID string, fn func(Aggregate) error) error` that uses GORM's `.Rows()` cursor or batched `FindInBatches` to iterate without loading the full result set. Alternatively, a simpler `ListAllForExport` that returns a minimal projection (no media/options preloads) with internal batching. The export handler will call this and write each row to the `http.ResponseWriter` with periodic `Flush()`.

### 8. GCS resumable upload support in internal/media
**Status:** MISSING
**Evidence:** `internal/media/gcs.go` implements `Verify`, `SignedUploadURL`, and `SignedReadURL` only — all are metadata/URL operations. `internal/media/uploader.go` defines the `Uploader` interface with only `Verify`. Neither file contains `NewWriter`, `ObjectWriter`, `Resumable`, or any write-path code. The error CSV streaming pattern (plan landmine #4) requires opening a GCS resumable upload session at job start and writing rows incrementally.
**Gap:** The csvjob package needs a new GCS writer abstraction — likely a thin wrapper around `bucket.Object(key).NewWriter(ctx)` with `ChunkSize` set for resumable upload behavior. This does NOT belong in `internal/media/` (which is product-media-specific). It should live in `internal/csvjob/` as a purpose-built error CSV writer.

## Recommended sub-tasks

Ordered execution plan for M7e Tasks 2-10:

1. **Task 2: Migration** — Create `000007_csv_import_jobs.{up,down}.sql` with the csv_import_jobs table, content-hash partial unique index, and status enum.
2. **Task 3: csvjob models** — `internal/csvjob/models.go` with entity, status constants, and DTO types.
3. **Task 4: csvjob repository** — `internal/csvjob/repository.go` with CRUD, `FindOrphanedForUpdate`, `UpsertByContentHash`. Tests against real Postgres.
4. **Task 5: Multipart limit fix** — Set `MaxMultipartMemory = 100 << 20` on the admin Gin engine. One-line change.
5. **Task 6: CSV parser** — `internal/csvjob/parser.go` with UTF-8 validation, header mapping, row-to-ProductDraft conversion, 50k cap enforcement.
6. **Task 7: Streaming export repository method** — Add `StreamForExport` or batched export method to `internal/product/repository.go`. Plus `internal/product/export_handler.go` with streaming CSV response + `Flush()`.
7. **Task 8: GCS error CSV writer** — `internal/csvjob/error_writer.go` wrapping `storage.Writer` for resumable upload of error rows.
8. **Task 9: Worker** — `internal/csvjob/worker.go` with goroutine lifecycle, advisory lock, heartbeat, checkpoint, cancellation, shutdown drain.
9. **Task 10: Knative minScale** — `internal/csvjob/knative_minscale.go` + Role/RoleBinding YAML. Evaluate `k8s.io/client-go` dependency weight; fall back to option B (keepalive HTTP) if unacceptable.
10. **Task 10b: Wiring** — Wire csvjob service, handlers, and routes into `cmd/marketplace-api/main.go` and `internal/handlers/admin/routes.go`. Add CSV role constants. Add startup recovery scan and shutdown drain hooks.
