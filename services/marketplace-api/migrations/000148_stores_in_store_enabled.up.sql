-- In-store sales on/off per store (#1101). Off for new stores; on for any
-- store that has already rung up an in-store sale, so nobody loses the
-- sale screen they are using.
ALTER TABLE stores ADD COLUMN in_store_enabled boolean NOT NULL DEFAULT false;
UPDATE stores SET in_store_enabled = true
 WHERE id IN (SELECT DISTINCT store_id FROM orders WHERE channel = 'in_store');
