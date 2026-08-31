# Multi-warehouse PR 3: checkout allocation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the allocator into checkout so an order is allocated across warehouses when it is placed, writing `order_allocations`, without changing behaviour for any store that has no warehouses.

**Architecture:** A new availability snapshot loader, then `commitStock` gains an allocation path. The binding allocation happens at order placement, inside the order transaction, where `order_items` rows already exist. Cart-time holds stay provisional. Every change is gated on the store actually having warehouse rows — which no store has today, so this PR is a no-op in production until PR 5 lets merchants create them.

**Tech Stack:** Go 1.26, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md` (see "Checkout", "The allocator", and "The sentinel backfill, and why it is two deploys")

## Global Constraints

- Work in the worktree `.claude/worktrees/177-checkout`, branch `feat/177-checkout-allocation`. Never switch the main checkout's branch.
- Run every Go command from `services/marketplace-api`, never path-scoped, always `-count=1`: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `//go:build integration`, gated on `TEST_DATABASE_URL` (**never** `TEST_DB_DSN`), run with `-p 1`. A skip prints `ok` and reads like a pass — every claim must name the DSN and the duration.
- Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` (container `dev-postgres-1`; the LAN IP, not localhost — a native Postgres owns 127.0.0.1:5432 on this machine).
- Commits: conventional, single line, no signature, no `Co-Authored-By`, no emoji. Stage explicit paths; never `git add -A`.
- Every guard added must be mutation-tested: delete it, watch a test fail, restore it.
- Never mutate an input.

## The four facts this plan is built on

Each was verified against the code or the live database. Do not re-derive them; do not contradict them without saying so.

**1. Production has ZERO warehouse rows.** Measured on the live database: `warehouses` = 0 rows, `variant_stock` = 380 rows all at the sentinel `00000000-0000-0000-0000-000000000001`, across one store. Warehouses are only created when a carrier config is saved, and no merchant has done that.

  Therefore: **a store with no warehouses must keep behaving exactly as it does today.** If allocation ran unconditionally, `Plan` would return `ErrNoWarehouse` and every checkout in production would fail. The legacy path is not a fallback for tidiness — it is the only path that currently executes.

**2. `Hold` REPLACES a quantity, it does not add to it.** `internal/stockhold/repository.go`: `ON CONFLICT (cart_token, variant_id, location_id) DO UPDATE SET qty = EXCLUDED.qty`. Two assignments for the same `(variant, warehouse)` — which happens whenever two order lines carry the same variant — would leave only the second quantity reserved.

  Therefore: **aggregate assignments by `(variant, storage location)` before calling `Hold`, summing quantities.** `order_allocations` rows stay per line; only the holds are aggregated.

**3. `WithinTx` runs AFTER the order items are inserted.** `internal/order/service.go`: `CreateInTx(tx, o, in.Items, …)` at ~line 197, then `in.WithinTx(tx, o)` at ~line 211. So inside `commitStock` the `order_items` rows exist and can be read back.

  Therefore: build the allocation lines by querying `order_items` for the order, so each line carries its `order_item_id`. Do not thread ids through `CheckoutItemRequest`.

**4. Stock lives at the SENTINEL, not at warehouse ids, until PR 6 backfills it.** A store can have a `warehouses` row whose id appears nowhere in `variant_stock`.

  Therefore the availability snapshot must carry, for each `(variant, warehouse)` pair, BOTH the units available AND the **storage location id** the units are actually recorded under. Allocation reasons in warehouse-id space; holds and decrements must use the storage location id. After PR 6 the two coincide and nothing changes.

---

## File Structure

- **Create** `services/marketplace-api/internal/handlers/storefront/checkout_availability.go` — the snapshot loader
- **Create** `services/marketplace-api/internal/handlers/storefront/checkout_availability_integration_test.go`
- **Modify** `services/marketplace-api/internal/handlers/storefront/checkout_stock.go` — `commitStock` gains the allocation path
- **Create** `services/marketplace-api/internal/handlers/storefront/checkout_allocation_integration_test.go`

`cart_holds.go` is deliberately NOT changed in this PR — see "Explicitly NOT in this PR".

---

### Task 1: The availability snapshot

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/checkout_availability.go`
- Test: `services/marketplace-api/internal/handlers/storefront/checkout_availability_integration_test.go`

**Interfaces:**
- Consumes: `allocation.Availability` from `internal/allocation` (PR 2, merged).
- Produces:

```go
// stockAt is units of a variant available at ONE physical storage location.
type stockAt struct {
    LocationID string
    Units      int
}

// storageLocations records, per variant and warehouse, where the units
// backing that warehouse's availability actually sit — possibly in more than
// one place while sentinel and real rows coexist. Sorted by LocationID.
type storageLocations map[string]map[string][]stockAt

func loadAvailability(
    ctx context.Context, tx *gorm.DB, cartToken string,
    warehouses []allocation.Warehouse, variantIDs []string,
) (allocation.Availability, storageLocations, error)
```

**Note (amended after Task 1's review):** this returns a per-location
BREAKDOWN, not a single location. A warehouse's availability can be backed by
units in two places during the transition — PR 5's per-location stock editing
writes to a real warehouse id while the sentinel row still exists. An
assignment may therefore need MORE THAN ONE hold.

  Task 2 calls this, passes the `Availability` to `allocation.Plan`, and uses `storageLocations` to decide which `location_id` each `Hold` targets.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/handlers/storefront/checkout_availability_integration_test.go`:

```go
//go:build integration

// Package storefront — coverage for the multi-warehouse availability
// snapshot (#177 PR 3).
//
// The snapshot is what allocation reasons over. Two things about it are
// easy to get wrong and expensive to get wrong:
//
//   - It must EXCLUDE the calling cart's own live holds, matching
//     stockhold.Hold and stockhold.Available. A cart asking what it can have
//     must not be told its own reservation is competition.
//   - Until PR 6's backfill, units are stored at the sentinel location, not
//     at any warehouse id. The snapshot therefore reports availability
//     against a WAREHOUSE while remembering the STORAGE location the units
//     actually sit at, because that is where a hold has to be placed.
package storefront

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedAvailStore creates a store, a product and one variant, and returns
// (storeID, variantID).
func seedAvailStore(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Avail Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "avail-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Avail Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "avail-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return storeID, variantID
}

func seedWarehouseRow(t *testing.T, db *gorm.DB, storeID, name string) string {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone)
		 VALUES (?, ?, ?, ?, '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		id, tenantID, storeID, name).Error)
	return id
}

func TestLoadAvailability_SentinelStockIsReportedAgainstTheFirstWarehouse(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")

	// Stock still lives at the sentinel — PR 6 has not backfilled yet.
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 7, now())`, variantID, product.DefaultLocationID).Error)

	warehouses := []allocation.Warehouse{{ID: whID}}
	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(), warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 7, avail.At(variantID, whID),
		"sentinel-stored units must be visible against the store's warehouse before the backfill")
	require.Equal(t, product.DefaultLocationID, storage[variantID][whID],
		"a hold must target the location the units are actually stored at")
}

func TestLoadAvailability_RealWarehouseStockIsReportedAgainstItself(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")

	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, whA, variantID, whB).Error)

	warehouses := []allocation.Warehouse{{ID: whA}, {ID: whB}}
	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(), warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 3, avail.At(variantID, whA))
	require.Equal(t, 4, avail.At(variantID, whB))
	require.Equal(t, whA, storage[variantID][whA], "post-backfill the storage location IS the warehouse")
	require.Equal(t, whB, storage[variantID][whB])
}

// Matching stockhold.Hold and stockhold.Available: a cart must not see its
// own reservation as competition, or it could never re-hold what it already
// has and checkout would refuse a cart it had itself reserved.
func TestLoadAvailability_ExcludesTheCallingCartsOwnHolds(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 10, now())`, variantID, product.DefaultLocationID).Error)

	mine, theirs := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, 4, ?, 'held'), (?, ?, ?, 3, ?, 'held')`,
		variantID, product.DefaultLocationID, mine, time.Now().Add(time.Hour),
		variantID, product.DefaultLocationID, theirs, time.Now().Add(time.Hour)).Error)

	warehouses := []allocation.Warehouse{{ID: whID}}
	avail, _, err := loadAvailability(context.Background(), db, mine, warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 7, avail.At(variantID, whID),
		"10 units less the OTHER cart's 3; this cart's own 4 must not count against it")
}

func TestLoadAvailability_ExpiredHoldsDoNotReduceAvailability(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, product.DefaultLocationID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, 5, ?, 'held')`,
		variantID, product.DefaultLocationID, uuid.NewString(), time.Now().Add(-time.Minute)).Error)

	avail, _, err := loadAvailability(context.Background(), db, uuid.NewString(),
		[]allocation.Warehouse{{ID: whID}}, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 5, avail.At(variantID, whID),
		"a hold expires by the clock, not by a sweeper running")
}

// A variant with no stock row anywhere must simply be absent, not zero-filled
// — allocation treats a missing entry as zero, and inventing rows would hide
// the difference between "none here" and "no such pairing".
func TestLoadAvailability_VariantWithNoStockRowsIsAbsent(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")

	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(),
		[]allocation.Warehouse{{ID: whID}}, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 0, avail.At(variantID, whID))
	require.Empty(t, storage[variantID])
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run TestLoadAvailability -count=1 -p 1
```

Expected: FAIL to compile — `undefined: loadAvailability`. If instead every test SKIPs, `TEST_DATABASE_URL` did not reach the process; fix that first, because a skip is indistinguishable from a pass.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/handlers/storefront/checkout_availability.go`:

```go
package storefront

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// storageLocations records, per variant and warehouse, the location_id the
// units are actually stored under.
//
// It exists because of the expand phase: until PR 6 backfills
// variant_stock.location_id, units sit at product.DefaultLocationID while
// allocation reasons about real warehouse ids. A hold must be placed against
// the location the row actually has, or it locks nothing. After the backfill
// the two values coincide and this map becomes an identity.
type storageLocations map[string]map[string]string

// loadAvailability builds the snapshot allocation.Plan reasons over.
//
// Availability is variant_stock.quantity minus OTHER carts' live holds — the
// same arithmetic stockhold.Available uses, and excluding the calling cart's
// own holds for the same reason: a cart must not be told its own reservation
// is competition.
//
// This is a READ. It takes no locks, and it is not what makes the decision
// safe: stockhold.Hold re-checks under SELECT ... FOR UPDATE, and that is the
// check that counts. A snapshot that is stale by the time it is used simply
// produces a plan whose holds fail, which is the same outcome a
// single-warehouse shopper gets today.
func loadAvailability(
	ctx context.Context,
	tx *gorm.DB,
	cartToken string,
	warehouses []allocation.Warehouse,
	variantIDs []string,
) (allocation.Availability, storageLocations, error) {
	avail := allocation.Availability{}
	storage := storageLocations{}
	if len(warehouses) == 0 || len(variantIDs) == 0 {
		return avail, storage, nil
	}

	var rows []struct {
		VariantID  string
		LocationID string
		Available  int
	}
	err := tx.WithContext(ctx).Raw(
		`SELECT vs.variant_id,
		        vs.location_id,
		        vs.quantity - COALESCE((
		            SELECT SUM(sh.qty) FROM stock_holds sh
		             WHERE sh.variant_id = vs.variant_id
		               AND sh.location_id = vs.location_id
		               AND sh.state = 'held'
		               AND sh.expires_at > now()
		               AND sh.cart_token <> ?), 0) AS available
		   FROM variant_stock vs
		  WHERE vs.variant_id IN ?`, cartToken, variantIDs).Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("storefront: load availability: %w", err)
	}

	// Which warehouse a stock row belongs to. Sentinel rows have no real
	// warehouse of their own, so they answer for the store's FIRST warehouse
	// in fill order — the one a single-warehouse store ships everything from
	// anyway. Real rows answer for themselves.
	byWarehouse := make(map[string]string, len(warehouses))
	for _, w := range warehouses {
		byWarehouse[w.ID] = w.ID
	}

	for _, r := range rows {
		warehouseID := r.LocationID
		if r.LocationID == product.DefaultLocationID {
			warehouseID = warehouses[0].ID
		} else if _, known := byWarehouse[r.LocationID]; !known {
			// Stock at a location that is not one of this store's warehouses
			// — another store's row cannot appear here (variantIDs are this
			// order's), so this is a warehouse deleted out from under its
			// stock. Skip it rather than allocate from somewhere that no
			// longer exists.
			continue
		}

		if r.Available < 0 {
			// Holds can exceed stock if a merchant lowered the count while
			// reservations were live. Allocation clamps too, but reporting a
			// negative here would be nonsense on the way in.
			r.Available = 0
		}

		if avail[r.VariantID] == nil {
			avail[r.VariantID] = map[string]int{}
			storage[r.VariantID] = map[string]string{}
		}
		avail[r.VariantID][warehouseID] += r.Available
		storage[r.VariantID][warehouseID] = r.LocationID
	}

	return avail, storage, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run TestLoadAvailability -count=1 -p 1 -v
```

Expected: all five PASS, with durations in the tens of milliseconds each (not zero — a zero-duration run means it skipped).

- [ ] **Step 5: Mutation-test the own-cart exclusion**

Change `AND sh.cart_token <> ?` to `AND sh.cart_token = ?`, re-run.

Expected: FAIL — `TestLoadAvailability_ExcludesTheCallingCartsOwnHolds` reports 6 rather than 7. Restore and confirm green. If it still passes, the test is not pinning the exclusion.

- [ ] **Step 6: Verify and commit**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

```bash
git add services/marketplace-api/internal/handlers/storefront/checkout_availability.go \
        services/marketplace-api/internal/handlers/storefront/checkout_availability_integration_test.go
git commit -m "feat(storefront): per-warehouse availability snapshot for checkout allocation (#177)"
```

---

### Task 2: Allocate at order placement

**Files:**
- Modify: `services/marketplace-api/internal/handlers/storefront/checkout_stock.go`
- Test: `services/marketplace-api/internal/handlers/storefront/checkout_allocation_integration_test.go`

**Interfaces:**
- Consumes: `loadAvailability` and `storageLocations` from Task 1; `allocation.InPriorityOrder`, `allocation.Plan`, `allocation.CannotFillError` from `internal/allocation`.
- Produces: `commitStock` writes `order_allocations` rows for stores that have warehouses. PR 4 groups those rows by warehouse to create one shipment each.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/handlers/storefront/checkout_allocation_integration_test.go`. It exercises `commitStock` directly rather than through an HTTP checkout, because the property under test is what lands in the database, and driving a full checkout would need payment and address fixtures irrelevant to it.

```go
//go:build integration

// Package storefront — coverage for allocation at order placement (#177 PR 3).
//
// The load-bearing case is the FIRST one: a store with no warehouses must
// behave exactly as it did before this PR. That is not a tidy fallback —
// production has zero warehouse rows, so it is the only path that currently
// runs, and a regression there breaks every checkout.
package storefront

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedOrderWithItems creates an order and one order_item per line and
// returns (orderID, []orderItemID).
func seedOrderWithItems(t *testing.T, db *gorm.DB, storeID string, lines []stockLine) (string, []string) {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))

	orderID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 10.00, 10.00)`,
		orderID, tenantID, storeID, "AL-"+uuid.NewString()[:8], uuid.NewString()).Error)

	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		itemID := uuid.NewString()
		require.NoError(t, db.Exec(
			`INSERT INTO order_items (id, order_id, variant_id, title_snapshot, sku_snapshot,
			                          unit_price, quantity, line_total, currency_code)
			 VALUES (?, ?, ?, 'Item', 'SKU', 10.00, ?, 10.00, 'INR')`,
			itemID, orderID, l.VariantID, l.Quantity).Error)
		ids = append(ids, itemID)
	}
	return orderID, ids
}

func stockAt(t *testing.T, db *gorm.DB, variantID, locationID string) int {
	t.Helper()
	var q int
	require.NoError(t, db.Raw(
		`SELECT COALESCE((SELECT quantity FROM variant_stock WHERE variant_id = ? AND location_id = ?), -1)`,
		variantID, locationID).Row().Scan(&q))
	return q
}

// THE load-bearing test. Production has zero warehouses; if this regresses,
// every checkout fails.
func TestCommitStock_StoreWithNoWarehousesBehavesExactlyAsBefore(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, product.DefaultLocationID).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), cart, orderID, storeID, lines)
	}))

	require.Equal(t, 3, stockAt(t, db, variantID, product.DefaultLocationID),
		"the sentinel row must still be the one decremented")

	var allocations int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM order_allocations WHERE order_id = ?`, orderID).Scan(&allocations).Error)
	require.Zero(t, allocations,
		"a store with no warehouses has nothing to allocate against — order_allocations.warehouse_id is NOT NULL")
}

func TestCommitStock_AllocatesAcrossWarehousesAndRecordsThem(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0 WHERE id = ?`, whA).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 1 WHERE id = ?`, whB).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, whA, variantID, whB).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	var got []struct {
		WarehouseID string
		Quantity    int
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, quantity FROM order_allocations
		  WHERE order_item_id = ? ORDER BY quantity DESC`, itemIDs[0]).Scan(&got).Error)
	require.Len(t, got, 2, "5 units from a 3+4 split must record two allocations")
	require.Equal(t, whA, got[0].WarehouseID)
	require.Equal(t, 3, got[0].Quantity)
	require.Equal(t, whB, got[1].WarehouseID)
	require.Equal(t, 2, got[1].Quantity)

	require.Equal(t, 0, stockAt(t, db, variantID, whA), "the higher-priority warehouse is drained first")
	require.Equal(t, 2, stockAt(t, db, variantID, whB))
}

// Two order lines carrying the SAME variant. stockLinesFromItems does not
// merge by variant, and stock_holds' ON CONFLICT REPLACES a quantity rather
// than adding to it — so holding per assignment without aggregating would
// reserve only the second line's units and oversell the first.
func TestCommitStock_TwoLinesOfOneVariantDoNotUnderHold(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 6, now())`, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}, {VariantID: variantID, Quantity: 3}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	require.Equal(t, 1, stockAt(t, db, variantID, whA),
		"6 units less 2 and 3 is 1 — under-holding would have left 4 or 3 here")

	for i, itemID := range itemIDs {
		var q int
		require.NoError(t, db.Raw(
			`SELECT quantity FROM order_allocations WHERE order_item_id = ?`, itemID).Row().Scan(&q))
		require.Equal(t, lines[i].Quantity, q, "each line records its own allocation")
	}
}

func TestCommitStock_UnfillableOrderFailsAndTakesNoStock(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 2, now())`, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 9}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	err := db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	})
	require.Error(t, err, "an order no combination of warehouses can fill must fail the transaction")

	require.Equal(t, 2, stockAt(t, db, variantID, whA), "a failed checkout must move no stock")
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run TestCommitStock -count=1 -p 1
```

Expected: FAIL to compile — `commitStock` does not yet take `orderID` and `storeID`.

- [ ] **Step 3: Change `commitStock`**

In `services/marketplace-api/internal/handlers/storefront/checkout_stock.go`:

1. Widen the signature to `commitStock(ctx context.Context, tx *gorm.DB, holds *stockhold.Repository, cartToken, orderID, storeID string, lines []stockLine) error`, and update its one caller at `internal/handlers/storefront/checkout_ext.go` (search for `commitStock(`) to pass `o.ID` and the store id it already has in scope.

2. After the existing `inventoryPolicies` lookup, load the store's warehouses in fill order:

```go
	// A store with NO warehouses keeps the pre-#177 behaviour exactly:
	// hold and decrement at the sentinel location, write no allocations.
	// This is not a tidy fallback — production has zero warehouse rows, so
	// it is the only path that currently runs, and allocation.Plan would
	// return ErrNoWarehouse for every checkout if it ran unconditionally.
	warehouses, err := storeWarehousesInFillOrder(ctx, tx, storeID)
	if err != nil {
		return err
	}
	if len(warehouses) == 0 {
		return commitStockAtSentinel(ctx, tx, holds, cartToken, lines, policies)
	}
```

   Move the existing per-line loop body verbatim into `commitStockAtSentinel` so the legacy path is provably unchanged rather than re-implemented.

3. Add `storeWarehousesInFillOrder`, which reads the store's warehouses and returns `allocation.InPriorityOrder(...)` of them. `Plan` takes an ordered slice and cannot detect an unordered one, so ordering here is mandatory.

4. Add the allocation path:

```go
	avail, storage, err := loadAvailability(ctx, tx, cartToken, warehouses, variantIDsOf(lines))
	if err != nil {
		return err
	}

	allocLines := make([]allocation.Line, 0, len(lines))
	for _, l := range lines {
		allocLines = append(allocLines, allocation.Line{
			VariantID:     l.VariantID,
			Quantity:      l.Quantity,
			SellsPastZero: policies[l.VariantID] == inventoryPolicyContinue,
		})
	}

	assignments, err := allocation.Plan(warehouses, avail, allocLines)
	var cannot allocation.CannotFillError
	if errors.As(err, &cannot) {
		// Same shape the shopper already gets for a sold-out cart.
		return outOfStockError{VariantID: cannot.VariantID}
	}
	if err != nil {
		return err
	}
```

5. Place the holds, **aggregated by `(variant, storage location)`**:

```go
	// stockhold.Hold's ON CONFLICT DO UPDATE SET qty = EXCLUDED.qty REPLACES
	// a quantity rather than adding to it, and its unique key is
	// (cart_token, variant_id, location_id). Two assignments for the same
	// variant and warehouse — which happens whenever two order lines carry
	// the same variant, because stockLinesFromItems does not merge them —
	// would leave only the SECOND quantity reserved. Aggregate first.
	type holdKey struct{ variantID, locationID string }
	totals := map[holdKey]int{}
	for _, a := range assignments {
		if policies[a.VariantID] == inventoryPolicyContinue {
			continue // sold past zero on purpose; decremented, not held
		}
		// A warehouse's units can sit in more than one physical location
		// while the sentinel and real rows coexist, so draw the assigned
		// quantity from that warehouse's locations in order until it is
		// covered. The breakdown is sorted by LocationID, so the same
		// inputs always produce the same holds.
		want := a.Quantity
		for _, at := range storage[a.VariantID][a.WarehouseID] {
			if want == 0 {
				break
			}
			take := min(want, at.Units)
			if take <= 0 {
				continue
			}
			totals[holdKey{a.VariantID, at.LocationID}] += take
			want -= take
		}
		if want > 0 {
			// The snapshot said this warehouse had the units and the
			// breakdown does not account for them. That is a bug in the
			// snapshot, not a stock shortage — fail loudly rather than
			// under-hold and oversell.
			return fmt.Errorf(
				"storefront: allocation for variant %s at warehouse %s is %d units short of its storage breakdown",
				a.VariantID, a.WarehouseID, want)
		}
	}
	for k, qty := range totals {
		err := holds.Hold(ctx, tx, cartToken, k.variantID, k.locationID, qty, HoldTTL)
		if errors.Is(err, stockhold.ErrInsufficientStock) {
			return outOfStockError{VariantID: k.variantID}
		}
		if err != nil {
			return err
		}
	}
```

   Handle `continue`-policy variants with the same clamped decrement the legacy path uses, at their assigned warehouse's storage location.

6. Write `order_allocations`, one row per assignment, mapped to the order's items:

```go
	// order_items rows already exist: order.CreateInput.WithinTx runs after
	// CreateInTx inserts them. Reading them back is how an allocation gets
	// its order_item_id without threading ids through the checkout request.
	if err := recordAllocations(ctx, tx, orderID, assignments, lines); err != nil {
		return err
	}
```

   `recordAllocations` reads `SELECT id, variant_id, quantity FROM order_items WHERE order_id = ? ORDER BY created_at, id`, pairs them positionally with `lines` (same order, same length — assert it and fail loudly if not), and inserts one row per assignment carrying `tenant_id`, `store_id`, `order_id`, `order_item_id`, `warehouse_id`, `quantity`.

7. Finish with the existing `holds.Commit(ctx, tx, cartToken)` — unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run TestCommitStock -count=1 -p 1 -v
```

Expected: all four PASS.

- [ ] **Step 4b: Add a test for an assignment spanning two storage locations**

This is the case Task 1's review added the breakdown for, and nothing in the
tests above reaches it. In `checkout_allocation_integration_test.go`:

```go
// A warehouse whose units sit in two places — the sentinel row that predates
// the backfill, plus a real row written by per-location stock editing. One
// assignment against that warehouse must produce a hold in EACH location,
// adding up to the assignment.
func TestCommitStock_AssignmentSpanningTwoStorageLocationsHoldsInBoth(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, product.DefaultLocationID, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	// 5 units drawn from a 3 + 4 breakdown: the sentinel is exhausted and the
	// remainder comes from the real row.
	require.Equal(t, 0, stockAt(t, db, variantID, product.DefaultLocationID))
	require.Equal(t, 2, stockAt(t, db, variantID, whA))
}
```

Run it, expect PASS.

- [ ] **Step 5: Mutation-test the hold aggregation**

Replace the `totals` aggregation with a direct `Hold` per assignment, re-run.

Expected: FAIL — `TestCommitStock_TwoLinesOfOneVariantDoNotUnderHold` finds 4 units left rather than 1, because the second `Hold` replaced the first rather than adding to it. That failure IS the oversell this aggregation exists to prevent. Restore and confirm green.

- [ ] **Step 6: Mutation-test the no-warehouse guard**

Delete the `if len(warehouses) == 0 { … }` early return, re-run.

Expected: FAIL — `TestCommitStock_StoreWithNoWarehousesBehavesExactlyAsBefore` errors with `allocation: store has no warehouses`. That is the production-breaking regression this guard exists to prevent. Restore and confirm green.

- [ ] **Step 7: Full verification**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/storefront/... ./internal/stockhold/... ./internal/order/... ./internal/warehouse/...
```

Expected: all `ok`, durations in seconds.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/checkout_stock.go \
        services/marketplace-api/internal/handlers/storefront/checkout_ext.go \
        services/marketplace-api/internal/handlers/storefront/checkout_allocation_integration_test.go
git commit -m "feat(storefront): allocate an order across warehouses at placement (#177)"
```

---

## Done when

- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` clean
- `go test ./... -count=1` green
- The storefront, stockhold, order and warehouse integration packages green against the real DSN, with durations that prove they ran
- Three guards mutation-tested: the own-cart hold exclusion, the hold aggregation, and the no-warehouse early return

## Explicitly NOT in this PR

- **`cart_holds.go` stays on the sentinel.** The spec calls for provisional per-location holds at cart time, but with zero warehouses in production that path allocates nothing today, and changing it would double this PR's surface for no behaviour change. It moves in its own PR once a merchant can actually create a second warehouse (PR 5).
- Shipment-per-warehouse, `fulfillment_status = 'partial'`, the order-detail `Shipment` field becoming a list (PR 4)
- Warehouse CRUD and per-location stock editing (PR 5)
- The sentinel backfill and deleting `DefaultLocationID` (PR 6) — and note this PR is what makes that backfill safe, because it reads whatever location a stock row actually carries rather than assuming one
- Re-planning when a hold is lost to a race: the spec defers it until the race is observed
