-- 000004_orders_seq_eager.down.sql
-- Reverse 000004: drop the AFTER INSERT trigger, drop all eagerly-created
-- per-store sequences, and restore the lazy ensure_store_sequence function
-- that 000003 introduced so the Go side can fall back if this migration is
-- rolled back in isolation.

BEGIN;

DROP TRIGGER IF EXISTS stores_after_insert_create_sequences ON stores;
DROP FUNCTION IF EXISTS mk_create_store_sequences();

-- Drop every sequence matching the mk_seq_(order|return)_* naming convention.
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind = 'S'
          AND n.nspname = current_schema()
          AND c.relname ~ '^mk_seq_(order|return)_'
    LOOP
        EXECUTE format('DROP SEQUENCE IF EXISTS %I', r.relname);
    END LOOP;
END $$;

-- Restore the lazy ensure_store_sequence function that 000003 created, so
-- any code path still referencing it (in a rollback scenario) continues to
-- work. This is intentionally identical to 000003's definition.
CREATE OR REPLACE FUNCTION ensure_store_sequence(
    p_store_id uuid,
    p_kind     text
) RETURNS text AS $$
DECLARE
    seq_name text;
BEGIN
    IF p_kind NOT IN ('order', 'return') THEN
        RAISE EXCEPTION 'ensure_store_sequence: invalid kind %', p_kind;
    END IF;

    seq_name := format('mk_seq_%s_%s',
        p_kind,
        translate(p_store_id::text, '-', '_')
    );

    EXECUTE format(
        'CREATE SEQUENCE IF NOT EXISTS %I START WITH 1 INCREMENT BY 1 CACHE 50 MINVALUE 1',
        seq_name
    );

    RETURN seq_name;
END;
$$ LANGUAGE plpgsql;

COMMIT;
