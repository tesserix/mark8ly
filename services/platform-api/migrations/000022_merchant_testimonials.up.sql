CREATE TABLE merchant_testimonials (
    store_id UUID PRIMARY KEY REFERENCES stores(id) ON DELETE CASCADE,
    author_name TEXT NOT NULL CHECK (char_length(btrim(author_name)) BETWEEN 1 AND 100),
    quote TEXT NOT NULL CHECK (char_length(btrim(quote)) BETWEEN 1 AND 1000),
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    published BOOLEAN NOT NULL DEFAULT false,
    consented_at TIMESTAMPTZ,
    submitted_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT published OR (consented_at IS NOT NULL AND submitted_by IS NOT NULL))
);

CREATE INDEX merchant_testimonials_published_idx
    ON merchant_testimonials (updated_at DESC, store_id) WHERE published;

-- Proposed copy only: a merchant must review it and explicitly consent in admin.
WITH drafts(store_name, author_name, quote) AS (VALUES
    ('Mugshot Studio', 'Abilipsa Naik', 'Thank you, Mark8ly, for helping me set up Mugshot Studio. Adding my mugs and getting the shop ready felt simple, so I could spend more time on my designs.'),
    ('Buttercup Bakes', 'Stithapragyan Behuria', 'Mark8ly helped me get Buttercup Bakes online without a long setup. Thank you for making it easier to share my bakes and start building my shop.'),
    ('Daily Basket', 'Sampurna Rout', 'Thank you, Mark8ly, for helping me bring Daily Basket online. Having a clear place to add products and manage the shop helped me get started faster.')
)
INSERT INTO merchant_testimonials (store_id, author_name, quote, rating)
SELECT s.id, d.author_name, d.quote, 5
FROM drafts d JOIN stores s ON lower(s.name) = lower(d.store_name)
WHERE (SELECT count(*) FROM stores other WHERE lower(other.name) = lower(d.store_name)) = 1
ON CONFLICT (store_id) DO NOTHING;
