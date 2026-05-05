-- bondi-product-media-lifestyle.sql
-- Adds a second product_media row per product (position=1) for the
-- lifestyle/atmosphere shot. Also doubles as the bondi IG grid pool.
\set ON_ERROR_STOP on

BEGIN;

INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/e14b4296-3166-4c3a-87bc-6e208129c909/bondi-linen-beach-shirt-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/e14b4296-3166-4c3a-87bc-6e208129c909/bondi-linen-beach-shirt-lifestyle.png', 'The Bondi Store — Bondi Linen Beach Shirt (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/e14b4296-3166-4c3a-87bc-6e208129c909/bondi-linen-beach-shirt-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bondi-linen-beach-shirt' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2d124cf2-71a2-44e1-b913-ec068e574534/tamarama-linen-slip-dress-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2d124cf2-71a2-44e1-b913-ec068e574534/tamarama-linen-slip-dress-lifestyle.png', 'The Bondi Store — Tamarama Linen Slip Dress (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2d124cf2-71a2-44e1-b913-ec068e574534/tamarama-linen-slip-dress-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'tamarama-linen-slip-dress' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6d3512ad-defa-444b-8260-7b234e3081cc/manly-linen-wide-leg-pants-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6d3512ad-defa-444b-8260-7b234e3081cc/manly-linen-wide-leg-pants-lifestyle.png', 'The Bondi Store — Manly Linen Wide Leg Pants (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6d3512ad-defa-444b-8260-7b234e3081cc/manly-linen-wide-leg-pants-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'manly-linen-wide-leg-pants' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ce16fe7c-f7bf-4673-824e-919b8b894cee/coogee-linen-beach-shorts-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ce16fe7c-f7bf-4673-824e-919b8b894cee/coogee-linen-beach-shorts-lifestyle.png', 'The Bondi Store — Coogee Linen Beach Shorts (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ce16fe7c-f7bf-4673-824e-919b8b894cee/coogee-linen-beach-shorts-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coogee-linen-beach-shorts' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/cc39b932-ac3c-43d3-a47d-fb359aa0f809/palm-beach-linen-robe-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/cc39b932-ac3c-43d3-a47d-fb359aa0f809/palm-beach-linen-robe-lifestyle.png', 'The Bondi Store — Palm Beach Linen Robe (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/cc39b932-ac3c-43d3-a47d-fb359aa0f809/palm-beach-linen-robe-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'palm-beach-linen-robe' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/12f7abae-dcf0-4660-a786-c25ab1450b22/coastal-cotton-crew-tee-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/12f7abae-dcf0-4660-a786-c25ab1450b22/coastal-cotton-crew-tee-lifestyle.png', 'The Bondi Store — Coastal Cotton Crew Tee (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/12f7abae-dcf0-4660-a786-c25ab1450b22/coastal-cotton-crew-tee-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coastal-cotton-crew-tee' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a20b5423-8299-440e-920f-0be41eec60b6/pacific-cotton-tank-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a20b5423-8299-440e-920f-0be41eec60b6/pacific-cotton-tank-lifestyle.png', 'The Bondi Store — Pacific Cotton Tank (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a20b5423-8299-440e-920f-0be41eec60b6/pacific-cotton-tank-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'pacific-cotton-tank' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/3bdbba2d-7326-4ab3-a200-b561aef47d75/bronte-stretch-denim-shorts-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/3bdbba2d-7326-4ab3-a200-b561aef47d75/bronte-stretch-denim-shorts-lifestyle.png', 'The Bondi Store — Bronte Stretch Denim Shorts (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/3bdbba2d-7326-4ab3-a200-b561aef47d75/bronte-stretch-denim-shorts-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bronte-stretch-denim-shorts' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/1a4e65af-c5dd-437a-ab6c-89aada15c23e/sandstone-ceramic-mug-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/1a4e65af-c5dd-437a-ab6c-89aada15c23e/sandstone-ceramic-mug-lifestyle.png', 'The Bondi Store — Sandstone Ceramic Mug (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/1a4e65af-c5dd-437a-ab6c-89aada15c23e/sandstone-ceramic-mug-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'sandstone-ceramic-mug' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d559f554-8079-4caa-ad9d-09b4c47297af/coastal-soy-candle-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d559f554-8079-4caa-ad9d-09b4c47297af/coastal-soy-candle-lifestyle.png', 'The Bondi Store — Coastal Soy Candle (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d559f554-8079-4caa-ad9d-09b4c47297af/coastal-soy-candle-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coastal-soy-candle' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/7c47a0ac-44c9-4fa1-9a37-39e21d79aa1b/eucalyptus-body-oil-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/7c47a0ac-44c9-4fa1-9a37-39e21d79aa1b/eucalyptus-body-oil-lifestyle.png', 'The Bondi Store — Eucalyptus Body Oil (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/7c47a0ac-44c9-4fa1-9a37-39e21d79aa1b/eucalyptus-body-oil-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'eucalyptus-body-oil' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/722f466c-aada-4b97-a10b-4069dabb102f/bondi-beach-cotton-towel-lifestyle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/722f466c-aada-4b97-a10b-4069dabb102f/bondi-beach-cotton-towel-lifestyle.png', 'The Bondi Store — Bondi Beach Cotton Towel (lifestyle)', 1, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/722f466c-aada-4b97-a10b-4069dabb102f/bondi-beach-cotton-towel-lifestyle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bondi-beach-cotton-towel' AND deleted_at IS NULL;

DO $$ DECLARE n int; BEGIN
  SELECT count(*) INTO n FROM product_media pm
    JOIN products p ON p.id = pm.product_id
   WHERE p.store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND p.deleted_at IS NULL AND pm.position = 1;
  RAISE NOTICE 'lifestyle media wired: %', n;
  IF n <> 12 THEN RAISE EXCEPTION 'expected 12 lifestyle rows, got %', n; END IF;
END $$;

COMMIT;
