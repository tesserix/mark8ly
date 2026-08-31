# Multi-warehouse: per-location inventory and order allocation

Design for the remainder of #177. Written 2026-08-31, after #480 (store-level
`warehouses` table, readers and writers moved onto `warehouse_id`) and #484
(the eight legacy `warehouse_*` columns dropped, migration 000117, live in
production).

## What this delivers

A store can run more than one warehouse. Stock is held per warehouse, an
order is allocated across warehouses when it is placed, and each warehouse
that contributes ships its own parcel.

## Product decisions

These three were decided by the product owner on 2026-08-31. They are
recorded here because each one has cheaper alternatives that a later reader
might otherwise assume were overlooked.

1. **Allocation rule: explicit priority order.** The merchant ranks their
   warehouses; allocation fills from the top down. Not nearest-pincode
   (needs distance data marketplace-api does not hold) and not most-stock
   (non-deterministic between two similar orders minutes apart).
2. **An order that no single warehouse can fill is split**, not rejected and
   not backordered. Two warehouses contributing means two shipments, two
   labels, two tracking numbers.
3. **The binding allocation happens at order placement**, not at
   add-to-cart and not at label creation.

### One refinement decision 3 forces

`stock_holds.location_id` is `NOT NULL` and part of the
`(cart_token, variant_id, location_id)` unique key, so a cart-time hold must
name a location. A cart hold parked on a sentinel would reserve nothing real
and the units could be sold from under the shopper between cart and
checkout — losing the guarantee #231 exists to provide.

So cart-time holds are **provisional**: add-to-cart runs the allocator and
places per-location holds. Order placement runs the allocator again against
fresh availability, adjusts the holds if the plan has changed, and only that
second plan is written to `order_allocations`. The binding decision is at
placement, as decided; the reservation is continuous, as #231 requires.

## Non-goals

- **Minimising parcel count.** Filling from the merchant's first warehouse
  down is explainable to a merchant; a true minimum is a bin-packing problem
  and buys one fewer parcel in rare cases.
- **Nearest-warehouse allocation.** Excluded by decision 1. The allocator's
  interface takes an ordered warehouse list, so adding a different ordering
  later does not change its callers.
- **Backorders.** No order line may be left partly unfulfilled at placement.
- **Re-planning on a lost hold race.** If a hold fails between snapshot and
  placement the order fails out-of-stock, exactly as a single-warehouse
  shopper's does today. Build the retry when the race is observed, not
  before.
- **Transfers between warehouses.** Stock moves by editing both numbers.

## Data model

### `warehouses.priority`

```sql
ALTER TABLE warehouses ADD COLUMN priority integer NOT NULL DEFAULT 0;
```

Allocation orders by `priority ASC, is_default DESC, created_at ASC`. The
tiebreakers make the ordering total: two warehouses created in the same
second must not allocate differently between two requests.

### `order_allocations`

```sql
CREATE TABLE order_allocations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    store_id      uuid NOT NULL,
    order_id      uuid NOT NULL REFERENCES orders(id)       ON DELETE CASCADE,
    order_item_id uuid NOT NULL REFERENCES order_items(id)  ON DELETE CASCADE,
    warehouse_id  uuid NOT NULL REFERENCES warehouses(id)   ON DELETE RESTRICT,
    quantity      integer NOT NULL CHECK (quantity > 0),
    shipment_id   uuid REFERENCES shipments(id)             ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_item_id, warehouse_id)
);
CREATE INDEX order_allocations_order_idx    ON order_allocations (order_id);
CREATE INDEX order_allocations_shipment_idx ON order_allocations (shipment_id);
```

One row per (line, warehouse). `ON DELETE RESTRICT` on `warehouse_id` is what
stops a merchant deleting a warehouse that owes a parcel.

The invariant — for each line, `SUM(quantity)` equals `order_items.quantity` —
cannot be a table CHECK, since it spans rows. It is enforced in the allocator
and pinned by a test that deletes the enforcement and watches the test fail.

`shipment_id` is NULL until a label is printed. That is the flag that makes
re-allocation safe before printing and refused after.

### `shipments.warehouse_id`

```sql
ALTER TABLE shipments ADD COLUMN warehouse_id uuid REFERENCES warehouses(id);
```

Nullable: every shipment created before this has no honest answer. Every
shipment created after it sets the column.

### The `sync_variant_inventory` trigger

Today:

```sql
UPDATE product_variants SET inventory_quantity = NEW.quantity
 WHERE id = NEW.variant_id;
```

It assigns the *last touched* location's quantity. With one location that is
the total; with two, `product_variants.inventory_quantity` — which browse,
PDP and cart all read — becomes whichever warehouse was written most
recently. The function's own comment says slice 2 changes it to a SUM.

It becomes a SUM over the variant's rows, and gains an `AFTER DELETE` arm it
does not have today: removing a warehouse's stock row must lower the total,
and with only INSERT and UPDATE triggers it would not.

### Warehouse deletion

Refused while the warehouse holds stock (`variant_stock.quantity > 0`) or owes
a parcel (`order_allocations` rows with a NULL `shipment_id`).

The `ON DELETE RESTRICT` FK is a backstop, not the rule: it refuses while
*any* allocation row references the warehouse, including long-shipped ones,
which would make a warehouse undeletable forever. Both rules therefore live in
the repository with their own tests, and the FK exists so that a path which
forgets them fails loudly instead of orphaning an unshipped parcel. Deleting a
warehouse with shipped history is allowed and nulls nothing — the allocation
rows keep pointing at it, which is why it is RESTRICT and not CASCADE.

## The sentinel backfill, and why it is two deploys

`variant_stock` and `stock_holds` both carry
`location_id = '00000000-0000-0000-0000-000000000001'`, the `DefaultLocationID`
constant in `internal/product/service.go`. Nine non-test call sites pass it:
five under `internal/product/`, and four in storefront checkout and cart-hold
code that did not exist when #177 was filed (the issue's count of seven
predates `stock_holds`).

A store can hold stock while having no `warehouses` row at all: warehouses are
created only when a carrier config is saved, and production currently has zero
of both. So the backfill must first *create* a warehouse for each store that
holds stock — `name = 'Main Warehouse'`, blank address, `is_default = true` —
then rewrite `location_id` to it. A blank address is the honest state: that
store has not told us where it ships from, and the existing admin settings
flow is what fills it in.

**This cannot be one deploy.** Migrations run in the `migrate` initContainer
on the **admin** deployment only; `marketplace-api-storefront` and the sweeper
CronJobs share the image but roll independently (verified while shipping #484
and #486). Code still on the old image reads and writes the sentinel. If the
backfill lands first, an old storefront pod holds stock against a location id
that no longer has rows, availability reads zero, and every shopper is told
the item is sold out.

So, the same expand/contract shape #484 used:

1. **Expand.** Add `priority`, `order_allocations`, `shipments.warehouse_id`,
   change the trigger to SUM (a no-op while every variant has one row). Ship
   code that resolves the store's warehouse and tolerates BOTH: a
   `variant_stock` row on a real warehouse id and one on the sentinel.
2. **Contract.** Once that is deployed on admin, storefront and the CronJobs,
   run the backfill and remove the sentinel tolerance.

`DefaultLocationID` is deleted at the end of step 2, not before.

## The allocator

A new package, `internal/allocation`, with a pure decision core:

```go
// Plan returns which warehouse ships how much of each line, or
// ErrCannotFill naming the first line that no combination satisfies.
func Plan(warehouses []Warehouse, avail Availability, lines []Line) ([]Assignment, error)
```

No database access in the decision, so its interesting cases are table tests
rather than fixtures.

**Algorithm.** Walk the warehouses in priority order. At each, take
`min(remaining, available)` for every line not yet satisfied. Stop when all
lines are satisfied; fail if any line still has a remainder.

**`continue`-policy variants sell past zero**, so availability does not
constrain them and a naive loop assigns them nowhere. They are assigned whole
to the highest-priority warehouse. Stated explicitly because the silent
alternative is an order line that ships from nothing.

**The plan is advisory; `stockhold.Hold` is the guarantee.** Allocation runs
inside the order transaction, then a hold is placed per assignment. `Hold`'s
own availability check, under the existing `SELECT … FOR UPDATE` discipline,
is what stops two orders taking the last unit. Nothing in this design adds a
new concurrency primitive.

## Checkout

`commitStock` in `internal/handlers/storefront/checkout_stock.go` is the
chokepoint and keeps its shape. It changes from "hold each line at the
sentinel" to:

1. Load the store's warehouses in priority order and the per-location
   availability snapshot.
2. `allocation.Plan(...)`.
3. Place a hold per assignment, at that assignment's warehouse.
4. Write `order_allocations` from the plan.
5. `holds.Commit(cartToken)` — unchanged; it already commits every live hold
   the cart owns.

A line split across two warehouses becomes two holds for the same cart and
variant at different locations, which the `(cart_token, variant_id,
location_id)` unique key already permits.

`cart_holds.go` runs steps 1–3 only: provisional holds, no
`order_allocations` row. At placement, assignments that no longer match are
released and replaced before the commit.

Shopper-facing availability is the sum across locations — already what the
storefront displays, because it reads `product_variants.inventory_quantity`.

## Fulfilment

Label creation groups `order_allocations` by warehouse and creates one
shipment per group. Each shipment:

- resolves `ship_from` from **its own** warehouse, not from the carrier
  config's `warehouse_id`,
- sets `shipments.warehouse_id`,
- stamps its id onto the allocation rows it covers.

The carrier config continues to supply credentials and provider; only the
origin moves. Each warehouse registers with the carrier separately —
`UpsertWarehouse` already does adopt-or-create by name.

`fulfillment_status` becomes `partial` while any allocation group has no
shipment, and `fulfilled` when every group has one. Both values and the
`unfulfilled → partial → fulfilled` transition already exist in
`internal/order/status.go`; `partial` has simply never been written.

## Admin

**The warehouse stops being a sub-form of a carrier config.** Today
`ShippingConfigForm` embeds `AddressFieldset` and saving a Delhivery config
upserts the warehouse behind it — #480 removed the duplicated storage but a
merchant still types an address into a carrier form.

- New `settings/warehouses` page: list, create, edit, delete, mark default,
  set priority.
- `ShippingConfigForm` drops `AddressFieldset` and gains a warehouse picker.
- `/api/v1/admin/stores/:storeId/warehouses` — list, create, update, delete,
  reorder. `internal/warehouse` has `Upsert`, `DefaultForStore` and `ByID`
  already; it gains `List`, `Delete` (with the refusal rules above) and
  `SetPriorities`.

**Per-location stock editing, under one rule: a store with one warehouse sees
exactly what it sees today.** The product form keeps its single stock field
until a second warehouse exists, then expands to a per-warehouse breakdown
with the total shown. Otherwise every existing merchant pays complexity for a
feature they are not using.

`UpdateVariantStockInTx` remains the single mutation chokepoint, guarded by
its existing test. It already takes a location; the change is carrying a real
warehouse id down from the API instead of the sentinel.

## Consumers of split shipments

**Refunds are unaffected**, and this is the reason for a separate
`order_allocations` table rather than a `warehouse_id` on `order_items`.
`refunds.order_item_id` is a RESTRICT FK onto `order_items`; lines are never
split, so a refund means the same thing whether the order shipped as one
parcel or three.

**The customer order page must change.** `order_detail.go` exposes
`Shipment *storefrontShipmentResponse` — singular — and `loadShipment(orderID)`
returns one row. With two parcels the customer sees one tracking number and
silently loses the other. The field becomes a list. This is a public
response-shape change, so the storefront app rendering it ships in the same
release.

**Cancellation narrows.** Cancelling a shipment stays coherent per parcel.
Cancelling an *order* may only cover groups that have not shipped; attempting
it against a shipped group is refused rather than half-applied.

**`orderdoc`** is already partly multi-shipment aware (`lookupDeliveredAt`
reads the most recently delivered shipment). The dispatched email is written
per shipment and will now send once per parcel. That is correct, and the copy
must name the parcel — two identical "your order has shipped" emails read as
a bug.

## Testing

- **Allocator:** table tests over the pure core. Cases: single warehouse
  fills; two warehouses split one line; a line no combination can fill;
  `continue`-policy assignment; priority ties broken deterministically.
- **Every guard is mutation-tested.** Delete the guard, watch a test fail. A
  guard whose removal breaks nothing is decoration. This applies at minimum
  to the per-line quantity invariant, the warehouse-deletion refusals, and
  the trigger's DELETE arm.
- **Integration tests** are `//go:build integration`, gated on
  `TEST_DATABASE_URL` (never `TEST_DB_DSN`), run with `-p 1`. A skip reads as
  a pass, so any claim about an integration run must name the DSN it ran
  with.
- **Trigger coverage** reads the migration verbatim from the embedded
  `MigrationsFS`, as migration 000116's test did, so the test exercises what
  ships rather than a copy that can drift.
- **The backfill** is tested at both deploy steps: sentinel rows tolerated
  before it, absent after.

## Risks

- **The two-deploy sequence is the highest risk in this design.** Landing the
  backfill before the tolerant code reaches storefront presents as every
  product being sold out. The contract migration must not merge until the
  expand half is confirmed running on admin, storefront and both CronJobs.
- **The trigger change is silent if wrong.** A SUM that misses the DELETE arm
  overstates stock and oversells. Mutation-test it.
- **The public `Shipment` field change** breaks any consumer not updated in
  the same release. The storefront app is the only known one; that must be
  verified, not assumed.
- **Nobody is using warehouses yet.** Production has zero warehouses and zero
  carrier configs, so none of this can be validated against real merchant
  data before it ships. Integration coverage carries the whole weight.

## Suggested delivery order

Each is a separate PR; each is deployable on its own.

1. `priority`, `order_allocations`, `shipments.warehouse_id`, trigger → SUM
   with the DELETE arm. Additive; no behaviour change while every store has
   one warehouse.
2. `internal/allocation` — the pure core and its tests. No callers.
3. Checkout: allocation at cart and at placement, tolerating the sentinel.
4. Fulfilment: shipment per group, `partial` fulfillment status, order-detail
   `Shipment` → list, storefront app updated.
5. Admin: warehouse CRUD, carrier-config picker, per-location stock editing.
6. **Contract:** backfill the sentinel, delete `DefaultLocationID`. Only after
   1–5 are deployed everywhere.
