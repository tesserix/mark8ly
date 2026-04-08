-- Phase R — store-level invitations.
--
-- Extends the Phase P invitations table with a nullable store_id.
-- When store_id is set, the accept endpoint writes a store-level
-- FGA tuple (e.g. user:X manager store:Y); when null, it keeps the
-- tenant-level behaviour from Phase P (user:X manager tenant:Y).
--
-- Both shapes coexist — the invite modal lets the merchant choose
-- "tenant-wide" (null) or "specific store" (set).

ALTER TABLE invitations
    ADD COLUMN store_id UUID REFERENCES stores(id) ON DELETE CASCADE;

CREATE INDEX idx_invitations_store ON invitations (store_id)
    WHERE store_id IS NOT NULL;
