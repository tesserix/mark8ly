DROP INDEX IF EXISTS idx_invitations_store;
ALTER TABLE invitations DROP COLUMN IF EXISTS store_id;
