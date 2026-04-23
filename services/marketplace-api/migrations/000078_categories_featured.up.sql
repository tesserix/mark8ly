-- 000078_categories_featured.up.sql
-- Mark8ly storefront editorial redesign (2026-04-24): stores can accumulate
-- hundreds of categories and a flat pill-filter list becomes unusable.
-- Merchants now curate a small set of "featured" categories that surface
-- on /products; everything else is reachable via the /categories browse
-- page. Default false so existing rows stay hidden until an admin opts
-- them in. Partial index keeps the featured lookup cheap on stores with
-- large taxonomies.

ALTER TABLE categories
    ADD COLUMN featured BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX categories_featured_per_store_idx
    ON categories (store_id, position, name)
    WHERE featured = true AND deleted_at IS NULL;

COMMENT ON COLUMN categories.featured IS
    'Storefront editorial flag: true → shown in the /products filter grid. False categories are still reachable via /categories browse.';
