-- cashfree-verify.sql — read-only diagnostics for the Cashfree integration.
--
-- Answers "why isn't Cashfree showing up / taking payments" by walking the same
-- chain the application walks, in order. Every query is a SELECT; this script
-- never writes.
--
-- Usage:
--   psql "$DATABASE_URL" -f scripts/sql/cashfree-verify.sql                 -- all stores
--   psql "$DATABASE_URL" -v store_slug=acme -f scripts/sql/cashfree-verify.sql
--
-- With store_slug set, sections 2–5 narrow to that store.

\set ON_ERROR_STOP on
\if :{?store_slug} \else \set store_slug '' \endif

\echo ''
\echo '=== 1. Country allowlist — is cashfree permitted, and is it preferred? ==='
\echo '    payment_providers order IS the preference order: position 1 is what'
\echo '    the storefront pre-selects. Expect cashfree first for IN (migration 000099).'
SELECT country_code,
       payment_providers,
       'cashfree' = ANY (payment_providers)            AS cashfree_allowed,
       payment_providers[1] = 'cashfree'               AS cashfree_preferred,
       array_position(payment_providers, 'cashfree')   AS cashfree_rank
  FROM supported_countries
 WHERE 'cashfree' = ANY (payment_providers)
    OR country_code = 'IN'
 ORDER BY country_code;

\echo ''
\echo '=== 2. Gateway configs — credentials present, active, and correctly wrapped? ==='
\echo '    ref_kind: gsm=Secret Manager reference (good), aes/noop=legacy inline,'
\echo '    plaintext=written by cashfree-enable.sql and not yet rewrapped.'
SELECT s.slug                                  AS store,
       s.country_code,
       s.currency_code,
       c.mode,
       c.is_active,
       CASE
           WHEN c.api_key_encrypted LIKE 'gsm://%' THEN 'gsm'
           WHEN c.api_key_encrypted LIKE 'aes:%'   THEN 'aes'
           WHEN c.api_key_encrypted LIKE 'noop:%'  THEN 'noop'
           ELSE 'plaintext'
       END                                      AS ref_kind,
       c.secret_key_encrypted IS NOT NULL       AS has_secret_key,
       c.webhook_secret_encrypted IS NOT NULL   AS has_dedicated_webhook_secret,
       c.updated_at
  FROM payment_gateway_configs c
  JOIN stores s ON s.id = c.store_id
 WHERE c.provider = 'cashfree'
   AND (:'store_slug' = '' OR s.slug = :'store_slug')
 ORDER BY s.slug;

\echo ''
\echo '=== 3. Misconfiguration checks ==='
\echo '    Each row below is a problem to fix. No rows = nothing detected.'
SELECT s.slug AS store, issue
  FROM payment_gateway_configs c
  JOIN stores s ON s.id = c.store_id
 CROSS JOIN LATERAL (
     SELECT unnest(ARRAY[
         CASE WHEN NOT c.is_active
              THEN 'config exists but is_active=false — it will not appear at checkout' END,
         CASE WHEN c.mode NOT IN ('test','live')
              THEN 'mode ' || quote_literal(c.mode) || ' is invalid — payment.NewGateway fails closed on anything but live|test' END,
         CASE WHEN coalesce(btrim(c.api_key_encrypted),'') = ''
              THEN 'api_key_encrypted is empty — Cashfree will reject with 401' END,
         CASE WHEN coalesce(btrim(c.secret_key_encrypted),'') = ''
              THEN 'secret_key_encrypted is empty — Cashfree will reject with 401, and webhooks cannot be verified' END,
         CASE WHEN s.currency_code <> 'INR'
              THEN 'store currency is ' || s.currency_code || ' — Cashfree settles INR only and the adapter rejects the rest' END,
         CASE WHEN NOT EXISTS (
                  SELECT 1 FROM supported_countries sc
                   WHERE sc.country_code = s.country_code
                     AND 'cashfree' = ANY (sc.payment_providers))
              THEN 'cashfree missing from payment_providers for ' || s.country_code || ' — apply migration 000099' END,
         CASE WHEN c.tenant_id <> s.tenant_id
              THEN 'tenant_id does not match the store''s tenant — row was inserted incorrectly' END
     ]) AS issue
 ) checks
 WHERE c.provider = 'cashfree'
   AND issue IS NOT NULL
   AND (:'store_slug' = '' OR s.slug = :'store_slug')
 ORDER BY s.slug, issue;

\echo ''
\echo '=== 4. Payment transactions — is money actually flowing? ==='
\echo '    provider_intent_id holds the Cashfree order_id (= our order uuid);'
\echo '    it is what the refund and confirm paths key off, so a captured row'
\echo '    with it empty is unrefundable.'
SELECT t.status,
       count(*)                                                   AS transactions,
       count(*) FILTER (WHERE coalesce(t.provider_intent_id,'') = '')  AS missing_intent_id,
       count(*) FILTER (WHERE coalesce(t.provider_payment_id,'') = '') AS missing_payment_id,
       min(t.created_at)                                          AS first_seen,
       max(t.created_at)                                          AS last_seen
  FROM payment_transactions t
 WHERE t.provider = 'cashfree'
   AND (:'store_slug' = '' OR t.store_id = (SELECT id FROM stores WHERE slug = :'store_slug'))
 GROUP BY t.status
 ORDER BY t.status;

\echo ''
\echo '=== 5. Refunds — any stuck or failed against Cashfree? ==='
\echo '    Rows pending for a long time mean the saga sweeper is not clearing'
\echo '    them; check REFUND_GATEWAY_ENABLED and the marketplace-api logs.'
SELECT r.status,
       count(*)          AS refunds,
       sum(r.amount)     AS total_amount,
       max(r.created_at) AS last_created
  FROM refund_transactions r
 WHERE r.provider = 'cashfree'
 GROUP BY r.status
 ORDER BY r.status;

\echo ''
\echo '=== 6. Webhook deliveries — is Cashfree reaching us? ==='
\echo '    No rows means no verified delivery has ever landed: check that the'
\echo '    dashboard webhook points at /api/v1/webhooks/{storeSlug}/cashfree'
\echo '    and that the signing secret matches.'
SELECT event_type,
       count(*)          AS events,
       max(created_at)   AS last_received
  FROM webhook_events
 WHERE provider = 'cashfree'
 GROUP BY event_type
 ORDER BY event_type;
\echo ''
