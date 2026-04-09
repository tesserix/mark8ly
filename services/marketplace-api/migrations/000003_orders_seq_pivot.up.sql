-- 000003_orders_seq_pivot.up.sql
-- Pivot the orders sequence strategy from a shared hot-row table
-- (document_number_seq) to per-store native Postgres sequences.
--
-- Motivation: the 000002 atomic-upsert-on-shared-row pattern showed
-- p99 sequence contention of ~60ms on Linux Postgres under 50 concurrent
-- checkouts, exceeding the 50ms M1 exit gate before any order-graph
-- inserts were added. Native sequences with CACHE use a separate
-- lightweight mechanism and do not take row locks, eliminating the
-- bottleneck entirely.
--
-- Spec reference: §2 decision 8 (revised), §11 risks (resolved).

BEGIN;

DROP TABLE IF EXISTS document_number_seq;

-- ensure_store_sequence creates (idempotently) a per-store, per-kind Postgres
-- SEQUENCE object on demand and returns its name. Called from Go inside the
-- create-order transaction the first time we need a number for a given
-- (store_id, kind) pair; the name is then cached in the Go process for the
-- lifetime of the pod.
--
-- Sequence naming: mk_seq_<kind>_<uuid-with-dashes-replaced-by-underscores>.
-- This keeps every name under Postgres' 63-char NAMEDATALEN limit
-- (13 + 32 = 45 chars) and makes names valid identifiers without quoting.
--
-- CACHE 50 reserves 50 values per backend connection, eliminating round-trip
-- contention under burst. Gaps in the allocated range on crash/disconnect
-- are expected and harmless — the sequence is advisory for human-readable
-- order numbers, not a financial invariant.
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
