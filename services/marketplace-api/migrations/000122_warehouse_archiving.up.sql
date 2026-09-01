-- #177 PR 5a — archiving as the removal mechanism for a warehouse that has
-- allocation history.
--
-- A warehouse with ANY allocation history — even fully shipped — must never
-- be deleted: order_allocations.warehouse_id is ON DELETE RESTRICT, and an
-- allocation row is the record of which warehouse shipped a line. Deleting
-- the warehouse would corrupt that record, so RESTRICT refuses it outright.
-- Archiving is what "remove" actually means for such a warehouse.
ALTER TABLE warehouses
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

-- The uniqueness must become PARTIAL, or archiving permanently burns a name:
-- a merchant who archives "Main Warehouse" could never create another one,
-- because the old row still occupies (store_id, name).
--
-- Safe to swap in either order only if no two LIVE rows collide, which the
-- old total index already guarantees — every existing row has archived_at
-- NULL, so the partial index covers exactly the same set today.
DROP INDEX IF EXISTS warehouses_store_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS warehouses_store_name_live_key
    ON warehouses (store_id, name)
    WHERE archived_at IS NULL;

-- Archived warehouses must never be offered to the allocator or the admin
-- pickers, and both filter on archived_at. Index the live set so that filter
-- stays cheap as archived rows accumulate.
CREATE INDEX IF NOT EXISTS warehouses_store_live_idx
    ON warehouses (store_id)
    WHERE archived_at IS NULL;
