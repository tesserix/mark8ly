# Variant soft-delete leaks through every Preload (#395) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A merchant removes a variant; it is soft-deleted; and it keeps appearing — including on the storefront. Make GORM enforce the filter rather than relying on five call sites to remember it.

**Architecture:** `Variant.DeletedAt` becomes `gorm.DeletedAt`, so GORM's implicit `WHERE deleted_at IS NULL` applies to the model everywhere, including inside `Preload`. Queries that legitimately need to see deleted rows opt out with `Unscoped()`.

**Issue:** [#395](https://github.com/tesserix/mark8ly/issues/395)

---

## Findings that shape this plan

Verified against the code. Three of these change the issue's framing.

**1. The bug is LIVE, not latent.** `internal/product/repository_variants.go:95-97` soft-deletes variants in the sync's "Removes" path: `Update("deleted_at", gorm.Expr("now()"))`. So rows really do get `deleted_at` set whenever a merchant removes a variant from a product.

**2. Five `Preload("Variants")` sites, none filtering.** `internal/product/repository.go:209, 281, 351, 405, 447`. The `deleted_at IS NULL` at `:407` is the *product's*, in the outer `Where` — it does not constrain the preloaded variants. **Two of the five (`:405`, `:447`) are storefront paths** for published products, so this is customer-visible: a removed variant stays purchasable-looking.

**3. It affects `Product` too, but harmlessly.** `gorm.DeletedAt` appears **zero** times as code in `internal/product/models.go` — the one grep hit is a comment. `Product.DeletedAt` (`:75`) is also `*time.Time`. It does not leak, because every product query filters `deleted_at IS NULL` explicitly (`:212, 262, 326, 407, 450, 499`). That is correct today and out of scope — but see the note at the end.

**4. There is no hard-delete disaster.** Deletion is done via explicit `Update("deleted_at", …)` (`repository.go:514-519`, `repository_variants.go:95-97`), never `db.Delete()`. So the missing `gorm.DeletedAt` has not been silently hard-deleting rows. The consequence is only the leak.

**5. Nothing reads the field, and nothing puts it on the wire.** No Go code references `Variant.DeletedAt` as a value, and no frontend reads `deleted_at` from a variant payload. So the type change breaks no reader, and the JSON tag can be dropped rather than reshaped.

---

## Global Constraints

- **No migration.** The `deleted_at` column already exists and is indexed. This is a Go type change. Any DDL means the plan is wrong.
- **`gorm.DeletedAt` serializes as a struct**, so `json:"deleted_at,omitempty"` would start emitting `{"Time":…,"Valid":false}` on every variant. Use **`json:"-"`** — a deleted-at timestamp has no business on an API response, and finding #5 confirms nothing consumes it.
- **The dangerous direction is a query that NEEDS to see deleted rows and now silently cannot.** Once the model carries `gorm.DeletedAt`, GORM filters *every* query on `Variant`, not just Preloads. Any code that must see soft-deleted variants has to say `Unscoped()` explicitly. Auditing for those is the load-bearing part of this change — see Task 1.
- **Do not change `Product.DeletedAt`.** Its manual filters are correct and tested; changing both at once doubles the audit surface for no additional fix.
- Go: run from the service root, `cd services/marketplace-api && go test ./... -count=1`, never path-scoped. Plus `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`.
- Integration tests use `//go:build integration`, gate on `TEST_DATABASE_URL`, and run with `-p 1`. Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`.
- Conventional single-line commit messages, no signature, no `Co-Authored-By` trailer.
- **Use explicit paths when staging (`git add <path>`), never `git add -A`.**
- **Pre-existing failures — not yours to fix:** `internal/billing/trial/subscribe_integration_test.go` (19 tests, #317), `internal/subscription/planchange` integration (9 FAIL), `internal/whitelabel` integration (nil-pointer panic).

---

## Tasks

### Task 1 — Audit what must still see deleted variants

Do this BEFORE changing the type. The change is one line; the risk is entirely in what it silently filters out.

- [ ] Enumerate every query touching `product_variants` or the `Variant` model — GORM calls and raw SQL both. Start from `internal/product/repository_variants.go`, `internal/product/repository.go`, `internal/product/service_aggregate.go`, and grep the service for `product_variants`.
- [ ] For each, decide and record: does it intend to see soft-deleted rows?
  - The sync's **re-create / revive** path is the one to look hardest at. `service_aggregate.go:218` comments that "UPDATE on `product_variants` doesn't move it" — establish whether re-adding a previously-removed variant is expected to find and revive the soft-deleted row (which would need `Unscoped()`) or to insert a new one. If it looks up by SKU or option-value combination, a unique index may make an insert fail against the surviving soft-deleted row — **that would turn this fix into a new bug**, so settle it here.
  - `internal/subscription/harddelete/sweeper.go` deletes via FK cascade and raw SQL; confirm it is unaffected.
- [ ] Write the findings into the report as a table: query, intent, needs `Unscoped()` yes/no.

**Verify:** the table, with a line per call site. This task produces no code.

### Task 2 — Make GORM enforce it

- [ ] `internal/product/models.go:136` — `DeletedAt gorm.DeletedAt` with `gorm:"column:deleted_at;index"` and `json:"-"`.
- [ ] Replace the existing comment (which documents the bug) with one stating that GORM now filters automatically and that a query needing deleted rows must use `Unscoped()`.
- [ ] Add `Unscoped()` to exactly the queries Task 1 identified, each with a comment saying why it needs to see deleted rows.
- [ ] Let the compiler find readers. If anything fails to build against the new type, that is a site that was reading the timestamp — report it rather than coercing it.

**Verify:** `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.

### Task 3 — Prove it, at the level that was broken

The unit level cannot prove this: the filter is applied by GORM against a real database. This needs an integration test.

- [ ] Add an integration test (`//go:build integration`, `TEST_DATABASE_URL`-gated, cleaned up in a transaction or defer) that:
  - seeds a product with two variants, soft-deletes one via the real code path (`repository_variants.go`'s remove), and asserts each of the **five** `Preload("Variants")` methods returns only the surviving variant. Name the storefront ones explicitly — they are the customer-visible pair.
  - asserts the soft-deleted row **still exists** in the table (`Unscoped().Count()` or raw SQL), so the test distinguishes "filtered" from "destroyed". A test that only counts returned variants would pass if the fix accidentally hard-deleted.
  - covers whatever Task 1 found needs `Unscoped()` — that path must still see the deleted row.
- [ ] **Prove the test fails without the fix:** revert the type to `*time.Time`, run, capture the failure verbatim, restore.

**Verify:** `go vet -tags=integration ./...`, then `go test -tags=integration -p 1 -count=1 ./internal/product/...`, plus the full suite from the service root.

### Task 4 — Close out

- [ ] Comment on #395 with what shipped, and correct the record on two points: the issue named only `Variant`, and the leak is live rather than theoretical because variants really are soft-deleted on removal.
- [ ] Note that `Product.DeletedAt` has the same declaration but does not leak, because every product query filters explicitly — and that this is a standing trap: a sixth product query added without the predicate would leak silently, exactly as the variant Preloads did. Worth its own issue rather than a silent fix.

---

## Out of scope

- **`Product.DeletedAt`.** It works today via explicit filters. Changing it belongs in its own change with its own audit.
- **Cascading product deletion to variants.** `repository.go:514-515` says deliberately: *"Variants carry their own deleted_at for future slices; we do NOT cascade here."* That is a product decision, not a defect.
- **Any change to what deletion means.** This plan changes what queries *return*, never what they write.
