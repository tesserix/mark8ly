-- Deliberately NOT reversible.
--
-- Reversing would mean summing every warehouse row back onto the sentinel,
-- which cannot distinguish stock this migration moved from a per-warehouse
-- split the merchant made themselves through the product form (#177 PR 5e).
-- A merchant who genuinely holds 10 in Sydney and 5 in Melbourne would come
-- back with 15 units in one undifferentiated pile and no record of the
-- split — silent, and impossible to reconstruct.
--
-- Failing loudly is the honest answer. To roll back, restore from a
-- snapshot taken before the up migration ran.
DO $$
BEGIN
    RAISE EXCEPTION
        'migration 000123 is not reversible: rolling the sentinel backfill back would merge merchant-entered per-warehouse splits into one pile. Restore from a pre-migration snapshot instead.';
END $$;
