-- Migration 058: customer portal secret + cancellation_reason column
-- Part of P11 — cancellation + GDPR customer portal.

-- gen_random_bytes below is a pgcrypto function, not core PostgreSQL. Nothing
-- in the chain created the extension, so replaying these migrations against a
-- FRESH database failed here with "function gen_random_bytes(integer) does not
-- exist" — which broke CI, disaster recovery, and every new developer machine,
-- while every existing database kept working because pgcrypto had been
-- installed out of band. 000072 depends on it too.
--
-- Adding it to an already-applied migration is safe: golang-migrate tracks
-- version numbers and never re-runs this file, and IF NOT EXISTS makes it a
-- no-op for anyone who does. (gen_random_uuid, used by 39 other migrations,
-- is core in PG13+ and needs nothing.)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Per-store HMAC signing key for the customer portal token (§15.4).
-- Generated at migration time using gen_random_bytes so existing stores are
-- backfilled immediately. The DEFAULT is then dropped so every future INSERT
-- must supply the value explicitly (auditable creation path).
ALTER TABLE stores
    ADD COLUMN storefront_customer_portal_secret CHAR(64) NOT NULL
        DEFAULT encode(gen_random_bytes(32), 'hex');

ALTER TABLE stores ALTER COLUMN storefront_customer_portal_secret DROP DEFAULT;

-- Cancellation reason is authoritative in audit_logs; we do NOT persist it on
-- store_subscriptions (see P11 plan §2 comment). No column added here.
