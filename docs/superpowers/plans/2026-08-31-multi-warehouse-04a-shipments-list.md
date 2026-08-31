# Multi-warehouse PR 4a: expose every shipment on the order — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The customer order-detail response exposes every shipment on an order, not just the most recent one — additively, so the storefront app can migrate on its own schedule.

**Architecture:** One new field. `shipments` is a list ordered oldest-first; the existing `shipment` keeps returning the most recent shipment exactly as it does today. No behaviour changes for any current consumer.

**Tech Stack:** Go 1.26, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md` (see "Consumers of split shipments")

## Global Constraints

- Work in the worktree `.claude/worktrees/177-fulfilment`, branch `feat/177-fulfilment`. Never switch the main checkout's branch.
- Run every Go command from `services/marketplace-api`, never path-scoped, always `-count=1`: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `//go:build integration`, gated on `TEST_DATABASE_URL` (**never** `TEST_DB_DSN`), run with `-p 1`. A skip prints `ok` and reads like a pass — name the DSN and the duration in every claim.
- Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` (container `dev-postgres-1`; the LAN IP, not localhost).
- Commits: conventional, single line, no signature, no `Co-Authored-By`, no emoji. Stage explicit paths; never `git add -A`.
- Every guard added must be mutation-tested.
- **`shipment` (singular) must not change.** Seven call sites in `apps/storefront` read it, including invoice rendering (`lib/invoices/render.ts` reads `order.shipment.status` and `delivered_at`). marketplace-api and the storefront app deploy independently, so a replacement would break invoices the moment the API rolled first.

---

## Why additive

The spec says the field "becomes a list". Taken literally that is a breaking change to a public response, and `marketplace-api` rolls independently of `apps/storefront` — the same split that forced #484 into two PRs and #491's deploy precondition.

So: add `shipments`, keep `shipment`. The app migrates to the list in its own PR, and `shipment` is retired later, once nothing reads it. This PR is safe to deploy alone, in either order relative to the app.

---

## File Structure

- **Modify** `services/marketplace-api/internal/handlers/storefront/order_detail.go` — add the `Shipments` field and a `loadShipments` reader; leave `loadShipment` untouched
- **Create** `services/marketplace-api/internal/handlers/storefront/order_detail_shipments_integration_test.go`

---

### Task 1: Expose every shipment

**Files:**
- Modify: `services/marketplace-api/internal/handlers/storefront/order_detail.go`
- Test: `services/marketplace-api/internal/handlers/storefront/order_detail_shipments_integration_test.go`

**Interfaces:**
- Consumes: the existing `storefrontShipmentResponse` and `shipmentRow` types in that file.
- Produces: `Shipments []storefrontShipmentResponse` with JSON key `shipments` on the order-detail response, and `loadShipments(ctx, orderID) []storefrontShipmentResponse`. PR 4b creates the multiple shipments this exposes; `apps/storefront` migrates to it in its own PR.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/handlers/storefront/order_detail_shipments_integration_test.go`:

```go
//go:build integration

// Package storefront — coverage for exposing every shipment on an order
// (#177 PR 4a).
//
// Until now the response carried one `shipment`, chosen as the most recent.
// Multi-warehouse orders ship as more than one parcel, and a customer seeing
// only one tracking number has silently lost the other.
//
// `shipment` is deliberately UNCHANGED: seven call sites in apps/storefront
// read it, including invoice rendering, and the API deploys independently of
// the app. The list is added alongside so the app can migrate on its own
// schedule.
package storefront

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedShipmentsOrder creates a store and an order, and returns (storeID, orderID).
func seedShipmentsOrder(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Ship List Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "shiplist-"+uuid.NewString()[:8], uuid.NewString()).Error)

	orderID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 10.00, 10.00)`,
		orderID, tenantID, storeID, "SL-"+uuid.NewString()[:8], uuid.NewString()).Error)
	return storeID, orderID
}

func seedShipment(t *testing.T, db *gorm.DB, storeID, orderID, carrier, tracking string, createdAt time.Time) {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	require.NoError(t, db.Exec(
		`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, tracking_number,
		                        status, ship_from, ship_to, handling_fee, currency_code, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}'::jsonb, '{}'::jsonb, 0, 'INR', ?)`,
		uuid.NewString(), tenantID, storeID, orderID, carrier, tracking, createdAt).Error)
}

func TestLoadShipments_ReturnsEveryShipmentOldestFirst(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	got := h.loadShipments(context.Background(), uuid.MustParse(orderID))

	require.Len(t, got, 2, "a two-parcel order must expose both shipments")
	require.Equal(t, "AWB-FIRST", got[0].TrackingNumber, "oldest first, so parcel order is stable as more are added")
	require.Equal(t, "AWB-SECOND", got[1].TrackingNumber)
}

func TestLoadShipments_EmptyForAnUnshippedOrder(t *testing.T) {
	db := testdb.NewTx(t)
	_, orderID := seedShipmentsOrder(t, db)

	h := &OrderDetailHandler{db: db}
	got := h.loadShipments(context.Background(), uuid.MustParse(orderID))

	require.Empty(t, got, "an unshipped order has no parcels, and must not error")
}

// The singular field is what apps/storefront reads today — including invoice
// rendering. If this diverges, invoices break the moment the API deploys
// ahead of the app.
func TestLoadShipment_StillReturnsTheMostRecent(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	got := h.loadShipment(context.Background(), uuid.MustParse(orderID))

	require.NotNil(t, got)
	require.Equal(t, "AWB-SECOND", got.TrackingNumber,
		"the singular field is unchanged: most recent, exactly as before this PR")
}

// The list and the singular field must agree about the same parcel, or a
// customer sees one status in the summary and another in the detail.
func TestLoadShipments_LastEntryMatchesTheSingularField(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	list := h.loadShipments(context.Background(), uuid.MustParse(orderID))
	single := h.loadShipment(context.Background(), uuid.MustParse(orderID))

	require.NotEmpty(t, list)
	require.NotNil(t, single)
	require.Equal(t, list[len(list)-1].TrackingNumber, single.TrackingNumber)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run 'TestLoadShipment' -count=1 -p 1
```

Expected: FAIL to compile — `h.loadShipments` undefined. If every test SKIPs instead, `TEST_DATABASE_URL` did not reach the process; fix that first, because a skip is indistinguishable from a pass.

- [ ] **Step 3: Add the field and the reader**

In `services/marketplace-api/internal/handlers/storefront/order_detail.go`:

1. Add to the order response struct, immediately after the existing `Shipment` field:

```go
	// Shipments is every parcel on the order, oldest first. A multi-warehouse
	// order ships as more than one (#177), and the singular Shipment above
	// shows only the most recent — a customer reading it alone would silently
	// lose the other tracking numbers.
	//
	// Shipment is kept, unchanged, because apps/storefront reads it in seven
	// places including invoice rendering, and this service deploys
	// independently of that app. It is retired once nothing reads it.
	Shipments []storefrontShipmentResponse `json:"shipments"`
```

  Note the tag has no `omitempty`: an unshipped order should serialise `"shipments": []`, not omit the key, so a client can distinguish "no parcels" from "old API".

2. Add `loadShipments`, modelled on the existing `loadShipment` but without the `Limit(1)` and ordered ascending:

```go
// loadShipments returns every shipment on the order, oldest first.
//
// Ordered ASCENDING by created_at so a parcel keeps its position as later
// ones are added — a customer who bookmarked "parcel 1" should not find it
// renumbered. loadShipment's DESC + Limit(1) is left alone: it answers a
// different question (the most recent parcel) that the singular field still
// promises.
//
// A read failure yields an empty list rather than an error: the order page
// must still render without its tracking numbers, exactly as loadShipment
// already degrades.
func (h *OrderDetailHandler) loadShipments(ctx context.Context, orderID uuid.UUID) []storefrontShipmentResponse {
	var rows []shipmentRow
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("carrier", "tracking_number", "status", "estimated_delivery", "shipped_at", "delivered_at").
		Where("order_id = ?", orderID).
		Order("created_at ASC, id ASC").
		Scan(&rows).Error
	if err != nil {
		if h.logger != nil {
			h.logger.Error("storefront: load shipments", "order_id", orderID, "err", err)
		}
		return nil
	}

	out := make([]storefrontShipmentResponse, 0, len(rows))
	for _, row := range rows {
		if row.Carrier == "" {
			continue
		}
		out = append(out, shipmentResponseFrom(row))
	}
	return out
}
```

3. If `loadShipment` builds its `storefrontShipmentResponse` inline, extract that construction into `shipmentResponseFrom(row shipmentRow) storefrontShipmentResponse` and have BOTH functions use it. Do not duplicate the field mapping — the two must not be able to drift, and `TestLoadShipments_LastEntryMatchesTheSingularField` pins that they agree.

4. Populate the field where `resp.Shipment` is already assigned:

```go
	resp.Shipments = h.loadShipments(c.Request.Context(), orderID)
```

**Note on the `created_at, id` ordering:** unlike `order_items`, shipments are inserted one per carrier call rather than in a batch, so their timestamps genuinely differ. The `id` tiebreak is there for the case where two land in the same instant, so the order is total either way — the same reasoning as PR 2's warehouse ordering.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration ./internal/handlers/storefront/... -run 'TestLoadShipment' -count=1 -p 1 -v
```

Expected: all four PASS.

- [ ] **Step 5: Mutation-test the ordering**

Change `Order("created_at ASC, id ASC")` to `Order("created_at DESC")`, re-run.

Expected: FAIL — `TestLoadShipments_ReturnsEveryShipmentOldestFirst` finds `AWB-SECOND` first, and `TestLoadShipments_LastEntryMatchesTheSingularField` fails too. Restore and confirm green.

- [ ] **Step 6: Verify the singular field is untouched**

```bash
cd services/marketplace-api
git diff main -- internal/handlers/storefront/order_detail.go | grep -E '^-' | grep -v '^---'
```

Expected: the only removed lines are those refactored into `shipmentResponseFrom`. If `loadShipment`'s query, its `DESC`, or its `Limit(1)` appears as a removal, the singular field's behaviour has changed and seven storefront call sites are at risk — revert that part.

- [ ] **Step 7: Full verification**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/handlers/storefront/... ./internal/order/...
```

Expected: all `ok`, durations in seconds.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/order_detail.go \
        services/marketplace-api/internal/handlers/storefront/order_detail_shipments_integration_test.go
git commit -m "feat(storefront): expose every shipment on the order detail, not just the most recent (#177)"
```

---

## Done when

- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` clean
- `go test ./... -count=1` green
- The storefront and order integration packages green against the real DSN, with durations that prove they ran
- The ordering mutation-tested
- `loadShipment`'s query, ordering and `Limit(1)` provably unchanged

## Explicitly NOT in this PR

- Creating more than one shipment (PR 4b) — until that lands, `shipments` simply holds the single parcel every order already has, which is why this is safe to deploy alone
- `fulfillment_status = 'partial'`, cancel narrowing, per-parcel dispatch email (PR 4b)
- Changing `apps/storefront` to read the list — its own PR, on its own schedule; that independence is the entire point of adding the field rather than replacing one
- Retiring the singular `shipment` — only once nothing reads it
