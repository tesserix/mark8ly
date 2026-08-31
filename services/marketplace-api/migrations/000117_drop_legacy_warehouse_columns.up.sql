-- Contract half of the expand/contract migration 000095 opened (#177, #484).
--
-- 000095 created warehouses, backfilled it from these 8 columns, added
-- shipping_carrier_configs.warehouse_id, and deliberately left the old
-- columns in place because the running code still read them. Its own
-- comment set the condition for this migration: "the contract half (drop
-- the columns) only lands once every reader goes through warehouse_id."
--
-- That condition is now met:
--   * #480 moved 3 of the 6 read sites onto warehouse_id
--   * #486 moved the remaining 3 (checkout tax, auto pickup schedule,
--     manual pickup schedule), removed the last writer, and deleted the
--     Warehouse* fields from the models — so nothing in the Go code names
--     these columns any more
--   * #486 also shipped migration 000116, which backfilled every straggler
--     row that still had populated legacy columns and a NULL warehouse_id
--   * #486 was deployed to marketplace-api-admin, marketplace-api-storefront
--     and the CronJobs sharing that image BEFORE this migration was merged.
--     That ordering is the reason this is a separate PR: storefront and the
--     sweeper CronJobs roll independently of admin, which is the only
--     deployment carrying the migrate initContainer, so a pod on the old
--     image would otherwise still be reading columns this drops.
--
-- The addresses themselves are not lost — they live in warehouses, which is
-- where every reader now gets them.

ALTER TABLE shipping_carrier_configs
    DROP COLUMN IF EXISTS warehouse_name,
    DROP COLUMN IF EXISTS warehouse_line1,
    DROP COLUMN IF EXISTS warehouse_line2,
    DROP COLUMN IF EXISTS warehouse_city,
    DROP COLUMN IF EXISTS warehouse_region,
    DROP COLUMN IF EXISTS warehouse_postal,
    DROP COLUMN IF EXISTS warehouse_country,
    DROP COLUMN IF EXISTS warehouse_phone;
