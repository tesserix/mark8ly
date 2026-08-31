# Multi-warehouse PR 4b: a shipment per warehouse — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Label creation produces one shipment per contributing warehouse, each shipping from its own address with its own parcel contents, and an order whose parcels are only partly shipped reports `fulfillment_status = 'partial'`.

**Architecture:** `ShipmentsHandler.Create` groups the order's `order_allocations` by warehouse and creates a shipment per group. An order with **no** allocations — every order placed before PR 3, and every order on a store with no warehouse — takes the existing single-shipment path unchanged.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md` (see "Fulfilment")

## Global Constraints

- Work in the worktree `.claude/worktrees/177-fulfilment-4b`, branch `feat/177-shipment-per-warehouse`. Never switch the main checkout's branch.
- Run every Go command from `services/marketplace-api`, never path-scoped, always `-count=1`: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `//go:build integration`, gated on `TEST_DATABASE_URL` (**never** `TEST_DB_DSN`), run with `-p 1`. A skip prints `ok` and reads like a pass — name the DSN and the duration in every claim.
- Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` (container `dev-postgres-1`; the LAN IP, not localhost).
- Commits: conventional, single line, no signature, no `Co-Authored-By`, no emoji. Stage explicit paths; never `git add -A`.
- Every guard added must be mutation-tested.
- Never mutate an input.

## The facts this plan is built on

Verified in the code and against production. Do not re-derive; do not contradict without saying so.

**1. Every existing order has ZERO allocations.** `order_allocations` is empty in production. Allocation only began with PR 3 (deployed today), and no order has been placed since. So the no-allocations path is the only one that currently executes, and a regression there breaks label creation for every real order — including `the-bondi-store`'s seven.

**2. `Create` currently makes one shipment for the whole order.** `CreateShipmentRequest` is `{provider, service}`; parcel contents come from all of the order's items; the pickup address comes from the carrier config's warehouse via `resolvePickupAddress`.

**3. The dispatched-email path is already per-shipment and deduped.** `dispatchShipmentDispatchedEmail` gates on `shipments.dispatched_email_sent_at IS NULL`, so N shipments send N emails without extra work. Only the copy does not name which parcel, and that template lives outside this service — out of scope here.

**4. `fulfillment_status` already permits `'partial'`.** The CHECK allows `unfulfilled | partial | fulfilled` and `internal/order/status.go` defines the value with a legal `unfulfilled → partial → fulfilled` transition. It has simply never been written.

**5. Carrier calls are external and not transactional.** A label that succeeds cannot be rolled back. The design consequence is in Task 1: create shipments group by group, persisting each as it succeeds, so a later failure leaves the earlier parcels intact and retryable rather than orphaning a label at the carrier.

---

## File Structure

- **Modify** `services/marketplace-api/internal/handlers/admin/shipments.go` — group by warehouse, one shipment per group
- **Create** `services/marketplace-api/internal/handlers/admin/shipments_per_warehouse_integration_test.go`
- **Modify** `services/marketplace-api/internal/order/service.go` — a `MarkPartiallyFulfilled` transition
- **Create** `services/marketplace-api/internal/order/partial_fulfilment_integration_test.go`

---

### Task 1: One shipment per warehouse group

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/shipments.go`
- Test: `services/marketplace-api/internal/handlers/admin/shipments_per_warehouse_integration_test.go`

**Interfaces:**
- Consumes: `order_allocations` (PR 1 schema, written by PR 3), `warehouse.Repository.ByID`.
- Produces: one `shipments` row per contributing warehouse with `warehouse_id` set, and `order_allocations.shipment_id` stamped. Task 2 reads those rows to decide `partial` versus `fulfilled`.

- [ ] **Step 0: Add the carrier seam `Create` needs to be testable**

`Create` calls `shipping.NewCarrier(provider, apiKey, secretKey, cfg.Mode)`
directly, so it cannot be exercised without a live carrier. The existing
`carrierFactory` is NOT the seam to use — its doc comment scopes it to the
tracking-sync loop deliberately, and its signature (`func(provider string, sh
any)`) is shaped for that loop's projection.

Add a separate, nil-safe override mirroring how `labelMailer` and
`carrierFactory` are already attached in this file:

```go
	// newCarrier overrides shipping.NewCarrier inside Create. Nil on
	// production builds, where Create constructs the carrier directly as
	// before. It exists because label creation cannot otherwise be tested
	// without a live carrier account — and the sync loop's carrierFactory
	// is deliberately scoped to that loop, so overloading it would blur a
	// boundary its own doc comment draws.
	newCarrier func(provider, apiKey, secretKey, mode string) (shipping.Carrier, error)
```

with a `WithCarrierConstructor` setter alongside the existing `With…`
methods, and in `Create`:

```go
	construct := shipping.NewCarrier
	if h.newCarrier != nil {
		construct = h.newCarrier
	}
	carrier, err := construct(provider, apiKey, secretKey, carrierCfg.Mode)
```

Production behaviour is unchanged: nothing calls the setter outside tests.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/handlers/admin/shipments_per_warehouse_integration_test.go`.

Your stub carrier implements `shipping.Carrier` and returns a canned
shipment (a tracking number that encodes which pickup address it was called
with, so the two-warehouse test can prove each parcel shipped from the right
origin). Attach it with `WithCarrierConstructor`.

The load-bearing case is the FIRST one. Every order in production has no allocations, so if that path changes, label creation breaks for all of them.

```go
//go:build integration

// Package admin — coverage for creating one shipment per contributing
// warehouse (#177 PR 4b).
//
// An order with NO allocations must behave exactly as it did before this
// change: one shipment, pickup from the carrier config's warehouse. That is
// not a fallback for tidiness — order_allocations is empty in production, so
// it is the only path that currently runs.
package admin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// groupsForOrder returns (warehouse_id, quantity) per allocation group,
// ordered for stable assertions.
func groupsForOrder(t *testing.T, db *gorm.DB, orderID string) []struct {
	WarehouseID string
	Quantity    int
	ShipmentID  *string
} {
	t.Helper()
	var rows []struct {
		WarehouseID string
		Quantity    int
		ShipmentID  *string
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, sum(quantity) AS quantity, max(shipment_id::text) AS shipment_id
		   FROM order_allocations WHERE order_id = ?
		  GROUP BY warehouse_id ORDER BY warehouse_id`, orderID).Scan(&rows).Error)
	return rows
}

func shipmentsForOrder(t *testing.T, db *gorm.DB, orderID string) []struct {
	ID          string
	WarehouseID *string
} {
	t.Helper()
	var rows []struct {
		ID          string
		WarehouseID *string
	}
	require.NoError(t, db.Raw(
		`SELECT id, warehouse_id::text AS warehouse_id FROM shipments
		  WHERE order_id = ? ORDER BY created_at, id`, orderID).Scan(&rows).Error)
	return rows
}
```

**The remaining test bodies need the package's existing shipment-test harness** — a router, a fake carrier, and a seeded carrier config. Find it: `grep -rn "func setup.*Shipment\|fakeCarrier\|stubCarrier" internal/handlers/admin/*_test.go`. Reuse that harness rather than building a second one; the carrier must be a stub, because a real carrier call cannot run in a test.

Write these four tests against that harness:

1. `TestCreateShipment_OrderWithNoAllocationsCreatesOneShipmentAsBefore` — seed an order with items and NO `order_allocations`; create a shipment; assert exactly one `shipments` row, and that its `warehouse_id` is NULL (there was no allocation to attribute it to). This is the production path.
2. `TestCreateShipment_OneAllocationGroupCreatesOneShipmentWithItsWarehouse` — one warehouse, allocations present; assert one shipment with `warehouse_id` set to that warehouse, and the allocation rows stamped with that shipment's id.
3. `TestCreateShipment_TwoAllocationGroupsCreateTwoShipments` — two warehouses; assert two shipments, each carrying its own `warehouse_id`, and each group's allocation rows stamped with the matching shipment. Assert the two shipments have DIFFERENT `ship_from` addresses, since shipping from the wrong warehouse is the failure this feature exists to prevent.
4. `TestCreateShipment_AlreadyShippedGroupIsNotShippedTwice` — run creation twice; assert the second run adds no shipment for a group whose allocations already carry a `shipment_id`. This is what makes retry-after-partial-failure safe.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/admin/... -run TestCreateShipment_ -count=1 -p 1
```

Expected: the three allocation-aware tests FAIL (one shipment created, no `warehouse_id`); the no-allocations test PASSES already, because that behaviour is what exists today. A test that passes before the change is fine here — it is a regression guard, and Step 5 mutation-tests it.

- [ ] **Step 3: Group by warehouse in `Create`**

In `internal/handlers/admin/shipments.go`, after the order, address, items and carrier config are loaded and the carrier is instantiated:

1. Load the unshipped allocation groups:

```go
// Groups this order still owes a parcel for. An order with none — every
// order placed before allocation shipped, and every order on a store with
// no warehouse — takes the single-shipment path below, unchanged.
type allocationGroup struct {
	WarehouseID string
	ItemIDs     []string
	Quantities  map[string]int
}
groups, err := h.unshippedAllocationGroups(ctx, orderID)
if err != nil {
	RespondErr(c, err, h.logger)
	return
}
```

  `unshippedAllocationGroups` reads
  `SELECT warehouse_id, order_item_id, quantity FROM order_allocations WHERE order_id = ? AND shipment_id IS NULL ORDER BY warehouse_id, order_item_id`
  and folds it into one group per warehouse. Ordering by `warehouse_id` makes the parcel sequence deterministic — two runs on the same order must produce the same parcels in the same order.

2. When `len(groups) == 0`, run the existing body exactly as it is today. Extract it into a helper (`createSingleShipment`) by MOVING it verbatim rather than rewriting, so the production path is provably unchanged.

3. When there are groups, loop. For each group:
   - resolve the pickup address from **that group's warehouse** (`h.warehouseRepo.ByID`), not from the carrier config
   - build `parcelItems` from **that group's** allocations — the order items it names, at the allocated quantities, not the whole order
   - call the carrier, persist the `shipments` row with `WarehouseID` set
   - stamp `order_allocations.shipment_id` for that group's rows
   - fire `dispatchShipmentDispatchedEmail` for that shipment

4. **A carrier failure part-way through must not undo earlier parcels.** Persist each shipment as it succeeds. On failure, respond with what was created and what failed, and leave the remaining groups unshipped so a retry picks them up:

```go
	if err != nil {
		// The label already created for an earlier group cannot be
		// un-created at the carrier, so it stays. The failed group keeps
		// shipment_id NULL and a retry will pick up exactly the groups
		// still owing a parcel.
		if h.logger != nil {
			h.logger.Error("shipments: carrier create failed for group",
				"order_id", orderID.String(), "warehouse_id", g.WarehouseID,
				"created", len(created), "err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "carrier_create_failed",
			"message": err.Error(),
			"created": len(created),
		})
		return
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/admin/... -run TestCreateShipment_ -count=1 -p 1 -v
```

Expected: all four PASS.

- [ ] **Step 5: Mutation-test the no-allocations guard**

Remove the `len(groups) == 0` branch so every order takes the grouping path, re-run.

Expected: FAIL — `TestCreateShipment_OrderWithNoAllocationsCreatesOneShipmentAsBefore` produces no shipment at all, because an order with no allocations has no groups to loop over. That failure is the production regression this guard exists to prevent. Restore and confirm green.

- [ ] **Step 6: Mutation-test the already-shipped filter**

Drop `AND shipment_id IS NULL` from the group query, re-run.

Expected: FAIL — `TestCreateShipment_AlreadyShippedGroupIsNotShippedTwice` creates a duplicate parcel for an already-shipped group. Restore and confirm green.

- [ ] **Step 7: Verify and commit**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/admin/... ./internal/shipping/... ./internal/order/...
```

```bash
git add services/marketplace-api/internal/handlers/admin/shipments.go \
        services/marketplace-api/internal/handlers/admin/shipments_per_warehouse_integration_test.go
git commit -m "feat(shipping): create one shipment per contributing warehouse (#177)"
```

---

### Task 2: `partial` fulfilment status

**Files:**
- Modify: `services/marketplace-api/internal/order/service.go`
- Modify: `services/marketplace-api/internal/handlers/admin/shipments.go`
- Test: `services/marketplace-api/internal/order/partial_fulfilment_integration_test.go`

**Interfaces:**
- Consumes: the shipments Task 1 creates, and `order_allocations.shipment_id`.
- Produces: `Service.MarkPartiallyFulfilled(ctx, tx, orderID) error`, and shipment creation choosing between it and the existing `MarkFulfilled`.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/order/partial_fulfilment_integration_test.go` with tests that:

1. `TestMarkPartiallyFulfilled_SetsPartial` — an `unfulfilled` order transitions to `partial`.
2. `TestMarkPartiallyFulfilled_RefusesFromFulfilled` — `fulfilled → partial` is not a legal transition and must return an invalid-transition error, not silently downgrade a completed order.
3. `TestMarkPartiallyFulfilled_IsIdempotent` — calling it twice on an already-`partial` order does not error.

Model these on the existing `MarkFulfilled` tests — find them with `grep -rn "MarkFulfilled" internal/order/*_test.go` and follow their fixture and transaction shape exactly.

- [ ] **Step 2: Run to verify failure**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/order/... -run TestMarkPartiallyFulfilled -count=1 -p 1
```

Expected: FAIL to compile — `MarkPartiallyFulfilled` undefined.

- [ ] **Step 3: Add the transition**

In `internal/order/service.go`, add `MarkPartiallyFulfilled` modelled on `MarkFulfilled` directly above or below it. It must use the same `CanTransitionTo` guard, the same repository update, and emit the same kind of status-change event with `To: string(FulfillmentStatusPartial)`. Do not copy `MarkFulfilled`'s `orders.status` change — `partial` moves only `fulfillment_status`; the order is not fulfilled.

- [ ] **Step 4: Call it from shipment creation**

In `shipments.go`, after the groups have been shipped, decide which transition to make:

```go
// An order is fulfilled only when every group it owes a parcel for has
// one. While any allocation still has a NULL shipment_id, the order is
// partially shipped — a status the schema and the transition table have
// always allowed but nothing has ever written.
remaining, err := h.unshippedAllocationCount(ctx, orderID)
```

  Zero remaining → the existing `MarkFulfilled` call. More than zero → `MarkPartiallyFulfilled`. An order with no allocations at all keeps whatever the single-shipment path does today — do not change it.

- [ ] **Step 5: Run the tests**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/order/... ./internal/handlers/admin/...
```

Expected: all `ok`.

- [ ] **Step 6: Mutation-test the remaining-groups check**

Make the decision always call `MarkFulfilled`, re-run.

Expected: FAIL — a two-group order with one parcel shipped reports `fulfilled`, which would tell a customer their order is complete while a parcel has not been created. Restore and confirm green.

- [ ] **Step 7: Verify and commit**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

```bash
git add services/marketplace-api/internal/order/service.go \
        services/marketplace-api/internal/order/partial_fulfilment_integration_test.go \
        services/marketplace-api/internal/handlers/admin/shipments.go
git commit -m "feat(orders): report partial fulfilment while an order still owes a parcel (#177)"
```

---

## Done when

- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` clean
- `go test ./... -count=1` green
- `internal/handlers/admin`, `internal/order`, `internal/shipping` integration packages green against the real DSN, with durations that prove they ran
- Three guards mutation-tested: the no-allocations branch, the already-shipped filter, and the remaining-groups check

## Explicitly NOT in this PR

- **Cancel narrowing.** Cancelling an order must eventually only cover groups that have not shipped. It touches order-cancel semantics and refund-adjacent code and deserves its own review.
- **Naming the parcel in the dispatched email.** `dispatchShipmentDispatchedEmail` is already per-shipment and deduped on `dispatched_email_sent_at`, so N parcels already send N emails correctly; only the copy is generic, and that template lives outside this service.
- Warehouse CRUD and per-location stock editing (PR 5)
- The sentinel backfill (PR 6)
- Any change to `commitStockAtSentinel` or the allocation path itself
