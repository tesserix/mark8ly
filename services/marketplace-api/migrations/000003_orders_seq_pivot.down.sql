-- 000003_orders_seq_pivot.down.sql
-- Rolls back the per-store sequence pivot by recreating document_number_seq
-- and dropping the helper function. Per-store sequences created by the
-- helper in production would be orphaned; the down migration does NOT drop
-- them automatically because Postgres has no reliable way to enumerate them
-- from inside the function. A cleanup script (ops task) can drop any
-- sequence matching `mk_seq_%` if a full rollback is ever needed.

BEGIN;

DROP FUNCTION IF EXISTS ensure_store_sequence(uuid, text);

CREATE TABLE IF NOT EXISTS document_number_seq (
    store_id uuid        NOT NULL,
    kind     varchar(10) NOT NULL,
    day      date        NOT NULL,
    last_seq integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, kind, day),
    CONSTRAINT document_number_seq_kind_valid CHECK (kind IN ('order','return'))
);

COMMIT;
