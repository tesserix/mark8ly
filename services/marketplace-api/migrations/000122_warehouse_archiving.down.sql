-- Restores the total unique index. This can only succeed when no two rows
-- share (store_id, name) — which is violated exactly when a name has been
-- reused after archiving, the very case the partial index exists to allow.
--
-- Failing loudly here is correct: silently dropping or renaming a merchant's
-- warehouse to force the old index through would lose the record of which
-- warehouse shipped an order. Resolve the duplicate deliberately, then roll
-- back.
DROP INDEX IF EXISTS warehouses_store_live_idx;
DROP INDEX IF EXISTS warehouses_store_name_live_key;

CREATE UNIQUE INDEX warehouses_store_name_key
    ON warehouses (store_id, name);

ALTER TABLE warehouses
    DROP COLUMN IF EXISTS archived_at;
