-- 000053_tax_id_window_shortened_at.up.sql
-- NULL = normal 14d window; non-NULL = 48h shortened window following a migration fast-path approval.
-- Clock is the merchant's signup_date, NOT approval time — we honour the original window origin.
-- P7 reads this column; P5 only writes it.
ALTER TABLE store_subscriptions
    ADD COLUMN tax_id_window_shortened_at TIMESTAMPTZ NULL;
