# Integration Fixture Drift Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the drifted integration-test fixtures in `services/marketplace-api` and widen `make test-int` from 4 packages toward `./...`, so CI actually guards the 47 packages currently dark.

**Architecture:** Migrations added `NOT NULL` columns and foreign keys; fixtures were never updated. Three signatures account for 165 of 187 failures. Fix them with shared seed helpers in the existing untagged `pkg/testdb` package (imported by 136 test files already) rather than copying a helper into 13 packages, then widen the Makefile target one cluster at a time. Re-measure before triaging the tail: within a single test, an early insert failure masks everything after it, so the remaining ~22 failures cannot be honestly classified until the big three are gone.

**Tech Stack:** Go 1.26, GORM v1.25, PostgreSQL 15, `stretchr/testify` v1.11.1, `google/uuid` v1.6.0, build tag `integration`.

**Spec:**
- GitHub issues [#316](https://github.com/tesserix/mark8ly/issues/316) and [#317](https://github.com/tesserix/mark8ly/issues/317)
- Precedent for the fix pattern: #315 (`fix(dunning): repair the integration suite and the billing bug it was hiding`)
- Measured baseline artifacts: `/private/tmp/m8-baseline-artifacts/` — `baseline-full.txt` (raw), `baseline-tests.txt` (187 unique test names), `baseline-sha.txt` (`30c3fdff`)

---

## Global Constraints

- Module path `github.com/mark8ly/marketplace-api`; all work under `services/marketplace-api/`.
- Single-line conventional commits. No signature, no `Co-Authored-By`, no multi-line body.
- **Never push, open a PR, merge, deploy, or switch branches.** Ask before merging anything.
- `gofmt -l .` must be empty before every commit. Inserting a comment into an aligned Go struct re-aligns neighbours — run it yourself, do not assume.
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean.
- **Use the isolated baseline container, never the shared dev DB.** Another session shares `dev-postgres-1`, and two integration suites against one database corrupt each other. The container is already running:
  ```
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable'
  ```
  If it is gone, recreate it — note port 55432 is taken by `tesserix-dev-postgres`:
  ```bash
  docker run -d --name m8-baseline -e POSTGRES_USER=dev -e POSTGRES_PASSWORD=dev \
    -e POSTGRES_DB=marketplace_db -p 55433:5432 postgres:15
  docker exec m8-baseline psql -U dev -d marketplace_db -c 'CREATE EXTENSION IF NOT EXISTS pgcrypto;'
  cd services/marketplace-api && \
    DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' go run ./cmd/migrate up
  ```
  `pgcrypto` must be enabled manually — migration 058 uses `gen_random_bytes`. Verify schema is **106** and `dirty=f` before trusting any run.
- LAN IP `192.168.1.110`, never `localhost`. Always `-p 1` on integration runs (a shared Postgres exhausts its connection limit otherwise).
- **`testdb.NewDB` and `NewTx` SKIP when `TEST_DATABASE_URL` is unset or unreachable.** `exit=0` does not distinguish PASS from SKIP. Confirm `--- PASS` by name in every verification step.
- Do not change production code in this plan. Two production defects were found while measuring; they are filed as separate issues in Task 9, not fixed here.
- Schema facts, verified against the migrated database — do not re-derive:
  - `stores` NOT NULL with no default: `id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret`. **`stores` has no `created_at` column at all.**
  - `vendors` NOT NULL: `id` (default `gen_random_uuid()`), `tenant_id`, `name`, `slug`, `status` (default `'active'`), `is_self` (default `false`), `created_at`/`updated_at` (default `now()`).
  - `vendors` unique **indexes** — `vendors_slug_key` on `slug`, and `vendors_tenant_self_idx`, a **partial unique index on `(tenant_id) WHERE is_self = true`** enforcing one self-vendor per tenant. These are `CREATE UNIQUE INDEX`, not table constraints, so **`pg_constraint` does not list them — query `pg_indexes`.** An earlier revision of this plan asserted "no unique constraint beyond the primary key" for exactly that reason and was wrong.
  - `stores.synced_at` is NOT NULL but carries a default, which is why the seed helper omits it and still inserts cleanly.
  - `products_handle_per_store_live_unique` — unique on `(store_id, handle) WHERE deleted_at IS NULL`. **Task 3 must not give two live products the same handle within one store.** Derive handles per product rather than reusing a literal like `"linen-shirt"` when a test seeds more than one.
  - `eak_tenant_prefix_uniq` — unique on `enterprise_api_keys (tenant_id, key_prefix)`. Task 8's fixtures mint a fresh `tenantID` per test so the shared `"USED1234"` prefix is safe; it stops being safe the moment two keys share a tenant.
  - `products.vendor_id` is `NOT NULL` with **no foreign key** — only `products_store_id_fkey` and `products_primary_category_id_fkey` exist.
  - `stores_slug_unique` exists, so seeded slugs must be derived from the store id.

## Measured Baseline

187 failures / 22 packages at `30c3fdff`, on a clean isolated database at schema 106. Every failure is accounted for:

| Cluster | Errors | Packages | Fixed by |
|---|---|---|---|
| **A** `products.vendor_id` NOT NULL | 74 | `product` (54), `handlers/storefront` (18), `handlers/admin` (2) | Task 3 |
| **B** `store_subscriptions_store_id_fkey` | 69 | 13 packages | Tasks 4–6 |
| **C** `stores.storefront_customer_portal_secret` NOT NULL | 22 | `category` (16), `handlers/internalsvc` (4), `subscription/planchange` (2) | Task 2 |
| **D** `apikeys` `varchar(60)` overflow | 6 | `apikeys` | Task 8 |
| **E–H** tail: `orderrefund` cleanup deadlock, unmounted routes (404), `shipments_order_id_fkey`, `stores.created_at` | ~16 | mixed | Task 7 triage |

## File Structure

| File | Responsibility |
|---|---|
| `pkg/testdb/seed.go` *(create)* | Shared fixture seeding: `SeedStore`, `SeedVendor`. Untagged, like the rest of `pkg/testdb`, so any package can import it. One responsibility: produce minimal valid parent rows. |
| `pkg/testdb/seed_integration_test.go` *(create)* | Proves the helpers satisfy the live constraints — the helpers are themselves fixtures and must be tested, or a bug in them looks like a bug in 13 packages. |
| `Makefile:68-74` *(modify)* | The `test-int` marketplace-api package list, widened once per cluster task. |
| Per-package `*_test.go` *(modify)* | Call the shared helpers instead of hand-rolled inserts. Enumerated per task. |

---

### Task 1: Shared seed helpers in `pkg/testdb`

**Files:**
- Create: `services/marketplace-api/pkg/testdb/seed.go`
- Create: `services/marketplace-api/pkg/testdb/seed_integration_test.go`

**Interfaces:**
- Consumes: nothing — this is the base of the plan.
- Produces, relied on by every later task:
  - `func SeedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID)`
  - `func SeedVendor(t *testing.T, db *gorm.DB, tenantID uuid.UUID) uuid.UUID`

**Design notes — read before writing code.**

`tenantID` is supplied by the caller, never minted inside the helper. This is the deliberate difference between `internal/subscription/dunning`'s helper and `internal/order`'s, and it is the whole point: tests already mint a tenant for the rows they seed, and the store must carry *that* tenant. If the helper minted its own, the store and the subscription would disagree about tenancy and every tenant-scoped assertion would pass while testing nothing.

`SeedVendor` inserts a real `vendors` row even though `products.vendor_id` has no foreign key and any UUID would satisfy the `NOT NULL`. A real row keeps vendor-scoped filtering (`VendorScopeFilter`) and any products→vendors join meaningful. `is_self` is `true` to match `product.Create`, which defaults a new product to the tenant's self-vendor.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/pkg/testdb/seed_integration_test.go`:

```go
//go:build integration

package testdb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestSeedStore_SatisfiesNotNullAndUniqueConstraints(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()

	testdb.SeedStore(t, db, tenantID, storeID)

	var got struct {
		TenantID uuid.UUID
		Secret   string
	}
	err := db.Raw(
		`SELECT tenant_id, storefront_customer_portal_secret AS secret FROM stores WHERE id = ?`,
		storeID,
	).Scan(&got).Error
	require.NoError(t, err)
	require.Equal(t, tenantID, got.TenantID, "store must carry the caller's tenant")
	require.Len(t, got.Secret, 64, "portal secret must be 32 random bytes hex-encoded")
}

func TestSeedStore_TwoStoresInOneTestDoNotCollideOnSlug(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()

	testdb.SeedStore(t, db, tenantID, uuid.New())
	testdb.SeedStore(t, db, tenantID, uuid.New())

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM stores WHERE tenant_id = ?`, tenantID).Scan(&n).Error)
	require.EqualValues(t, 2, n)
}

func TestSeedVendor_ReturnsUsableSelfVendorForTenant(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()

	vendorID := testdb.SeedVendor(t, db, tenantID)
	require.NotEqual(t, uuid.Nil, vendorID)

	var got struct {
		TenantID uuid.UUID
		IsSelf   bool
	}
	err := db.Raw(
		`SELECT tenant_id, is_self FROM vendors WHERE id = ?`, vendorID,
	).Scan(&got).Error
	require.NoError(t, err)
	require.Equal(t, tenantID, got.TenantID)
	require.True(t, got.IsSelf, "seeded vendor stands in for the tenant's self-vendor")
}

// A product insert is the actual thing 74 baseline failures could not do.
func TestSeededParents_AcceptAProductInsert(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()

	testdb.SeedStore(t, db, tenantID, storeID)
	vendorID := testdb.SeedVendor(t, db, tenantID)

	err := db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, storeID, vendorID, "seed-probe", "Seed Probe", "draft",
	).Error
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./pkg/testdb/... -run 'TestSeed' -v
```

Expected: **build failure** — `undefined: testdb.SeedStore`, `undefined: testdb.SeedVendor`. A `--- SKIP` here means `TEST_DATABASE_URL` is wrong; fix that before continuing.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/pkg/testdb/seed.go`:

```go
package testdb

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SeedStore inserts a minimal stores row for (tenantID, storeID), satisfying
// every NOT NULL column the schema requires — including
// storefront_customer_portal_secret, which a later migration added and most
// fixtures never caught up with.
//
// tenantID is supplied by the caller rather than generated here. Callers
// already mint a tenant for the rows they seed, and the store must carry that
// same tenant: otherwise the store and whatever references it disagree about
// tenancy, and every tenant-scoped assertion passes while testing nothing.
//
// Registers a t.Cleanup deleting the row, so packages using NewDB (which
// truncates only the tables it was told about) still start clean.
func SeedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	// Derived from the store id to dodge stores_slug_unique when one test
	// seeds several stores.
	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("testdb.SeedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// SeedVendor inserts a vendors row owned by tenantID and returns its id, for
// use as products.vendor_id — NOT NULL since migration 000028.
//
// The column has no foreign key, so any UUID would satisfy the constraint.
// A real row is inserted anyway so that vendor-scoped filtering and any
// products→vendors join stay meaningful; a synthetic id would make those
// assertions vacuous in exactly the way this plan is trying to stop.
//
// is_self is true because product.Create defaults a new product to the
// tenant's self-vendor, so that is the vendor a fixture should stand in for.
func SeedVendor(t *testing.T, db *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()

	vendorID := uuid.New()
	slug := "vnd-" + strings.ReplaceAll(vendorID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO vendors (id, tenant_id, name, slug, status, is_self)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		vendorID, tenantID, "Test Vendor", slug, "active", true,
	).Error
	if err != nil {
		t.Fatalf("testdb.SeedVendor: insert vendors row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM vendors WHERE id = ?", vendorID)
	})

	return vendorID
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./pkg/testdb/... -run 'TestSeed' -v
```

Expected: four `--- PASS` lines, by name. Not `ok` alone — confirm each name.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: `gofmt -l .` prints nothing; the rest exit 0.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/pkg/testdb/seed.go services/marketplace-api/pkg/testdb/seed_integration_test.go
git commit -m "test(testdb): add shared SeedStore and SeedVendor fixture helpers"
```

---

### Task 2: Cluster C — `stores.storefront_customer_portal_secret` (22 failures)

Smallest of the three big clusters, and it exercises `SeedStore` end-to-end before 13 packages depend on it.

**Files:**
- Modify: `services/marketplace-api/internal/category/repository_integration_test.go:64`
- Modify: `services/marketplace-api/internal/handlers/internalsvc/storefront_status_test.go:81`
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange_integration_test.go:40` — the local `seedStores` helper
- Modify: `Makefile:68-74`

**Interfaces:**
- Consumes: `testdb.SeedStore(t, db, tenantID, storeID)` from Task 1.
- Produces: nothing new.

**Note on `internal/subscription/planchange` — corrected site.** #317 points at
`criterion_39_test.go:41`. That is the *call site* (`seedStores(t, db, tenantID, 1)`), not the broken
insert. The defect is in the package-local `seedStores` helper at `planchange_integration_test.go:40`,
whose INSERT omits `storefront_customer_portal_secret`. A second insert at
`planchange_integration_test.go:302` already supplies it correctly — leave that one alone.

Fix `seedStores` by having it call `testdb.SeedStore` in its loop, so there is one definition of a
valid store. Keep its `(t, db, tenantID, n)` signature and its `synced_at` behaviour: `synced_at` is
NOT NULL but carries a default, which is why `testdb.SeedStore` does not set it and still works. No
planchange production code reads `synced_at`, so dropping the explicit `now()` is safe.

Per the controller's pre-flight ruling: this task edits ONLY `planchange_integration_test.go`'s
`seedStores` helper in that package, and does NOT widen the Makefile for planchange — the package
stays red on cluster B until Task 5 owns it.

**Note on `internal/outbox`.** #317 lists `outbox/publisher_integration_test.go:58` as a cluster-C
site. It is **not** failing — the baseline records `ok  internal/outbox  3.002s`. The issue's site
list is stale there. Leave that file alone; do not "fix" a passing test to match an issue.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/category/... ./internal/handlers/internalsvc/... \
  ./internal/subscription/planchange/... 2>&1 | tee /tmp/c-before.txt
grep -c 'storefront_customer_portal_secret' /tmp/c-before.txt
```

Expected: a non-zero count. Record it — you will assert it reaches 0.

- [ ] **Step 2: Replace each hand-rolled stores INSERT with the helper**

At each of the four sites, delete the inline `INSERT INTO stores (...)` and call the helper. The import to add is `"github.com/mark8ly/marketplace-api/pkg/testdb"` if not already present.

Before (shape varies slightly per file):

```go
err := db.Exec(`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    storeID, tenantID, "test-store", "Test Store", "IE", "EUR", "Europe/Dublin", "active").Error
require.NoError(t, err)
```

After:

```go
testdb.SeedStore(t, db, tenantID, storeID)
```

If a site does not already have a `tenantID` in scope, mint one with `uuid.New()` and pass the *same* value to every other row the test seeds. Do not let the helper invent one.

- [ ] **Step 3: Run the four packages and verify the signature is gone**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/category/... ./internal/handlers/internalsvc/... \
  ./internal/subscription/planchange/... 2>&1 | tee /tmp/c-after.txt
grep -c 'storefront_customer_portal_secret' /tmp/c-after.txt
```

Expected: `0`. Any remaining failures in these packages belong to cluster B or the tail — leave them, they are handled later. Do not widen the Makefile for a package that is not fully green.

- [ ] **Step 4: Widen `make test-int` for whichever of these packages are now fully green**

In `Makefile`, add the green packages to the marketplace-api list (currently `./internal/audit/...`, `./internal/handlers/platformadmin/...`, `./internal/tenantpurge/...`, `./internal/subscription/dunning/...`), keeping the existing comment block intact:

```make
	    ./internal/audit/... \
	    ./internal/handlers/platformadmin/... \
	    ./internal/tenantpurge/... \
	    ./internal/subscription/dunning/... \
	    ./internal/category/...
```

Add only packages whose full run is green. A permanently red target is worse than a narrow green one — that is exactly how #315's production dunning bug stayed hidden.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(fixtures): seed stores via testdb.SeedStore and widen test-int"
```

---

### Task 3: Cluster A — `products.vendor_id` NOT NULL (74 failures)

**Files:**
- Modify: `services/marketplace-api/internal/product/models_integration_test.go:81` and siblings in that package
- Modify: `services/marketplace-api/internal/handlers/storefront/storefront_integration_test.go:233` and siblings
- Modify: `services/marketplace-api/internal/handlers/admin/products_integration_test.go:223`
- Modify: `services/marketplace-api/internal/handlers/storefront/dto_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/dto_test.go`
- Modify: `services/marketplace-api/internal/product/repository_integration_test.go`
- Modify: `services/marketplace-api/internal/category/` — **added after Task 2**, see below
- Modify: `Makefile`

**Interfaces:**
- Consumes: `testdb.SeedVendor(t, db, tenantID) uuid.UUID` and `testdb.SeedStore` from Task 1.
- Produces: nothing new.

**`internal/category` was added to this task after Task 2 landed, and the reason matters.** At
baseline `category` showed 16 `storefront_customer_portal_secret` failures and zero `vendor_id`
failures. With cluster C fixed, it now shows **2 `vendor_id` failures that were previously invisible**
— the earlier insert aborted the transaction (`SQLSTATE 25P02`) before the product insert was ever
reached. This is the masking effect this plan predicted, observed for real, and it is why Task 7
re-measures rather than trusting the baseline's per-package attribution. Expect the same to happen
again: after this task, packages you did not touch may reveal new signatures. That is the process
working, not a regression.

**Note on the model.** `internal/product/models.go:52` declares `VendorID *string` against a `NOT NULL` column. That mismatch is *why* these fixtures compile while producing invalid rows. Tightening it is deliberately **out of scope** — it changes production code, JSON `omitempty` behaviour and every nil check. Task 9 files it as its own issue. Here, set the pointer.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/product/... ./internal/handlers/storefront/... ./internal/handlers/admin/... 2>&1 | tee /tmp/a-before.txt
grep -c 'null value in column "vendor_id"' /tmp/a-before.txt
```

Expected: non-zero (74 across these three packages at baseline).

- [ ] **Step 2: Seed a vendor per test and attach it to every product insert**

Where the test builds a `product.Product` struct:

```go
vendorID := testdb.SeedVendor(t, db, tenantID)
vendorIDStr := vendorID.String()

prod := &product.Product{
	TenantID:          tenantID,
	StoreID:           storeID,
	VendorID:          &vendorIDStr,
	Handle:            "linen-shirt",
	Title:             "Linen Shirt",
	Status:            product.StatusDraft,
	Tags:              []string{"summer", "linen"},
	PrimaryCategoryID: &shirtCat.ID,
}
```

Where the test inserts products with raw SQL, add the column and the value:

```go
err := db.Exec(
	`INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, status)
	 VALUES (?, ?, ?, ?, ?, ?, ?)`,
	uuid.New(), tenantID, storeID, vendorID, "linen-shirt", "Linen Shirt", "draft",
).Error
```

Seed the vendor **once per test** and reuse the id for every product in that test. Pass the same `tenantID` used for the store and the products — a vendor on a different tenant makes vendor-scoped assertions vacuous.

- [ ] **Step 3: Run the three packages and verify the signature is gone**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/product/... ./internal/handlers/storefront/... ./internal/handlers/admin/... 2>&1 | tee /tmp/a-after.txt
grep -c 'null value in column "vendor_id"' /tmp/a-after.txt
```

Expected: `0`. `internal/handlers/admin` will still fail on the `stores.created_at` signature — that is cluster F, Task 7. Do not widen the Makefile for it.

- [ ] **Step 4: Widen `make test-int` for the packages now fully green**

Same edit shape as Task 2, Step 4. `./internal/product/...` is the expected addition.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(fixtures): seed vendors for product inserts and widen test-int"
```

---

### Task 4: Cluster B part 1 — billing packages (21 failures)

Cluster B is one root cause across 13 packages: tests insert `store_subscriptions` for a `store_id` with no `stores` row. It is split across three tasks purely so each is reviewable; the edit is identical everywhere.

**Files:**
- Modify: `services/marketplace-api/internal/billing/appaddon/*_test.go` (4)
- Modify: `services/marketplace-api/internal/billing/dispatch/*_test.go` (9)
- Modify: `services/marketplace-api/internal/billing/tax/*_test.go` (4)
- Modify: `services/marketplace-api/internal/billing/tax/revalidation/*_test.go` (4)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `testdb.SeedStore` from Task 1.
- Produces: nothing new.

**Only four billing packages are failing.** `internal/billing/trial`, `internal/billing/migration`
and `internal/billing/tax/windowguard` are GREEN at baseline — they reference `store_subscriptions`
but seed correctly. Running `./internal/billing/...` exercises them, which is fine and desirable, but
**do not edit them**. The failing four are `appaddon` (4), `dispatch` (9), `tax` (4) and
`tax/revalidation` (4) = 21.

**Why these first.** `internal/billing/dispatch` is the package #384 and #389 rewrote. Its tests currently do not run in CI at all, which means the email-delivery invariant those PRs established — `Send` returns `nil` iff a provider accepted — is guarded by tests nothing executes.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/billing/... 2>&1 | tee /tmp/b1-before.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b1-before.txt
```

- [ ] **Step 2: Seed the store before each `store_subscriptions` insert**

Add, immediately before the subscription insert:

```go
testdb.SeedStore(t, db, tenantID, storeID)
```

Some of these packages already define a local `seedStore`. Delete the local copy and use the shared helper, so there is one definition of a valid store. If a local helper does something extra (dropping per-store sequences, for instance), keep that extra behaviour in the caller rather than reintroducing a second store-seeding path.

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/billing/... 2>&1 | tee /tmp/b1-after.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b1-after.txt
```

Expected: `0`, and `--- PASS` for the dispatch tests by name.

- [ ] **Step 4: Widen `make test-int` for the green billing packages**

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(billing): seed stores for subscription fixtures and widen test-int"
```

---

### Task 5: Cluster B part 2 — subscription packages (22 failures)

**Files:**
- Modify: `services/marketplace-api/internal/subscription/*_test.go` (6)
- Modify: `services/marketplace-api/internal/subscription/planchange/*_test.go` (7)
- Modify: `services/marketplace-api/internal/subscription/readonly/*_test.go` (2)
- Modify: `services/marketplace-api/internal/subscription/statemachine/cas_conflict_test.go:34` and siblings (7)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `testdb.SeedStore` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/... 2>&1 | tee /tmp/b2-before.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b2-before.txt
```

- [ ] **Step 2: Apply the same edit as Task 4, Step 2**

`internal/subscription/dunning` is already correct and already in `make test-int` — it is the source of this pattern. Replace its local `seedStore` with the shared helper too, so there is one definition, but keep `seedPastDueSubscription` where it is: that helper encodes what "eligible for dunning" means and is dunning-specific.

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/subscription/... 2>&1 | tee /tmp/b2-after.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b2-after.txt
```

Expected: `0`. Confirm `internal/subscription/dunning` still passes — it is currently green and in CI, so a regression there is a real regression, not drift.

- [ ] **Step 4: Widen `make test-int` for the green subscription packages**

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(subscription): seed stores for subscription fixtures and widen test-int"
```

---

### Task 6: Cluster B part 3 — remaining packages (26 failures)

**Files:**
- Modify: `services/marketplace-api/internal/arbitrage/appeal_test.go:40` and siblings (15)
- Modify: `services/marketplace-api/internal/campaignbudget/*_test.go` (4)
- Modify: `services/marketplace-api/internal/campaignbudget/cron/*_test.go` (2)
- Modify: `services/marketplace-api/internal/handlers/webhooks/*_test.go` (2)
- Modify: `services/marketplace-api/tests/integration/*_test.go` (3)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `testdb.SeedStore` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/arbitrage/... ./internal/campaignbudget/... \
  ./internal/handlers/webhooks/... ./tests/integration/... 2>&1 | tee /tmp/b3-before.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b3-before.txt
```

- [ ] **Step 2: Apply the same edit as Task 4, Step 2**

Note `internal/arbitrage/appeal_test.go:31` contains a stub `seedFlaggedAudit` whose body is an empty comment and whose `db` parameter is an anonymous `interface{}`. Give it the concrete `*gorm.DB` type and a real body, or delete it if nothing calls it — a helper that silently does nothing is how a test passes while asserting nothing.

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/arbitrage/... ./internal/campaignbudget/... \
  ./internal/handlers/webhooks/... ./tests/integration/... 2>&1 | tee /tmp/b3-after.txt
grep -c 'store_subscriptions_store_id_fkey' /tmp/b3-after.txt
```

Expected: `0`.

- [ ] **Step 4: Widen `make test-int` for the green packages**

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(fixtures): seed stores across arbitrage, campaignbudget and webhooks"
```

---

### Task 7: Re-measure and triage the tail

The tail could not be classified before now: within a single test, an early insert failure aborts the transaction (`SQLSTATE 25P02`, "current transaction is aborted") and masks every assertion after it. Some tail failures will have disappeared with clusters A–C; others will only now become visible. Both directions are expected.

**Files:**
- Create: `docs/superpowers/plans/2026-08-27-integration-fixture-drift-tail.md` (findings only; no code changes in this task)

**Interfaces:**
- Consumes: everything from Tasks 1–6.
- Produces: a triaged list the next task acts on.

- [ ] **Step 1: Re-run the full suite against the isolated container**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./... > /tmp/after-full.txt 2>&1
grep -E '^\s*--- FAIL: ' /tmp/after-full.txt | sed -E 's/^[[:space:]]*--- FAIL: //; s/ \(.*//' | sort -u > /tmp/after-tests.txt
wc -l < /tmp/after-tests.txt
```

- [ ] **Step 2: Diff both directions against the baseline**

```bash
[ -s /private/tmp/m8-baseline-artifacts/baseline-tests.txt ] || { echo "BASELINE MISSING — re-measure before trusting any diff"; exit 1; }
echo "=== fixed by this branch ==="
comm -23 /private/tmp/m8-baseline-artifacts/baseline-tests.txt /tmp/after-tests.txt
echo "=== NEW failures — these are yours ==="
comm -13 /private/tmp/m8-baseline-artifacts/baseline-tests.txt /tmp/after-tests.txt
```

The guard on the first line is not decoration: `comm` against a deleted baseline prints nothing and looks like total success. Any name under "NEW failures" is a regression this branch caused and must be fixed before proceeding.

- [ ] **Step 3: Cluster whatever remains by error signature**

```bash
grep -oE '(ERROR: [^(]*\(SQLSTATE [0-9A-Z]+\)|value too long for type [a-z()0-9 ]+|deadlock detected|404 page not found|nil pointer dereference)' /tmp/after-full.txt \
  | sed 's/ERROR: //' | sort | uniq -c | sort -rn
```

- [ ] **Step 4: Write up the triage**

For each remaining signature record: the signature, affected packages, whether it is a **fixture**
problem (fix here) or a **product/routing** problem (file an issue, do not paper over). The
`404 page not found` failures in `tests/integration` are the ones to be most careful with — a test
expecting an unmounted route may be asserting a real gap, and silently deleting it would destroy the
signal.

Then **append a concrete task to this plan for every fixture-class finding**, following the shape of
Task 2: exact files, exact edit, a verify command scoped by `-run`, a Makefile widening, and a
commit. A triage document that does not become tasks is where this work stalls. The known
candidates, all named in #317 and all still unfixed at that point, are `internal/orderrefund`
(cleanup deadlock plus a config-wiring gap at `resolver_integration_test.go:239`), `internal/page`,
`internal/handlers/admin` (`stores.created_at`, a column the schema does not have at all), and
`internal/whitelabel/lifecycle` (blocked on the Task 9 audit-emitter issue, since the panic is
production-side).

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-08-27-integration-fixture-drift-tail.md
git commit -m "docs: triage the remaining integration failures after clusters A-C"
```

---

### Task 8: Cluster D — `apikeys` `varchar(60)` overflow (6 failures)

**Files:**
- Modify: `services/marketplace-api/internal/apikeys/lastused_test.go:33`
- Modify: `services/marketplace-api/internal/apikeys/repo_test.go:37,59,77`
- Modify: `Makefile`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: nothing new.

**This one is already resolved — do not re-derive it.** The table is `enterprise_api_keys`, not
`api_keys`, and the offending column is `key_hash varchar(60)`. Both failing fixtures write the same
literal:

```
$2a$12$abcdefghijklmnopqrstuv.QwertyuiopAsdfghjklZxcvbnmAsdfgh.
```

which is **63 characters**. A real bcrypt hash is exactly **60**. So `varchar(60)` is correct and the
fixture is wrong — this is not a production bug, and no migration is needed.

- [ ] **Step 1: Replace the over-long literal in both files**

Use a literal that is exactly 60 characters and still bcrypt-shaped (`$2a$12$` + 53). Verify the
length rather than trusting the eye — the current value is wrong by three characters, which is
exactly the kind of error that reading does not catch:

```bash
python3 -c "s='\$2a\$12\$abcdefghijklmnopqrstuv.QwertyuiopAsdfghjklZxcvbnmAsd'; print(len(s))"
```

Expected: `60`. Then set `KeyHash` to that value in both
`internal/apikeys/lastused_test.go:33` and `internal/apikeys/repo_test.go:37,59,77`.

- [ ] **Step 2: Confirm no other fixture carries the bad literal**

```bash
cd services/marketplace-api
grep -rn 'QwertyuiopAsdfghjklZxcvbnmAsdfgh' --include='*_test.go' . || echo "clean"
```

Expected: `clean`.

- [ ] **Step 3: Verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/apikeys/... -v 2>&1 | grep -E '^(---|ok|FAIL)'
```

Expected: `--- PASS` for all four previously failing tests, by name.

- [ ] **Step 4: Widen `make test-int` for `./internal/apikeys/...`**

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(apikeys): fix varchar(60) overflow in fixtures and widen test-int"
```

---

### Task 9: File the production defects found while relighting the suite

*(Rewritten after Task 7's triage. This was scoped as two issues; the triage found ten defects, so it
is now eight new issues plus one comment. None is a fixture problem, and none may be fixed inside a
test-repair branch. Every one of them was found by running a suite that had been dark — which is the
entire argument for this plan.)*

**Files:**
- No code changes except one comment in `internal/product/models.go`. Eight GitHub issues, one comment.

**CHECK FOR DUPLICATES BEFORE FILING ANYTHING.** `gh issue list --state all --search "<keywords>"`
for each defect. The `audit.Emitter` panic is **already filed as #318** (created 2026-08-23) — do NOT
re-file it; add a comment recording the independent reproduction instead. A duplicate issue is worse
than no issue. Read back every write with `gh issue view <n>` — a `gh` write that is not read back is
not known to have happened.

**Severity order — file the customer-facing one first**, so that if anything interrupts you the most
important one exists:

1. **GORM `default:` + non-pointer `bool`: `false` can never be persisted.** `page.Published`
   (`internal/page/models.go:21`) and `category.IsActive` (`internal/category/models.go:20`) are plain
   `bool` fields carrying `gorm:"...default:true"`. GORM omits a zero-valued field from the INSERT
   when it has a `default:` tag, and `false` is the zero value for `bool`, so the column default
   `true` silently wins. `page.Repository.Create` (`repository.go:29-34`) does a plain `.Create(p)`
   with no workaround. **Consequence: categories cannot be deactivated, and unpublished pages are
   served to customers on the storefront.** Five failing tests across `page`, `handlers/storefront`
   and `category` share this one root cause. Suggested fix: `*bool` fields, or `Select`/`Omit` on
   create, or drop the `default:` tag. Note in the issue that any other model with the same
   shape (`bool` + `default:`) is suspect and worth an audit.
2. **`Variant.DeletedAt` is `*time.Time`, not `gorm.DeletedAt`** (`internal/product/models.go:130`;
   `gorm.DeletedAt` appears zero times in that file). GORM's automatic soft-delete filtering
   therefore never applies, and all five `Preload("Variants")` sites in
   `internal/product/repository.go` (lines 209, 281, 351, 405, 447) return soft-deleted variants
   alongside active ones on every aggregate read. Systemic — affects every caller, not one test.
3. **The tax revalidation cron deadlocks, and takes the whole integration suite with it.**
   `Cron.Run` (`internal/billing/tax/revalidation/cron.go:78`) opens a transaction and takes
   `pg_advisory_xact_lock`; `processOne` UPDATEs `store_subscriptions` inside it (`cron.go:143`);
   then `c.Svc.Submit(ctx, in)` — which takes no `tx` (`internal/billing/tax/service.go:67`) — runs on
   a different pooled connection and writes the same row. It blocks on the uncommitted row lock while
   the outer transaction waits for it to return. Postgres's deadlock detector sees no cycle (the
   outer transaction is idle-in-transaction, not waiting on a lock), so there is no error — just an
   unbounded hang. **Include the blast radius**, measured: with this package in a `-p 1` run,
   `billing/trial`, `campaignbudget`, `campaignbudget/cron` and `category` all hit the test timeout;
   with it absent they take 11.3s, 1.1s, 0.6s and 0.9s. In production the cron hangs forever on the
   first stale validation, holding the advisory lock and blocking every subsequent run. It has never
   run successfully. Note that `make test-int` deliberately lists `./internal/billing/tax`
   non-recursively because of this.
4. **A blocked downgrade's audit row is written and then rolled back.**
   `internal/subscription/planchange/downgrade.go:41-50` calls `WritePlanChangeAuditRowTx` with
   `Action: "downgrade_blocked_over_quota"` on the same `tx` the operation runs in, then returns
   `ErrStoreCountOverQuota` at `downgrade.go:70`. That `tx` comes from `WithAdvisoryLock`
   (`internal/subscription/advisory_lock.go:15-22`), which wraps everything in
   `db.Transaction(...)` — a non-nil return rolls it back. The comment at `downgrade.go:40` says the
   row exists "so ops can see why the downgrade was refused"; it is defeated by the transaction it
   runs inside. The audit trail for the one case ops most needs never persists.
5. **Merchant appeals cannot be recorded.** `internal/arbitrage/appeal.go:89` builds
   `existingReason + "\n---\nMERCHANT_APPEAL jurisdiction=" + country`, then appends
   `" justification=" + <merchant free text>` and `" doc=" + <URL>`, and writes the result to
   `subscription_arbitrage_audit.mismatch_reason`, which is `varchar(100)`. The boilerplate alone is
   ~40 characters on top of an existing reason, so any realistic appeal overflows and the UPDATE
   fails with SQLSTATE 22001 — the appeal is not recorded. Unbounded by design: every appeal appends
   to the same 100-char column. Suggested fix: widen the column, or move appeals to their own table.
6. **`ApplyTrialRamp` is not idempotent against consumption.**
   `internal/campaignbudget/ramp.go:56-73` runs `SET remaining = GREATEST(remaining, 2000)`. Its own
   docstring (`ramp.go:44-46`) claims idempotency, but if a merchant spends between runs, a re-run
   re-inflates the balance they already consumed. Seed 50 → ramp(day4) → 2000 → `Reserve(200)` → 1800
   → ramp(day4) again → back to 2000.
7. **`OptionValueInUse` appears unreachable through the public API.**
   `internal/product/service_aggregate.go:445`'s guard sits behind `ValidateMatrix`
   (`internal/product/matrix.go:52`, called at `service_aggregate.go:108`), which rejects any variant
   referencing an option value absent from the desired spec — which is precisely the scenario the
   guard exists for. Either loosen the validation so the intended check can run, or confirm the path
   is dead and document it as defence-in-depth. State the uncertainty honestly; this one is
   "appears", not "is".
8. **Cross-package integration-test pollution.** `pkg/testdb`'s `NewDB` truncates only the tables it
   is explicitly given, so a package doing raw inserts leaves rows that break the next package. Since
   the fixtures were repaired, a full `./...` run reports ~46 failures that do not reproduce, and 7 of
   19 "failing" packages actually pass on a clean database. **Any full-suite measurement must
   TRUNCATE between packages or it reports fiction.** This did not surface before because tests
   aborted early on FK/NOT NULL violations and never wrote enough to pollute anything — the repair
   made it visible. Include the reproduction: run the 19 packages sequentially without a reset and
   watch `dispatch` and `dunning` flip between pass and fail depending on order.

**Interfaces:**
- Consumes: findings from the baseline run.
- Produces: issue numbers to reference from `internal/product/models.go` and the tail triage doc.

- [ ] **Step 1: Check every defect for an existing issue before filing**

```bash
for q in "Emitter nil Repo" "revalidation deadlock" "mismatch_reason" "vendor_id NOT NULL" \
         "soft delete variant" "published default" "ApplyTrialRamp" "OptionValueInUse" \
         "testdb truncate"; do
  printf "\n### %s\n" "$q"
  gh issue list --state all --search "$q" --limit 5 --json number,state,title \
    --jq '.[]|"  #\(.number) \(.state) \(.title)"'
done
```

At the time of writing only one match existed: **#318**, `audit.NewEmitter accepts a nil Repo and
panics a worker goroutine on first write`, created 2026-08-23. Verify that is still the case — if
someone has filed one of the others in the meantime, comment on theirs instead of opening a second.

- [ ] **Step 2: File the eight new issues, in the severity order given above**

One issue per numbered defect in this task's header. Copy the file:line evidence verbatim — it was
verified against source by two independent reviewers, and re-deriving it risks introducing errors.
Do not merge defects 1 and 2 into a single "GORM modelling" issue: they have different fixes, different
blast radii, and one is customer-facing while the other is not.

- [ ] **Step 3: Comment on #318 rather than re-filing it**

```bash
gh issue comment 318 --body "<reproduction note>"
```

The comment should record: reproduced independently on 2026-08-27 by `internal/whitelabel/lifecycle`
during the integration-fixture repair; the panic aborts the whole test binary, so that package's
remaining tests never run; and that this is why the package still shows a failure after the fixture
work. Do not restate the diagnosis already in the issue.

- [ ] **Step 4: Read back every write**

```bash
for n in <each new issue number>; do
  gh issue view "$n" --json number,state,title,body --jq '"#\(.number) \(.state) bodylen=\(.body|length) \(.title)"'
done
gh issue view 318 --json comments --jq '.comments[-1].body|length'
```

A `gh` write that is not read back is not known to have happened. A body length of 0 means the issue
was created empty — refill it with `gh issue edit <n> --body-file`.

- [ ] **Step 5: Reference the model issues from the code**

Above `internal/product/models.go:52`:

```go
// VendorID is a pointer for historical reasons; the column has been NOT NULL
// since migration 000028. See issue #<vendor-id-issue> — tightening it to a
// plain string touches JSON omitempty and every nil check, so it is tracked
// separately.
```

Above `internal/product/models.go:130` (the `Variant.DeletedAt` field):

```go
// DeletedAt is a plain *time.Time, not gorm.DeletedAt, so GORM's automatic
// soft-delete filtering does NOT apply — every Preload("Variants") returns
// deleted rows. See issue #<soft-delete-issue>.
```

Run `gofmt -l .` afterwards: inserting a comment into an aligned struct re-aligns the fields above
it, and the review will not catch it if you did not run the tooling.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/product/models.go
git commit -m "docs(product): record the VendorID and soft-delete modelling issues"
```

---

### Task 10: Wire `SubscriptionHandler`/`PromoHandler` into the admin test routers (2 failures)

Source: tail triage (`docs/superpowers/plans/2026-08-27-integration-fixture-drift-tail.md`).

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/products_integration_test.go` — `setupTestRouter` (~line 74)
- Modify: `services/marketplace-api/internal/handlers/admin/refund_integration_test.go` — `setupRefundTestEnv` (~line 35)

**Interfaces:**
- Consumes: `admin.NewSubscriptionHandler(svc *subscription.Service, logger *slog.Logger) *SubscriptionHandler`, `admin.NewPromoHandler(db, svc PromoApplier, subRepo subscription.Repository, logger) *PromoHandler`, `subscription.NewService(subscription.ServiceConfig{...}) *Service`, `promo.NewService(db, promo.NewRepository(), nil, nil) *Service`.
- Produces: nothing new.

**Root cause, not a routing bug.** `internal/handlers/admin/routes.go:704` mounts the entire
`/subscription` group only `if deps.SubscriptionHandler != nil`, and nests `/apply-promo`,
`/promo`, and `/refund` further behind `if deps.PromoHandler != nil` / `if deps.RefundHandler != nil`
respectively. Both test routers construct `admin.Deps{}` without ever setting `SubscriptionHandler`,
so the group is never registered — the routes exist in production and would work the moment a
`SubscriptionHandler` is wired. Do not add a route; wire the missing dependency.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... \
  -run 'TestPromoApply_BelowAbsoluteFloor_INR|TestRefund_OutsideCoolingOff' -v 2>&1 | tee /tmp/t10-before.txt
grep -c '404 page not found' /tmp/t10-before.txt
```

Expected: non-zero.

- [ ] **Step 2: Wire the missing handlers**

In `products_integration_test.go`, add imports `"github.com/mark8ly/marketplace-api/internal/promo"`
and `"github.com/mark8ly/marketplace-api/internal/subscription"`. Inside `setupTestRouter`, before
`r := gin.New()`:

```go
subSvc := subscription.NewService(subscription.ServiceConfig{
	DB:   db,
	Repo: subscription.NewRepository(),
})
subHandler := admin.NewSubscriptionHandler(subSvc, nil)
promoHandler := admin.NewPromoHandler(db, promo.NewService(db, promo.NewRepository(), nil, nil), subscription.NewRepository(), nil)
```

Add `SubscriptionHandler: subHandler, PromoHandler: promoHandler,` to the `admin.Deps{}` literal
passed to `admin.RegisterAdmin`.

In `refund_integration_test.go`, `subscription` is already imported and `subRepo :=
subscription.NewRepository()` already exists. Add, before `r := gin.New()`:

```go
subHandler := admin.NewSubscriptionHandler(
	subscription.NewService(subscription.ServiceConfig{DB: db, Repo: subRepo}), nil,
)
```

Add `SubscriptionHandler: subHandler,` to that file's `admin.Deps{}` literal (`RefundHandler` is
already set there).

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... \
  -run 'TestPromoApply_BelowAbsoluteFloor_INR|TestRefund_OutsideCoolingOff' -v 2>&1 | tee /tmp/t10-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t10-after.txt
```

Expected: `--- PASS` for both, by name.

- [ ] **Step 4: Widen `make test-int`**

Add `./internal/handlers/admin/...` only once Task 13 (below) also lands — this package has two
other failing tests until then. Do not widen for a partially-red package.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(admin): wire SubscriptionHandler and PromoHandler into test routers"
```

---

### Task 11: `orderrefund` — wire a `NoopEncryptor` in the resolver test (1 failure)

Source: tail triage. Exactly the "config-wiring gap at `resolver_integration_test.go:239`" named in
the original brief for #317.

**Files:**
- Modify: `services/marketplace-api/internal/orderrefund/resolver_integration_test.go:229` (`TestGatewayFor_ActiveConfig`)

**Interfaces:**
- Consumes: `orderrefund.(*Resolver).WithEncryptor(e crypto.Encryptor) *Resolver`, `crypto.NewNoopEncryptor()` — both already exist and are already used the same way by the sibling file `resolver_creds_test.go:56` in this package.
- Produces: nothing new.

`orderrefund.NewResolver(db)` deliberately returns an unconfigured resolver — see the doc comment at
`resolver.go:48`: "Wire `WithSecretStore` (or `WithEncryptor`) before use." `TestGatewayFor_ActiveConfig`
never chains either, so `GatewayFor` hits the intentional guard at `resolver.go:78`. This is not a
production defect; every other test in the package already wires one.

- [ ] **Step 1: Confirm the current failure**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/orderrefund/... -run 'TestGatewayFor_ActiveConfig' -v 2>&1 | tee /tmp/t11-before.txt
grep -c 'no secret store or encryptor wired' /tmp/t11-before.txt
```

Expected: `1`.

- [ ] **Step 2: Chain `.WithEncryptor`**

Add import `"github.com/mark8ly/marketplace-api/internal/crypto"`. In `TestGatewayFor_ActiveConfig`:

```go
r := orderrefund.NewResolver(db).WithEncryptor(crypto.NewNoopEncryptor())
```

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/orderrefund/... -run 'TestGatewayFor_ActiveConfig' -v 2>&1 | tee /tmp/t11-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t11-after.txt
```

Expected: `--- PASS: TestGatewayFor_ActiveConfig`. Do not widen `make test-int` for this package —
`internal/orderrefund` has a separate cleanup deadlock named in #317 that this task does not touch.

- [ ] **Step 4: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test(orderrefund): wire a NoopEncryptor into the resolver active-config test"
```

---

### Task 12: `handlers/webhooks` — fix the off-by-one fixture path (1 failure)

Source: tail triage.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/webhooks/stripe_integration_test.go:41`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing new.

The test file lives at `internal/handlers/webhooks/`, three directories below
`services/marketplace-api/`. Its `filepath.Join("..", "..", "scripts", "webhook-fixtures")` only
climbs two, landing on `internal/scripts/webhook-fixtures` — which does not exist. The fixtures are
real, tracked, and present at `services/marketplace-api/scripts/webhook-fixtures/` (verified via
`git ls-files` — 11 JSON files). This is a path bug, not a missing-fixture bug.

- [ ] **Step 1: Confirm the current failure**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/webhooks/... -run 'TestFullWebhookFlow_AllAllowlistedEvents' -v 2>&1 | tee /tmp/t12-before.txt
grep -c 'no such file or directory' /tmp/t12-before.txt
```

Expected: `1`.

- [ ] **Step 2: Add the missing `..`**

```go
dir := filepath.Join("..", "..", "..", "scripts", "webhook-fixtures")
```

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/webhooks/... -run 'TestFullWebhookFlow_AllAllowlistedEvents' -v 2>&1 | tee /tmp/t12-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t12-after.txt
```

Expected: `--- PASS: TestFullWebhookFlow_AllAllowlistedEvents`.

- [ ] **Step 4: Widen `make test-int` for `./internal/handlers/webhooks/...`**, if the full package is green.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(webhooks): fix the off-by-one relative path to scripts/webhook-fixtures"
```

---

### Task 13: `handlers/admin` shipments — seed the parent order and use `testdb.SeedStore` (3 failures)

Source: tail triage. This is the `stores.created_at` / `shipments_order_id_fkey` cluster the original
Task 7 brief named for `internal/handlers/admin`.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/shipments_dispatched_dedup_test.go:34` (`TestShipmentDispatchedEmailGate`)
- Modify: `services/marketplace-api/internal/handlers/admin/shipments_tracking_sync_test.go:273` (`seedStoreRowForSync`)

**Interfaces:**
- Consumes: `testdb.SeedStore(t, db, tenantID, storeID)` from Task 1; `seedOrderRowForSync(t, db, orderID, storeID, tenantID)`, already defined in this package at `shipments_tracking_sync_test.go:290`.
- Produces: nothing new.

**`stores.created_at` does not exist.** `seedStoreRowForSync` hand-rolls
`INSERT INTO stores (..., created_at, ...)`. Per this plan's verified schema facts, `stores` has no
`created_at` column at all. Replace the hand-rolled insert with the shared helper, same as every
other cluster-C/A site.

**`shipments_order_id_fkey`.** `TestShipmentDispatchedEmailGate` mints `orderID := uuid.New()` and
inserts a `shipments` row referencing it directly, without ever inserting the parent `orders` row.
`seedOrderRowForSync` already exists in this same package for exactly this purpose.

- [ ] **Step 1: Confirm the current failures, by name**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... \
  -run 'TestShipmentDispatchedEmailGate|TestShipmentsSync_AdvancesStatusLadder|TestShipmentsSync_CarrierErrorDoesNotBlockOthers' -v 2>&1 | tee /tmp/t13-before.txt
grep -cE 'created_at.*does not exist|shipments_order_id_fkey' /tmp/t13-before.txt
```

Expected: non-zero.

- [ ] **Step 2: Apply both edits**

In `shipments_tracking_sync_test.go`, replace `seedStoreRowForSync`'s body with a call to
`testdb.SeedStore(t, db, tenantID, storeID)` (add the `pkg/testdb` import if not already present —
it already is, per the file's existing `testdb.NewDB` usage). Delete the hand-rolled `INSERT INTO
stores` and its comment about `migrations/000002_orders_initial.up.sql`.

In `shipments_dispatched_dedup_test.go`, immediately before the `db.Create(&ship)` call in
`TestShipmentDispatchedEmailGate`, add:

```go
seedOrderRowForSync(t, db, orderID, storeID, tenantID)
```

(Both files are in package `admin_test`, so the helper is already in scope — no new import needed.)

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... \
  -run 'TestShipmentDispatchedEmailGate|TestShipmentsSync_AdvancesStatusLadder|TestShipmentsSync_CarrierErrorDoesNotBlockOthers' -v 2>&1 | tee /tmp/t13-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t13-after.txt
```

Expected: `--- PASS` for all three, by name.

- [ ] **Step 4: Widen `make test-int` for `./internal/handlers/admin/...`**

Only once this task's three tests and Task 10's two tests are all green — confirm with a full
package run before widening:

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... 2>&1 | tail -5
```

Expected: `ok`.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test(admin): seed the parent order for shipments and use testdb.SeedStore for sync tests"
```

---

### Task 14: `page` — savepoint the expected-to-fail duplicate-slug insert (1 failure)

Source: tail triage.

**Files:**
- Modify: `services/marketplace-api/internal/page/repository_integration_test.go:148` (`TestRepository_Create_DuplicateSlug_Errors`)

**Interfaces:**
- Consumes: nothing new — mirrors `category`'s existing `expectCreateFails` pattern at `internal/category/repository_integration_test.go:39-52`.
- Produces: nothing new.

The test runs three `repo.Create` calls inside one shared `testdb.NewTx` transaction and expects the
second (a real duplicate-slug constraint violation) to fail while the third succeeds. Postgres aborts
the whole transaction on any real SQL-level error until a `ROLLBACK` or a `SAVEPOINT` recovery; there
is none here, so the third statement fails with `25P02` regardless of what it is. This is the masking
effect the parent plan's Task 7 brief predicted, occurring within a single test instead of across
tests.

- [ ] **Step 1: Confirm the current failure**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/page/... -run 'TestRepository_Create_DuplicateSlug_Errors' -v 2>&1 | tee /tmp/t14-before.txt
grep -c 'current transaction is aborted' /tmp/t14-before.txt
```

Expected: `1`.

- [ ] **Step 2: Savepoint the expected-to-fail insert**

Wrap the second `repo.Create` call (the one asserted to error) with `SAVEPOINT` /
`ROLLBACK TO SAVEPOINT`, matching `category`'s `expectCreateFails`:

```go
require.NoError(t, tx.Exec("SAVEPOINT sp").Error)
err := repo.Create(context.Background(), &Page{
	TenantID: tenantID,
	StoreID:  storeID,
	Slug:     "dup-slug",
	Title:    "Second",
})
require.Error(t, err, "inserting duplicate (store_id, slug) should violate the unique index")
require.True(t, errors.Is(err, apperrors.ErrSlugTaken), "expected SlugTaken, got %v", err)
require.NoError(t, tx.Exec("ROLLBACK TO SAVEPOINT sp").Error)
```

Leave the first and third `repo.Create` calls untouched — only the one expected to fail needs the
savepoint.

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/page/... -run 'TestRepository_Create_DuplicateSlug_Errors' -v 2>&1 | tee /tmp/t14-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t14-after.txt
```

Expected: `--- PASS: TestRepository_Create_DuplicateSlug_Errors`. Do not widen `make test-int` for
`internal/page` — the package still has two PRODUCT-class failures (see the tail triage doc) that are
out of scope for this task.

- [ ] **Step 4: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test(page): savepoint the expected-to-fail duplicate-slug insert"
```

---

### Task 15: `product` — stop comparing against a transaction-frozen `now()` (1 failure)

Source: tail triage.

**Files:**
- Modify: `services/marketplace-api/internal/product/repository_integration_test.go:403` (`TestIntegration_ProductRepo_ListPublished_ExcludesDraftArchivedDeletedUnpublished`)

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing new.

Postgres freezes `now()` to the moment a transaction begins and holds that value for every statement
inside it. `testdb.NewTx` opens the transaction before the test does anything else; the test then
seeds a store and creates several aggregates before capturing `now := time.Now()` and using it as
`PublishedAt` — an app-clock value captured strictly after the transaction began. `ListPublished`'s
`WHERE ... published_at <= now()` evaluates Postgres's frozen `now()`, which is earlier than the
freshly-captured Go timestamp, so the row reads as "published in the future" and is filtered out.
Deterministic given any nonzero seeding time before the capture, not flaky.

- [ ] **Step 1: Confirm the current failure**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/product/... -run 'TestIntegration_ProductRepo_ListPublished_ExcludesDraftArchivedDeletedUnpublished' -v 2>&1 | tee /tmp/t15-before.txt
grep -c 'expected 1, got 0' /tmp/t15-before.txt
```

Expected: `1`.

- [ ] **Step 2: Backdate `PublishedAt`**

```go
now := time.Now().Add(-time.Hour)
```

An hour is comfortably clear of any realistic transaction duration while still passing every other
assertion in the test (the `d` aggregate's soft-delete check does not depend on the absolute value of
`now`, only that it is a valid past timestamp).

- [ ] **Step 3: Run and verify**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55433/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/product/... -run 'TestIntegration_ProductRepo_ListPublished_ExcludesDraftArchivedDeletedUnpublished' -v 2>&1 | tee /tmp/t15-after.txt
grep -E '^(---|ok|FAIL)' /tmp/t15-after.txt
```

Expected: `--- PASS`. Do not widen `make test-int` for `internal/product` — the package still has two
PRODUCT-class failures (soft-deleted variants leaking through `Preload`, and an apparently-unreachable
`OptionValueInUse` path) documented in the tail triage doc, out of scope for this task.

- [ ] **Step 4: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test(product): backdate PublishedAt to avoid the transaction-frozen now() gotcha"
```

---

## Definition of Done

- `comm -13 baseline-tests.txt after-tests.txt` is empty — this branch added zero failures.
- Clusters A, B and C contribute zero failures; 165 of the 187 baseline failures are gone.
- `make test-int` covers every package made green, and passes.
- Two issues filed and read back; `models.go` references the model one.
- Any remaining failure is either triaged in the tail doc with a named cause, or filed as its own issue. Nothing is left unexplained.
