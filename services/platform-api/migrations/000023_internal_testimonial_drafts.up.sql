SET lock_timeout = '5s';
SET statement_timeout = '30s';

ALTER TABLE merchant_testimonials ADD COLUMN id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE merchant_testimonials ADD COLUMN store_name TEXT;
ALTER TABLE merchant_testimonials DROP CONSTRAINT merchant_testimonials_pkey;
ALTER TABLE merchant_testimonials ADD PRIMARY KEY (id);
ALTER TABLE merchant_testimonials ALTER COLUMN store_id DROP NOT NULL;
ALTER TABLE merchant_testimonials ADD CONSTRAINT merchant_testimonials_store_id_key UNIQUE (store_id);
ALTER TABLE merchant_testimonials ADD CONSTRAINT merchant_testimonials_draft_check
    CHECK (store_id IS NOT NULL OR (
        store_name IS NOT NULL AND char_length(btrim(store_name)) BETWEEN 1 AND 200 AND NOT published
    ));

CREATE UNIQUE INDEX merchant_testimonials_unlinked_name_idx
    ON merchant_testimonials (lower(store_name)) WHERE store_id IS NULL;

-- Internal proposed copy is never published without a registered merchant's consent.
WITH drafts(store_name, quote) AS (VALUES
    ('Mugshot Studio', 'Thank you, Mark8ly, for helping me set up Mugshot Studio. Adding my mugs and getting the shop ready felt simple, so I could spend more time on my designs.'),
    ('Buttercup Bakes', 'Mark8ly helped me get Buttercup Bakes online without a long setup. Thank you for making it easier to share my bakes and start building my shop.'),
    ('Daily Basket', 'Thank you, Mark8ly, for helping me bring Daily Basket online. Having a clear place to add products and manage the shop helped me get started faster.'),
    ('The Bondi Store', 'Thank you, Mark8ly, for helping us set up The Bondi Store. Getting our products online was straightforward, and having the shop in one place made it easier to get started.')
)
INSERT INTO merchant_testimonials (store_id, store_name, position, quote, rating)
SELECT (
    SELECT s.id FROM stores s WHERE lower(s.name) = lower(d.store_name)
    AND (SELECT count(*) FROM stores other WHERE lower(other.name) = lower(d.store_name)) = 1
    LIMIT 1
), d.store_name, 'Store owner', d.quote, 5
FROM drafts d
WHERE NOT EXISTS (
    SELECT 1 FROM merchant_testimonials r LEFT JOIN stores s ON s.id = r.store_id
    WHERE lower(COALESCE(s.name, r.store_name)) = lower(d.store_name)
)
ON CONFLICT DO NOTHING;
