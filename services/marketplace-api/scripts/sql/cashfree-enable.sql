-- cashfree-enable.sql — provision (or rotate) a store's Cashfree gateway config.
--
-- This is an OPERATIONAL script, deliberately NOT a migration: it writes tenant
-- data and credentials, which must never live in a file CI replays against
-- every environment. Migration 000099 handles the schema/seed half (adding
-- 'cashfree' to India's payment_providers allowlist); this handles the
-- per-merchant half.
--
-- The normal path is Admin → Settings → Payments, which stores credentials in
-- GCP Secret Manager and writes a "gsm://" reference. Use this script only when
-- the admin UI is not an option — seeding a UAT store, bootstrapping the first
-- merchant, or recovering a config out-of-band.
--
--   SECURITY: the values written here land in the api_key_encrypted /
--   secret_key_encrypted columns as PLAINTEXT. The application accepts that
--   (carriersecrets.HybridStore.Get treats an unrecognised shape as a legacy
--   plaintext value) and lazily rewraps the row to a gsm:// reference on the
--   first read that touches it — see maybeRewrapRow in payment_methods.go and
--   webhooks.go. Until that first read, the raw keys are readable in the
--   database, in this file, and in your shell history. Prefer the admin UI,
--   pass credentials via -v rather than editing this file, and rotate anything
--   that leaks.
--
-- Usage:
--   psql "$DATABASE_URL" \
--     -v store_slug=acme \
--     -v app_id='<Cashfree App ID>' \
--     -v secret_key='<Cashfree Secret Key>' \
--     -v webhook_secret='<optional dedicated webhook secret>' \
--     -v mode=test \
--     -v is_active=true \
--     -f scripts/sql/cashfree-enable.sql
--
-- store_slug, app_id and secret_key are required. webhook_secret defaults to
-- empty (Cashfree then signs with the secret key, which the app falls back to);
-- mode defaults to 'test'; is_active defaults to true.
--
-- Safe to re-run: the write is an upsert on the (store_id, provider) unique
-- constraint, so a second run rotates credentials rather than failing or
-- creating a duplicate config.

\set ON_ERROR_STOP on

-- Optional variables get their defaults here. :{?name} tests whether a psql
-- variable was supplied, so an omitted -v is a documented default rather than
-- an "unrecognised value :'mode'" syntax error.
\if :{?mode} \else \set mode 'test' \endif
\if :{?is_active} \else \set is_active 'true' \endif
\if :{?webhook_secret} \else \set webhook_secret '' \endif

BEGIN;

-- psql interpolates variables during lexing and does NOT substitute inside
-- dollar-quoted strings, so the inputs are staged in a temp table the DO block
-- below can read. That indirection is what lets the validation raise real
-- exceptions with useful messages instead of failing on a silent zero-row
-- INSERT ... SELECT when the slug does not exist.
CREATE TEMP TABLE cashfree_input ON COMMIT DROP AS
SELECT :'store_slug'::text     AS store_slug,
       :'app_id'::text         AS app_id,
       :'secret_key'::text     AS secret_key,
       :'webhook_secret'::text AS webhook_secret,
       :'mode'::text           AS mode,
       :'is_active'::boolean   AS is_active;

DO $$
DECLARE
    v_in      cashfree_input;
    v_store   RECORD;
    v_existed boolean;
BEGIN
    SELECT * INTO v_in FROM cashfree_input;

    IF coalesce(btrim(v_in.store_slug), '') = '' THEN
        RAISE EXCEPTION 'store_slug is required (-v store_slug=<slug>)';
    END IF;
    IF coalesce(btrim(v_in.app_id), '') = '' THEN
        RAISE EXCEPTION 'app_id is required — Cashfree''s x-client-id (-v app_id=...)';
    END IF;
    IF coalesce(btrim(v_in.secret_key), '') = '' THEN
        RAISE EXCEPTION 'secret_key is required — Cashfree''s x-client-secret (-v secret_key=...)';
    END IF;

    -- payment.NewGateway validates mode fail-closed and rejects anything that
    -- is not live|test, so a typo here would make every checkout 503 at
    -- gateway-init time. Catch it now instead.
    IF v_in.mode NOT IN ('test', 'live') THEN
        RAISE EXCEPTION 'mode must be test or live, got %', v_in.mode;
    END IF;

    SELECT id, tenant_id, country_code, currency_code, status
      INTO v_store
      FROM stores
     WHERE slug = v_in.store_slug;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'no store with slug % — check the slug, or the database you are pointed at', v_in.store_slug;
    END IF;

    -- The admin write path validates the provider against this same allowlist
    -- (settings.go containsProvider), and the storefront payment-methods
    -- endpoint filters through it too. Without migration 000099 a config row
    -- inserted here would be invisible at checkout and unmanageable in admin —
    -- a config that looks present but can never take a payment.
    IF NOT EXISTS (
        SELECT 1 FROM supported_countries
         WHERE country_code = v_store.country_code
           AND 'cashfree' = ANY (payment_providers)
    ) THEN
        RAISE EXCEPTION
            'cashfree is not in payment_providers for country % — apply migration 000099 before enabling it',
            v_store.country_code;
    END IF;

    -- Cashfree settles INR only, and the adapter rejects a mismatched currency
    -- outright rather than billing the wrong number. Advisory, not fatal: the
    -- store's currency can be changed after the gateway is configured.
    IF v_store.currency_code <> 'INR' THEN
        RAISE WARNING
            'store % is in % — Cashfree only settles INR, so every Cashfree payment on this store will be rejected',
            v_in.store_slug, v_store.currency_code;
    END IF;

    SELECT EXISTS (
        SELECT 1 FROM payment_gateway_configs
         WHERE store_id = v_store.id AND provider = 'cashfree'
    ) INTO v_existed;

    INSERT INTO payment_gateway_configs (
        tenant_id, store_id, provider,
        api_key_encrypted, secret_key_encrypted, webhook_secret_encrypted,
        mode, is_active
    )
    VALUES (
        v_store.tenant_id, v_store.id, 'cashfree',
        btrim(v_in.app_id), btrim(v_in.secret_key), nullif(btrim(v_in.webhook_secret), ''),
        v_in.mode, v_in.is_active
    )
    ON CONFLICT (store_id, provider) DO UPDATE
       SET api_key_encrypted    = EXCLUDED.api_key_encrypted,
           secret_key_encrypted = EXCLUDED.secret_key_encrypted,
           -- An omitted webhook_secret must not silently clear a secret the
           -- merchant already configured in admin: only overwrite when a new
           -- value was actually supplied.
           webhook_secret_encrypted = coalesce(
               EXCLUDED.webhook_secret_encrypted,
               payment_gateway_configs.webhook_secret_encrypted
           ),
           mode       = EXCLUDED.mode,
           is_active  = EXCLUDED.is_active,
           updated_at = now();

    RAISE NOTICE 'cashfree % for store % (tenant %, mode %, active %)',
        CASE WHEN v_existed THEN 'config UPDATED' ELSE 'config CREATED' END,
        v_in.store_slug, v_store.tenant_id, v_in.mode, v_in.is_active;
    RAISE NOTICE 'next: point the Cashfree dashboard webhook at POST /api/v1/webhooks/%/cashfree', v_in.store_slug;
END
$$;

COMMIT;

-- Echo the resulting row with credentials masked, so a run is verifiable
-- without printing secrets into a terminal or a CI log.
SELECT s.slug           AS store,
       c.provider,
       c.mode,
       c.is_active,
       left(c.api_key_encrypted, 6) || '…' AS api_key_prefix,
       c.secret_key_encrypted IS NOT NULL     AS has_secret_key,
       c.webhook_secret_encrypted IS NOT NULL AS has_webhook_secret,
       c.updated_at
  FROM payment_gateway_configs c
  JOIN stores s ON s.id = c.store_id
 WHERE s.slug = :'store_slug'
   AND c.provider = 'cashfree';
