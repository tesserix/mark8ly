# Task 3 — Prove it, at the level that was broken

Service: `services/marketplace-api`. Branch `fix/395-variant-soft-delete`, built on Task 2's
commit `9b52bf44` (`Variant.DeletedAt` → `gorm.DeletedAt`).

## What was added

New file: `internal/product/repository_variant_softdelete_integration_test.go`
(`//go:build integration`, gated on `TEST_DATABASE_URL` via `testdb.NewTx`, which runs the test
inside a transaction that is automatically rolled back on cleanup — no manual row cleanup needed,
and reruns are idempotent by construction).

`TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants`:

1. Seeds a product (status=active, published in the past, so it's storefront-visible) with two
   variants: a survivor and one to be removed.
2. Soft-deletes the second variant through the **real production code path** —
   `repo.ApplyVariantDiffInTx(..., VariantDiff{Removes: [removedID]})`, i.e.
   `repository_variants.go`'s Removes branch — not a hand-written `UPDATE`.
3. Asserts each of the five `Preload("Variants")` sites named in the plan returns only the
   survivor, as subtests naming each site and, for the two storefront ones, saying so explicitly:
   - `GetByIDForStore (admin detail)` — `repository.go:209`
   - `ListAdmin (admin list)` — `repository.go:281`
   - `ListPublished (storefront — customer-visible)` — `repository.go:351`
   - `ListPublishedBySlugs (storefront — customer-visible)` — `repository.go:405`
   - `GetPublishedByHandle (storefront — customer-visible, PDP)` — `repository.go:447`
4. Asserts the soft-deleted row **still exists** via `tx.Unscoped().Where("id = ?", removedID).First(...)`
   and that `DeletedAt.Valid` is true — the assertion that distinguishes "filtered" from
   "destroyed." A fix that hard-deleted the row instead of filtering it would make every assertion
   in step 3 pass too; this is the one that would catch that.
5. Re-add path: calls `ApplyVariantDiffInTx` again with `Adds` containing a **new** variant using
   the **same SKU** as the soft-deleted one. Asserts it succeeds (not `SKUTaken`), that there are
   now 2 live variants, that the new row has a new UUID (not `removedID`), and via
   `tx.Unscoped().Model(&Variant{}).Where("store_id = ? AND sku = ?", ...).Count()` that both rows
   (one soft-deleted, one live) coexist under the same SKU — proving
   `variants_sku_per_store_live_unique`'s partial index (`WHERE deleted_at IS NULL`) does exactly
   what Task 1's audit predicted.

Result on the current code (with the fix): **all subtests pass.**

## What I did with `repository_integration_test.go:577`

The brief was correct: this test tolerated the bug. It iterated `got.Variants` and `continue`d
past any row with `DeletedAt.Valid`, then asserted `live == 2` — so it would pass whether or not
GORM filtered soft-deleted variants out of the preload, because it filtered them itself in Go
first.

Rewrote `TestIntegration_ProductRepo_ApplyVariantDiffInTx_AddUpdateRemove`
(`internal/product/repository_integration_test.go`) to assert `len(got.Variants) == 2` directly
(no skip-and-count), and to `t.Fatalf` if the soft-deleted `v2ID` shows up in the preload at all.
Deleted the false "so sanity-check by iterating and ignoring deleted" comment and replaced it with
one stating the actual invariant: GORM's implicit filter applies to `Variant`, including inside
Preload. I did not find any reason the skip-and-count pattern was load-bearing for something else
— nothing downstream of this test cared about the exact count of *all* variants including deleted
ones, and the whole point of `#395` is that soft-deleted variants shouldn't be countable/visible
here at all.

## Re-add / partial-index result

**Succeeds, as predicted by Task 1's audit — no defect found.** Re-adding a variant with the same
SKU as a soft-deleted row inserts a new row with a new UUID; `variants_sku_per_store_live_unique`
being a partial unique index (`WHERE deleted_at IS NULL`) means the new INSERT never sees the
surviving soft-deleted row as a conflict. Confirmed both via the `ApplyVariantDiffInTx` call
returning no error and via a raw `Unscoped()` count showing 2 rows share the SKU afterward (one
`deleted_at IS NOT NULL`, one live). Not BLOCKED.

## Proof the test fails without the fix

Reverted, ran, captured failure, then fully restored — in that order:

1. Backed up `internal/product/models.go`, `internal/product/service_aggregate.go`, and the new
   test file to `/tmp/*.bak`.
2. Edited `models.go`: `Variant.DeletedAt` back to `*time.Time` (marked
   `// TEMP REVERT FOR TASK 3 PROOF — DO NOT COMMIT.`), removed the now-unused `gorm.io/gorm`
   import (only used for `gorm.DeletedAt`).
3. Edited `service_aggregate.go`: reverted both `v.DeletedAt.Valid` call sites (lines 230, 498)
   back to `v.DeletedAt != nil` via `sed`.
4. Edited the new test's `stillThere.DeletedAt.Valid` check to `stillThere.DeletedAt == nil` (this
   one assertion in the "row still exists" subtest has to compile against either type; everything
   else in the test only compares `.ID`, so it isn't type-dependent).
5. `go build ./...` → exit 0 (confirms the revert alone compiles).
6. `go vet -tags=integration ./...` → exit 0.
7. Ran both the rewritten legacy test and the new test under
   `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable
   go test -tags=integration -p 1 -count=1 -run 'TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants|TestIntegration_ProductRepo_ApplyVariantDiffInTx_AddUpdateRemove' -v ./internal/product/...`

**Verbatim failure output (reverted state):**

```
=== RUN   TestIntegration_ProductRepo_ApplyVariantDiffInTx_AddUpdateRemove
    repository_integration_test.go:575: variants = 3, want 2: [{...SKU:D-3...DeletedAt:<nil>...} {...SKU:D-1...DeletedAt:<nil>...} {...SKU:D-2...DeletedAt:2026-08-28 18:32:29.769109 +1000 AEST...}]
--- FAIL: TestIntegration_ProductRepo_ApplyVariantDiffInTx_AddUpdateRemove (0.04s)
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/GetByIDForStore_(admin_detail)
    repository_variant_softdelete_integration_test.go:91: GetByIDForStore: variants = 2, want 1: [...LEAK-SURVIVOR...DeletedAt:<nil>...] [...LEAK-REMOVED...DeletedAt:2026-08-28 18:32:29.800256 +1000 AEST...]
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListAdmin_(admin_list)
    repository_variant_softdelete_integration_test.go:110: ListAdmin: variants = 2, want 1: [...same leak...]
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListPublished_(storefront_—_customer-visible)
    repository_variant_softdelete_integration_test.go:132: ListPublished: variants = 2, want 1: [...same leak...]
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListPublishedBySlugs_(storefront_—_customer-visible)
    repository_variant_softdelete_integration_test.go:143: ListPublishedBySlugs: variants = 2, want 1: [...same leak...]
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/GetPublishedByHandle_(storefront_—_customer-visible,_PDP)
    repository_variant_softdelete_integration_test.go:151: GetPublishedByHandle: variants = 2, want 1: [...same leak...]
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/soft-deleted_row_still_exists_(not_destroyed)
=== RUN   TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/re-add_with_same_SKU_as_soft-deleted_variant_succeeds_(partial_index)
    repository_variant_softdelete_integration_test.go:193: live variants after re-add = 3, want 2: [...LEAK-SURVIVOR... LEAK-REMOVED(soft-deleted)... LEAK-REMOVED(new, live)...]
--- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants (0.04s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/GetByIDForStore_(admin_detail) (0.00s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListAdmin_(admin_list) (0.00s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListPublished_(storefront_—_customer-visible) (0.00s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/ListPublishedBySlugs_(storefront_—_customer-visible) (0.00s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/GetPublishedByHandle_(storefront_—_customer-visible,_PDP) (0.00s)
    --- PASS: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/soft-deleted_row_still_exists_(not_destroyed) (0.00s)
    --- FAIL: TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants/re-add_with_same_SKU_as_soft-deleted_variant_succeeds_(partial_index) (0.00s)
FAIL
FAIL	github.com/mark8ly/marketplace-api/internal/product	1.223s
```

(Full untruncated output with complete struct dumps was also captured to `/tmp/revert_failure_output.txt`
during the run — a local scratch file, not committed.)

Note: the "soft-deleted row still exists" subtest correctly **PASSED** under the reverted type
too — that assertion tests something GORM's filter change doesn't affect (existence in the table),
so it's expected to pass in both states. This is exactly why it's a separate assertion from "not
in the preload": it isolates "destroyed" from "filtered," and only the fix changes the filtered
behavior.

Also as expected, the re-add subtest failed in the reverted state — not because the insert failed,
but because the pre-existing bug meant `GetByIDForStore` returned 3 "live" variants (the leaked
soft-deleted one plus the two real live ones) instead of 2, since nothing filtered the leak. This
confirms the assertion is actually exercising the fix, not something else.

### Restore

```
cp /tmp/models.go.bak internal/product/models.go
cp /tmp/service_aggregate.go.bak internal/product/service_aggregate.go
cp /tmp/newtest.go.bak internal/product/repository_variant_softdelete_integration_test.go
```

Confirmed via `diff` against each backup — all three matched exactly (no residual edits). Then:

```
git diff --stat   # only internal/product/repository_integration_test.go (the intentional edit)
git status --short
  M internal/product/repository_integration_test.go
  ?? internal/product/repository_variant_softdelete_integration_test.go  (untracked, new file)
go build ./...              → exit 0
go vet ./...                → exit 0
go vet -tags=integration ./... → exit 0
```

## Commands run, with exit codes

| Command | Exit |
|---|---|
| `go build ./...` | 0 |
| `go vet ./...` | 0 |
| `go vet -tags=integration ./...` | 0 |
| `go test ./... -count=1` (full non-integration suite, service root) | 0, no `FAIL` lines |
| `TEST_DATABASE_URL=... go test -tags=integration -p 1 -count=1 ./internal/product/...` | 1 |

The one non-zero exit is `TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected`
(`service_aggregate_test.go:259`, `expected ErrOptionValueInUse, got variant_matrix_mismatch:
variant count does not match option-value product`). **Confirmed pre-existing and unrelated**: ran
the same targeted test with my new test file moved out of the package and
`repository_integration_test.go` stashed back to its pre-Task-3 state (i.e. exactly Task 2's
commit `9b52bf44`, no Task 3 changes at all) — it fails identically:

```
--- FAIL: TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected (0.06s)
    service_aggregate_test.go:259: expected ErrOptionValueInUse, got variant_matrix_mismatch: variant count does not match option-value product
```

This is not one of the three pre-existing failures named in the plan's Global Constraints
(`internal/billing/trial`, `internal/subscription/planchange`, `internal/whitelabel`), so I'm
flagging it explicitly here rather than silently absorbing it into "expected failures." It exists
on the branch before any Task 3 work touched it and is out of scope for this task.

I also started an additional, non-required full-service integration run
(`go test -tags=integration -p 1 -count=1 ./...` from the service root) purely as extra
confirmation beyond the task's own verify line. It surfaced two more pre-existing, unrelated
failures before I killed it early (it was going to run every integration-tagged package in the
repo and wasn't needed for this task):

- `internal/arbitrage`: `TestAppealService_MarksAuditRowUnderReview` — `value too long for type
  character varying(100)` on `subscription_arbitrage_audit.mismatch_reason`.
- `internal/billing/migration`: 4 tests — `column "reviewer_operator_id" of relation
  "migration_fast_path_reviews" does not exist` — a schema-drift issue (model doesn't match the
  live migrations for that table).

Neither touches `product_variants`, `Variant`, or anything in `internal/product`. Not caused by
this change; flagging for completeness, not fixing (out of scope — see plan's Global Constraints,
"pre-existing failures not yours to fix").

## Concerns

- None regarding correctness of the fix or this test. The one loose end is the
  `variant_matrix_mismatch` failure above, which is pre-existing but not on the plan's named
  exclusion list — worth someone filing separately.
