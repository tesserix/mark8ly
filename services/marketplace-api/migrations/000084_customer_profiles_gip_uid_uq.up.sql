-- 000084: Partial unique index on (store_id, gip_uid) so Phase 2/3
-- Google sign-in cannot create duplicate customer_profiles rows in the
-- race window. WHERE clause keeps existing rows with NULL gip_uid
-- (password-only signups) compatible.
--
-- Note: CREATE INDEX CONCURRENTLY cannot run inside a transaction.
-- golang-migrate's postgres driver wraps each file in a BEGIN/COMMIT
-- by default, so we use a plain CREATE UNIQUE INDEX here (consistent
-- with the rest of this service's migrations). The index is still a
-- non-blocking operation for small-to-medium tables.
BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS customer_profiles_store_gip_uid_uq
    ON customer_profiles (store_id, gip_uid)
    WHERE gip_uid IS NOT NULL;

COMMIT;
