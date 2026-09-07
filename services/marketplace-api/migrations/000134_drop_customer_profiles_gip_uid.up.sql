-- Drops customer_profiles.gip_uid and its partial unique index (#793).
--
-- The column was the customer identity key under GIP. Every reader and writer
-- was removed in #814 (deployed before this migration, per the code-first rule
-- the whole GIP removal followed): sessionClaims.UIDOrGipUID became
-- IdentityUID and returns the Zitadel uid only, GetProfileByGipUID had no
-- callers, and the upsert no longer writes the column.
--
-- Data: 1 of 5 production rows carried a non-null gip_uid when this was
-- written. That customer's session already fails closed after #814 (a cookie
-- carrying only a gip_uid is rejected rather than admitted with an empty
-- identity), so they re-authenticate and are matched on their verified email
-- like everyone else. Nothing is silently rebound.
--
-- The index goes with the column. 000084 created it to close a Phase 2/3 race
-- on (store_id, gip_uid); with the column always NULL since #814 it constrains
-- nothing, and Postgres would drop it with the column regardless -- naming it
-- here keeps the intent explicit rather than incidental.
DROP INDEX IF EXISTS customer_profiles_store_gip_uid_uq;
ALTER TABLE customer_profiles DROP COLUMN IF EXISTS gip_uid;
