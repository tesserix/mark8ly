-- 000069_store_branding_support_email.up.sql
-- §5.4 closed-store page needs a contact mailto. P12 reads this column to
-- render the "If you believe this is a mistake, please reach out" link.
-- Empty string (default) renders the page without a contact link.

ALTER TABLE store_branding
    ADD COLUMN IF NOT EXISTS support_email VARCHAR(255) NOT NULL DEFAULT '';
