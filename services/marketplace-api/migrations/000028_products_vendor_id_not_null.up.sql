-- 000028_products_vendor_id_not_null.up.sql
--
-- Phase 1 final step: products.vendor_id is populated for every row
-- by migration 000027. Lock it down now that:
--   - the vendors table exists,
--   - every existing tenant has a self-vendor (migration backfill +
--     platform-api backfill CLI),
--   - onboarding.Complete calls EnsureSelfVendor for new tenants,
--   - product.Create (Task 13) defaults vendor_id to the self-vendor.

-- Safety check: abort loudly if any product is still missing a vendor_id.
-- This should never fire in healthy environments; if it does we fix the
-- backfill first.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM products WHERE vendor_id IS NULL) THEN
        RAISE EXCEPTION 'products rows with NULL vendor_id remain; run 000027 backfill first';
    END IF;
END $$;

ALTER TABLE products ALTER COLUMN vendor_id SET NOT NULL;
