-- Restores the column and its partial unique index.
--
-- Values are NOT recoverable: the GIP identities they held are gone with the
-- Identity Platform tenant pools, and nothing has written this column since
-- #814. A rollback gives back the shape, not the data.
ALTER TABLE customer_profiles ADD COLUMN IF NOT EXISTS gip_uid TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS customer_profiles_store_gip_uid_uq
    ON customer_profiles (store_id, gip_uid)
    WHERE gip_uid IS NOT NULL;
