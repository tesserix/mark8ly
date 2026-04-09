-- 000004_orders_seq_eager.up.sql
-- Move per-store order/return sequence creation from lazy (on first use, via
-- ensure_store_sequence called from the order-create hot path) to eager
-- (at store-insert time, via an AFTER INSERT trigger on stores).
--
-- Motivation: the 000003 lazy ensure_store_sequence function used
-- CREATE SEQUENCE IF NOT EXISTS, which is a check-then-act against pg_class.
-- Under concurrent first-order-per-store creation, two sessions could both
-- observe "not exists" and race on the pg_class_relname_nsp_index unique
-- constraint, producing SQLSTATE 23505 errors and blowing the M1 p99 gate
-- (observed 154–486ms vs 75ms target on local benchmarks).
--
-- Eager creation eliminates the race by construction (sequences always
-- exist by the time any order write could reach the store) and also
-- removes DDL from the order-create hot path, reducing first-call latency.
--
-- Spec reference: §2 decision 8 (revised again), §11 risks.

BEGIN;

-- -----------------------------------------------------------------------------
-- Trigger function: create both per-store sequences (order + return) whenever
-- a new store row is inserted.
--
-- Naming convention MUST match internal/order/number.go buildSequenceName:
--     mk_seq_<kind>_<uuid-with-dashes-replaced-by-underscores>
-- with kind ∈ {'order','return'}.
--
-- CACHE 50 reserves 50 values per backend connection so bursts do not hit
-- the sequence page on every nextval. Gaps on crash/disconnect are expected
-- and harmless — sequences are advisory numbering, not a financial invariant.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION mk_create_store_sequences()
RETURNS trigger AS $$
DECLARE
    order_seq  text;
    return_seq text;
BEGIN
    order_seq  := format('mk_seq_order_%s',  translate(NEW.id::text, '-', '_'));
    return_seq := format('mk_seq_return_%s', translate(NEW.id::text, '-', '_'));

    EXECUTE format(
        'CREATE SEQUENCE IF NOT EXISTS %I START WITH 1 INCREMENT BY 1 CACHE 50 MINVALUE 1',
        order_seq
    );
    EXECUTE format(
        'CREATE SEQUENCE IF NOT EXISTS %I START WITH 1 INCREMENT BY 1 CACHE 50 MINVALUE 1',
        return_seq
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS stores_after_insert_create_sequences ON stores;
CREATE TRIGGER stores_after_insert_create_sequences
AFTER INSERT ON stores
FOR EACH ROW
EXECUTE FUNCTION mk_create_store_sequences();

-- -----------------------------------------------------------------------------
-- Backfill: create sequences for any stores that already exist at migration
-- time. In local dev this is a no-op (zero stores); in prod it ensures the
-- switch is seamless. The loop runs serially inside this migration's single
-- transaction, so CREATE SEQUENCE IF NOT EXISTS is race-free here.
-- -----------------------------------------------------------------------------
DO $$
DECLARE
    r RECORD;
    order_seq  text;
    return_seq text;
BEGIN
    FOR r IN SELECT id FROM stores LOOP
        order_seq  := format('mk_seq_order_%s',  translate(r.id::text, '-', '_'));
        return_seq := format('mk_seq_return_%s', translate(r.id::text, '-', '_'));
        EXECUTE format(
            'CREATE SEQUENCE IF NOT EXISTS %I START WITH 1 INCREMENT BY 1 CACHE 50 MINVALUE 1',
            order_seq
        );
        EXECUTE format(
            'CREATE SEQUENCE IF NOT EXISTS %I START WITH 1 INCREMENT BY 1 CACHE 50 MINVALUE 1',
            return_seq
        );
    END LOOP;
END $$;

-- -----------------------------------------------------------------------------
-- Retire the lazy ensure_store_sequence function. Go no longer calls it.
-- -----------------------------------------------------------------------------
DROP FUNCTION IF EXISTS ensure_store_sequence(uuid, text);

COMMIT;
