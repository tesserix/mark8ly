-- 000053_tax_id_window_shortened_at.down.sql
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS tax_id_window_shortened_at;
