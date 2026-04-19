-- Migration 058: customer portal secret + cancellation_reason column
-- Part of P11 — cancellation + GDPR customer portal.

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
