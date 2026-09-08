-- 000136_stores_created_at.up.sql
--
-- The stores projection mirrors platform_api.stores but never carried WHEN a
-- store was created, only synced_at — the time this service last copied the
-- row, which moves on every upsert.
--
-- That gap is not cosmetic. #827 dates a backfilled trial from the store's
-- creation, and cmd/backfill-trial-start selected s.created_at from a table
-- that has no such column: it would have failed on its first real run.
--
-- Nullable on purpose. Rows mirrored before this column existed genuinely do
-- not know their creation time, and NULL says so. A DEFAULT now() would have
-- stamped every existing row with the migration's own timestamp — which reads
-- as a fact and would date those merchants' trials from a schema change.
ALTER TABLE stores ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;

COMMENT ON COLUMN stores.created_at IS
  'When the store was created in platform_api. NULL for rows mirrored before migration 136. Distinct from synced_at, which is when this projection last copied the row.';
