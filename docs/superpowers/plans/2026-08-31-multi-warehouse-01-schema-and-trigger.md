# Multi-warehouse PR 1: schema and inventory trigger — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the schema multi-warehouse needs and fix `product_variants.inventory_quantity` to be the sum across locations rather than the last one written.

**Architecture:** Two migrations, additive only. Nothing in Go changes behaviour: while every store has one warehouse, a SUM over one row is that row, so this PR is a no-op in production and safe to deploy alone. It exists so that later PRs — the allocator, checkout, fulfilment, admin — have somewhere to write.

**Tech Stack:** Go 1.26, PostgreSQL 15, golang-migrate (embedded `MigrationsFS`), GORM, testify.

**Spec:** `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md`

## Global Constraints

- Work in the worktree `.claude/worktrees/177-multi-warehouse`, branch `feat/177-multi-warehouse`. Never switch the main checkout's branch.
- Run every Go command from `services/marketplace-api`, never path-scoped, always `-count=1`: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`. `go vet -tags=integration ./...` is the only command that compiles build-tagged files — include it.
- Integration tests: `//go:build integration`, gated on `TEST_DATABASE_URL` (**never** `TEST_DB_DSN`), run with `-p 1`.
- Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` (container `dev-postgres-1`; use the LAN IP, not localhost — a native Postgres owns 127.0.0.1:5432 on this machine).
- **A skipped integration test reads exactly like a passing one.** Any claim that an integration test passed must name the DSN it ran with. `./internal/whitelabel/lifecycle` takes 1.4s skipping versus 3.2s running — check the duration.
- Migrations are embedded. After adding a migration file, apply it before running integration tests: `DATABASE_URL=<dsn> go run ./cmd/migrate up`.
- `ExpectedSchemaVersion` in `services/marketplace-api/migrations.go` must equal the highest migration number on disk. `TestExpectedSchemaVersionMatchesHighestMigration` fails at CI time if not.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Every guard added must be mutation-tested: delete the guard, watch a test fail, restore it. A guard whose removal breaks nothing is decoration.

---

## File Structure

- **Create** `services/marketplace-api/migrations/000118_multi_warehouse_schema.up.sql` — `warehouses.priority`, `order_allocations`, `shipments.warehouse_id`
- **Create** `services/marketplace-api/migrations/000118_multi_warehouse_schema.down.sql`
- **Create** `services/marketplace-api/migrations/000119_variant_inventory_sum.up.sql` — trigger function → SUM, plus the AFTER DELETE arm
- **Create** `services/marketplace-api/migrations/000119_variant_inventory_sum.down.sql`
- **Modify** `services/marketplace-api/migrations.go` — `ExpectedSchemaVersion` 117 → 119
- **Create** `services/marketplace-api/internal/warehouse/allocations_schema_integration_test.go` — behavioural coverage for the 000118 schema
- **Create** `services/marketplace-api/internal/product/variant_inventory_sum_integration_test.go` — coverage for the 000119 trigger

Two migration files rather than one: the 000118 additions are inert, while 000119 changes the behaviour of a trigger every stock write goes through. Separating them means a revert of the risky half does not take the schema with it.

---

### Task 1: Schema additions (000118)

**Files:**
- Create: `services/marketplace-api/migrations/000118_multi_warehouse_schema.up.sql`
- Create: `services/marketplace-api/migrations/000118_multi_warehouse_schema.down.sql`
- Modify: `services/marketplace-api/migrations.go:17`
- Test: `services/marketplace-api/internal/warehouse/allocations_schema_integration_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: the `order_allocations` table, `warehouses.priority integer NOT NULL DEFAULT 0`, and `shipments.warehouse_id uuid NULL`. PR 2's allocator orders warehouses by `priority ASC, is_default DESC, created_at ASC`; PR 3 inserts `order_allocations` rows; PR 4 sets `shipments.warehouse_id` and stamps `order_allocations.shipment_id`.

- [ ] **Step 1: Write the migration**

Create `services/marketplace-api/migrations/000118_multi_warehouse_schema.up.sql`:

```sql
-- #177 multi-warehouse, schema half. Everything here is additive and inert:
-- no running code reads these columns or this table, and a store with one
-- warehouse behaves identically before and after. The allocator (PR 2) and
-- checkout (PR 3) are what give them meaning.

-- The order the allocator walks a store's warehouses. Merchant-ranked.
ALTER TABLE warehouses
    ADD COLUMN IF NOT EXISTS priority integer NOT NULL DEFAULT 0;

-- Which warehouse ships how much of which order line.
--
-- A separate table rather than a warehouse_id on order_items, because
-- refunds.order_item_id is a RESTRICT FK onto order_items: splitting a line
-- across warehouses by splitting its row would change what a refund points
-- at. Lines stay whole; allocation is its own concern with its own lifecycle.
CREATE TABLE IF NOT EXISTS order_allocations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    store_id      uuid NOT NULL,
    order_id      uuid NOT NULL REFERENCES orders(id)      ON DELETE CASCADE,
    order_item_id uuid NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
    warehouse_id  uuid NOT NULL REFERENCES warehouses(id)  ON DELETE RESTRICT,
    quantity      integer NOT NULL CHECK (quantity > 0),
    -- NULL until a label is printed. This is the flag that makes
    -- re-allocation safe before printing and refused after.
    shipment_id   uuid REFERENCES shipments(id)            ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- One row per (line, warehouse): a second allocation of the same line to
    -- the same warehouse is a bug, not a top-up.
    UNIQUE (order_item_id, warehouse_id)
);

CREATE INDEX IF NOT EXISTS order_allocations_order_idx
    ON order_allocations (order_id);
CREATE INDEX IF NOT EXISTS order_allocations_shipment_idx
    ON order_allocations (shipment_id);

-- Where a shipment actually shipped from. Nullable because every shipment
-- created before this migration has no honest answer; every one created
-- after it sets the column.
ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS warehouse_id uuid REFERENCES warehouses(id);
```

Create `services/marketplace-api/migrations/000118_multi_warehouse_schema.down.sql`:

```sql
-- Reversible without loss for the columns; order_allocations rows are lost,
-- which is correct — they only have meaning alongside the code that writes
-- them, and that code does not exist until PR 3.
DROP TABLE IF EXISTS order_allocations;

ALTER TABLE shipments  DROP COLUMN IF EXISTS warehouse_id;
ALTER TABLE warehouses DROP COLUMN IF EXISTS priority;
```

- [ ] **Step 2: Bump the schema version**

In `services/marketplace-api/migrations.go`, change:

```go
const ExpectedSchemaVersion uint = 117
```

to:

```go
const ExpectedSchemaVersion uint = 118
```

- [ ] **Step 3: Run the schema-version guard, expect PASS**

```bash
cd services/marketplace-api
go test . -count=1
```

Expected: PASS. If it fails with `ExpectedSchemaVersion = 118, but highest migration on disk is N`, the migration filename is wrong — it must be exactly `000118_multi_warehouse_schema.up.sql`.

- [ ] **Step 4: Write the failing test**

Create `services/marketplace-api/internal/warehouse/allocations_schema_integration_test.go`:

```go
//go:build integration

// Package warehouse_test — coverage for migration 000118, the schema half
// of #177's multi-warehouse work.
//
// These assert BEHAVIOUR (what the table refuses) rather than catalogue
// entries: a test that only checks a column exists passes against a table
// whose constraints were all dropped.
package warehouse_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedAllocatableOrder creates the store, warehouse, order and order line an
// order_allocations row needs, and returns their ids.
func seedAllocatableOrder(t *testing.T, db *gorm.DB) (tenantID, storeID, warehouseID, orderID, orderItemID string) {
	t.Helper()
	tenantID, storeID = seedStore(t, db)

	warehouseID = uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone)
		 VALUES (?, ?, ?, 'Alloc WH', '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		warehouseID, tenantID, storeID).Error)

	orderID = uuid.NewString()
	require.NoError(t, db.Exec(
		// customer_email, not email. idempotency_key is NOT NULL with no
		// default and is unique per store, so it gets a fresh uuid.
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 100.00, 100.00)`,
		orderID, tenantID, storeID, "AL-"+uuid.NewString()[:8], uuid.NewString()).Error)

	orderItemID = uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO order_items (id, order_id, title_snapshot, sku_snapshot, unit_price,
		                          quantity, line_total, currency_code)
		 VALUES (?, ?, 'Ink Tee', 'SKU-1', 50.00, 2, 100.00, 'INR')`,
		orderItemID, orderID).Error)

	return tenantID, storeID, warehouseID, orderID, orderItemID
}

func insertAllocation(db *gorm.DB, tenantID, storeID, orderID, orderItemID, warehouseID string, qty int) error {
	return db.Exec(
		`INSERT INTO order_allocations (tenant_id, store_id, order_id, order_item_id, warehouse_id, quantity)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tenantID, storeID, orderID, orderItemID, warehouseID, qty).Error
}

func TestOrderAllocations_AcceptsARowAndDefaultsShipmentToNull(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 2))

	var shipmentID *string
	require.NoError(t, db.Raw(
		`SELECT shipment_id FROM order_allocations WHERE order_item_id = ?`, orderItemID,
	).Row().Scan(&shipmentID))
	require.Nil(t, shipmentID, "an allocation is unshipped until a label is printed")
}

// A line allocated twice to the same warehouse is a bug, not a top-up: the
// allocator emits one row per (line, warehouse).
func TestOrderAllocations_RejectsASecondRowForTheSameLineAndWarehouse(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 1))
	err := insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 1)
	require.Error(t, err, "the (order_item_id, warehouse_id) unique key must refuse the duplicate")
}

func TestOrderAllocations_RejectsZeroQuantity(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	err := insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 0)
	require.Error(t, err, "an allocation of nothing is meaningless and must be refused")
}

// The FK is the backstop behind the repository's deletion rules (PR 5): a
// path that forgets them must fail loudly rather than orphan a parcel.
func TestOrderAllocations_WarehouseCannotBeDeletedWhileAllocated(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)
	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 2))

	err := db.Exec(`DELETE FROM warehouses WHERE id = ?`, warehouseID).Error
	require.Error(t, err, "ON DELETE RESTRICT must refuse while an allocation references the warehouse")
}

func TestWarehouses_PriorityDefaultsToZero(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := seedStore(t, db)

	warehouseID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name) VALUES (?, ?, ?, 'Prio WH')`,
		warehouseID, tenantID, storeID).Error)

	var priority int
	require.NoError(t, db.Raw(`SELECT priority FROM warehouses WHERE id = ?`, warehouseID).
		Row().Scan(&priority))
	require.Equal(t, 0, priority, "an unranked warehouse sorts with the rest, not ahead of them")
}
```

Note: `seedStore` already exists in this package, in `repository_integration_test.go:33`. Do not redefine it.

- [ ] **Step 5: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/warehouse/... -run 'TestOrderAllocations|TestWarehouses_Priority' -count=1 -p 1 -v
```

Expected: FAIL with `relation "order_allocations" does not exist` — the migration has not been applied to the test database yet. If instead every test reports `--- SKIP`, `TEST_DATABASE_URL` did not reach the process; fix that before continuing, because a skip is indistinguishable from a pass.

- [ ] **Step 6: Apply the migration**

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate up
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate version
```

Expected: `migrations applied`, then `version=118 dirty=false`.

- [ ] **Step 7: Run the test to verify it passes**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/warehouse/... -count=1 -p 1
```

Expected: `ok`. A run under ~0.3s means it skipped — check the DSN.

- [ ] **Step 8: Verify the down migration**

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate down 1
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate up
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate version
```

Expected: rollback succeeds, re-apply succeeds, `version=118 dirty=false`. A down that errors is a defect in this task, not a later one.

- [ ] **Step 9: Full verification**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

Expected: no output from `gofmt -l .`, no vet findings, all unit packages `ok`.

- [ ] **Step 10: Commit**

```bash
git add services/marketplace-api/migrations/000118_multi_warehouse_schema.up.sql \
        services/marketplace-api/migrations/000118_multi_warehouse_schema.down.sql \
        services/marketplace-api/migrations.go \
        services/marketplace-api/internal/warehouse/allocations_schema_integration_test.go
git commit -m "feat(shipping): add warehouse priority, order_allocations and shipments.warehouse_id (#177)"
```

---

### Task 2: `inventory_quantity` becomes a SUM across locations (000119)

**Files:**
- Create: `services/marketplace-api/migrations/000119_variant_inventory_sum.up.sql`
- Create: `services/marketplace-api/migrations/000119_variant_inventory_sum.down.sql`
- Modify: `services/marketplace-api/migrations.go:17`
- Test: `services/marketplace-api/internal/product/variant_inventory_sum_integration_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 — the two migrations are independent.
- Produces: the guarantee every later PR depends on, that
  `product_variants.inventory_quantity` equals `SUM(variant_stock.quantity)`
  for the variant. PR 3 relies on it for shopper-facing availability; PR 5
  relies on it so the product form's total stays right while a merchant edits
  one location.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/product/variant_inventory_sum_integration_test.go`:

```go
//go:build integration

// Package product_test — coverage for migration 000119.
//
// product_variants.inventory_quantity is what browse, PDP and cart read.
// Until 000119 the sync trigger assigned the LAST WRITTEN location's
// quantity, which is the total only while a variant has one stock row. The
// second warehouse would have made the storefront's stock number mean
// "whichever warehouse was touched most recently" — wrong, and silently so.
package product_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedVariantForSum creates the store/product/variant a stock row needs and
// returns the variant id. No variant_stock row is created: each test decides
// how many locations it wants.
func seedVariantForSum(t *testing.T, db *gorm.DB) string {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Sum Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "sum-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		// status='active' requires published_at (products_published_requires_active).
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Sum Test Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "sum-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return variantID
}

func addStock(t *testing.T, db *gorm.DB, variantID, locationID string, qty int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, locationID, qty).Error)
}

func inventoryQuantity(t *testing.T, db *gorm.DB, variantID string) int {
	t.Helper()
	var qty int
	require.NoError(t, db.Raw(
		`SELECT inventory_quantity FROM product_variants WHERE id = ?`, variantID,
	).Row().Scan(&qty))
	return qty
}

func TestInventorySync_SumsAcrossLocations(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)

	addStock(t, db, variantID, uuid.NewString(), 3)
	addStock(t, db, variantID, uuid.NewString(), 2)

	require.Equal(t, 5, inventoryQuantity(t, db, variantID),
		"the storefront's stock number must be the total across warehouses")
}

// The pre-000119 trigger assigned NEW.quantity, so writing the SMALLER
// location second would have reported 2 for a variant holding 5. This is the
// case that pins the fix rather than merely exercising it.
func TestInventorySync_UpdatingOneLocationKeepsTheTotal(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	locA, locB := uuid.NewString(), uuid.NewString()

	addStock(t, db, variantID, locA, 4)
	addStock(t, db, variantID, locB, 1)
	require.Equal(t, 5, inventoryQuantity(t, db, variantID))

	require.NoError(t, db.Exec(
		`UPDATE variant_stock SET quantity = 2 WHERE variant_id = ? AND location_id = ?`,
		variantID, locB).Error)

	require.Equal(t, 6, inventoryQuantity(t, db, variantID),
		"updating one location must re-sum, not overwrite the total with that location")
}

// Without an AFTER DELETE arm the trigger never fires on removal and
// inventory_quantity keeps counting stock in a warehouse that no longer has
// a row — an oversell that no test would otherwise catch.
func TestInventorySync_DeletingALocationLowersTheTotal(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	locA, locB := uuid.NewString(), uuid.NewString()

	addStock(t, db, variantID, locA, 4)
	addStock(t, db, variantID, locB, 3)
	require.Equal(t, 7, inventoryQuantity(t, db, variantID))

	require.NoError(t, db.Exec(
		`DELETE FROM variant_stock WHERE variant_id = ? AND location_id = ?`,
		variantID, locB).Error)

	require.Equal(t, 4, inventoryQuantity(t, db, variantID),
		"removing a location's stock must lower the total")
}

func TestInventorySync_LastLocationRemovedLeavesZeroNotNull(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	loc := uuid.NewString()

	addStock(t, db, variantID, loc, 6)
	require.NoError(t, db.Exec(
		`DELETE FROM variant_stock WHERE variant_id = ?`, variantID).Error)

	require.Equal(t, 0, inventoryQuantity(t, db, variantID),
		"SUM over no rows is NULL; the trigger must coalesce it to zero")
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/product/... -run TestInventorySync -count=1 -p 1 -v
```

Expected: `TestInventorySync_SumsAcrossLocations` FAILS with `expected: 5, actual: 2` — the old trigger assigned the last row written. `TestInventorySync_DeletingALocationLowersTheTotal` FAILS with `expected: 4, actual: 7`.

If they SKIP, the DSN did not reach the process. If they PASS, migration 000119 has already been applied to this database — check `go run ./cmd/migrate version` before assuming the code is right.

Note: `./internal/product/...` currently has one known pre-existing failure,
`TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected`
(`variant_matrix_mismatch`, tracked as #400). It is unrelated to this work —
do not fix it here, and do not let it mask these results. The `-run
TestInventorySync` filter above excludes it.

- [ ] **Step 3: Write the migration**

Create `services/marketplace-api/migrations/000119_variant_inventory_sum.up.sql`:

```sql
-- #177: product_variants.inventory_quantity becomes the SUM across a
-- variant's locations.
--
-- 000001 created this trigger with `SET inventory_quantity = NEW.quantity`
-- and a comment saying slice 2 would change it to a SUM. That assignment is
-- the total only while a variant has one stock row; with two warehouses the
-- number browse, PDP and cart all read would become whichever location was
-- written most recently.
--
-- The DELETE arm is new. Without it, removing a warehouse's stock row leaves
-- inventory_quantity counting units that no longer exist — an oversell with
-- no error anywhere.
--
-- NOTE for anyone editing this: in a DELETE trigger PL/pgSQL leaves NEW
-- unassigned, and referencing NEW.variant_id there raises "record new is not
-- assigned yet". Branch on TG_OP; do not reach for COALESCE(NEW, OLD).

CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
DECLARE
    v_variant uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_variant := OLD.variant_id;
    ELSE
        v_variant := NEW.variant_id;
    END IF;

    UPDATE product_variants
       SET inventory_quantity = COALESCE(
               (SELECT SUM(quantity) FROM variant_stock WHERE variant_id = v_variant), 0)
     WHERE id = v_variant;

    -- AFTER trigger: the return value is ignored.
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER variant_stock_sync_delete
    AFTER DELETE ON variant_stock
    FOR EACH ROW EXECUTE FUNCTION sync_variant_inventory();
```

Create `services/marketplace-api/migrations/000119_variant_inventory_sum.down.sql`:

```sql
-- Restores 000001's function verbatim and drops the DELETE arm. Safe while
-- every variant has one stock row, which is the only state a rollback to
-- this point can be in: nothing writes a second location until PR 5.
DROP TRIGGER IF EXISTS variant_stock_sync_delete ON variant_stock;

CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
BEGIN
    UPDATE product_variants
    SET inventory_quantity = NEW.quantity
    WHERE id = NEW.variant_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

- [ ] **Step 4: Bump the schema version**

In `services/marketplace-api/migrations.go`, change `ExpectedSchemaVersion` from `118` to `119`.

- [ ] **Step 5: Apply the migration and run the tests**

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate up
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/product/... -run TestInventorySync -count=1 -p 1 -v
```

Expected: all four `TestInventorySync_*` tests PASS.

- [ ] **Step 6: Mutation-test the DELETE arm**

The DELETE arm is the guard most likely to be quietly wrong, so prove a test catches its absence.

```bash
docker exec -i dev-postgres-1 psql -U dev -d marketplace_db -v ON_ERROR_STOP=1 \
  -c "DROP TRIGGER variant_stock_sync_delete ON variant_stock;"

cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/product/... -run TestInventorySync -count=1 -p 1
```

Expected: FAIL — `TestInventorySync_DeletingALocationLowersTheTotal` and
`TestInventorySync_LastLocationRemovedLeavesZeroNotNull` both fail. If they
pass, the tests are decoration and must be fixed before continuing.

Restore it:

```bash
docker exec -i dev-postgres-1 psql -U dev -d marketplace_db -v ON_ERROR_STOP=1 \
  -c "CREATE TRIGGER variant_stock_sync_delete AFTER DELETE ON variant_stock FOR EACH ROW EXECUTE FUNCTION sync_variant_inventory();"

cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/product/... -run TestInventorySync -count=1 -p 1
```

Expected: `ok` again.

- [ ] **Step 7: Verify the down migration**

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate down 1
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate up
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go run ./cmd/migrate version
```

Expected: `version=119 dirty=false`.

- [ ] **Step 8: Run the affected integration packages**

The trigger sits under every stock write, so the packages that write stock must be re-run — this is the blast radius of the change.

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 \
  ./internal/stockhold/... ./internal/warehouse/... ./internal/handlers/storefront/...
```

Expected: all `ok`. Durations should be seconds, not milliseconds.

- [ ] **Step 9: Full verification**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

Expected: clean.

- [ ] **Step 10: Commit**

```bash
git add services/marketplace-api/migrations/000119_variant_inventory_sum.up.sql \
        services/marketplace-api/migrations/000119_variant_inventory_sum.down.sql \
        services/marketplace-api/migrations.go \
        services/marketplace-api/internal/product/variant_inventory_sum_integration_test.go
git commit -m "fix(product): sum variant stock across locations instead of taking the last write (#177)"
```

---

## Done when

- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` all clean
- `go test ./... -count=1` green, including `TestExpectedSchemaVersionMatchesHighestMigration` at 119
- Integration packages `internal/warehouse`, `internal/product` (`-run TestInventorySync`), `internal/stockhold`, `internal/handlers/storefront` green against the real DSN, with durations that prove they ran
- Both down migrations exercised
- The DELETE arm mutation-tested: removed, tests failed, restored, tests passed

## Explicitly NOT in this PR

- `internal/allocation` and the allocator (PR 2)
- Any change to `commitStock`, `cart_holds.go`, or anything that writes `order_allocations` (PR 3)
- Shipment-per-warehouse, `fulfillment_status = 'partial'`, the order-detail `Shipment` field becoming a list (PR 4)
- Warehouse CRUD, the carrier-config picker, per-location stock editing (PR 5)
- The sentinel backfill and deleting `DefaultLocationID` — that is PR 6 and must not land until PRs 1–5 are deployed on admin, storefront and both CronJobs
- Fixing `TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected` (#400), which fails before this PR and after it
