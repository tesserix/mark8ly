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
-- Postgres does not index the referencing side of an FK automatically.
-- Without this, DELETE FROM warehouses sequential-scans order_allocations,
-- and PR 5's "does this warehouse owe a parcel" query has nothing to use.
CREATE INDEX IF NOT EXISTS order_allocations_warehouse_idx
    ON order_allocations (warehouse_id);

-- Where a shipment actually shipped from. Nullable because every shipment
-- created before this migration has no honest answer; every one created
-- after it sets the column.
ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS warehouse_id uuid REFERENCES warehouses(id);

-- Same FK-indexing gap as above: without this, DELETE FROM warehouses
-- sequential-scans shipments too.
CREATE INDEX IF NOT EXISTS shipments_warehouse_idx
    ON shipments (warehouse_id);
