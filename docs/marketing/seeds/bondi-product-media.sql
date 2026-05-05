-- bondi-product-media.sql
-- Wires the 12 newly uploaded GCS images into the product_media table.
-- Run AFTER uploading the PNGs to gs://.../products/media/<folder>/<handle>.png
\set ON_ERROR_STOP on

BEGIN;

INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ba8f69c7-2f71-4a8d-b792-150d753b4ce4/bondi-linen-beach-shirt.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ba8f69c7-2f71-4a8d-b792-150d753b4ce4/bondi-linen-beach-shirt.png', 'The Bondi Store — Bondi Linen Beach Shirt', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/ba8f69c7-2f71-4a8d-b792-150d753b4ce4/bondi-linen-beach-shirt.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bondi-linen-beach-shirt' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/f40db3bb-ceed-4df6-b44e-64566a5d8b48/tamarama-linen-slip-dress.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/f40db3bb-ceed-4df6-b44e-64566a5d8b48/tamarama-linen-slip-dress.png', 'The Bondi Store — Tamarama Linen Slip Dress', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/f40db3bb-ceed-4df6-b44e-64566a5d8b48/tamarama-linen-slip-dress.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'tamarama-linen-slip-dress' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2fb2efc6-baf5-4cf8-b8e2-486b4536948b/manly-linen-wide-leg-pants.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2fb2efc6-baf5-4cf8-b8e2-486b4536948b/manly-linen-wide-leg-pants.png', 'The Bondi Store — Manly Linen Wide Leg Pants', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2fb2efc6-baf5-4cf8-b8e2-486b4536948b/manly-linen-wide-leg-pants.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'manly-linen-wide-leg-pants' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6ea68b2c-ea3d-4045-b4df-dabdd667d2bf/coogee-linen-beach-shorts.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6ea68b2c-ea3d-4045-b4df-dabdd667d2bf/coogee-linen-beach-shorts.png', 'The Bondi Store — Coogee Linen Beach Shorts', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/6ea68b2c-ea3d-4045-b4df-dabdd667d2bf/coogee-linen-beach-shorts.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coogee-linen-beach-shorts' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/dc1fda50-9efd-4136-835b-445947ab6bbb/palm-beach-linen-robe.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/dc1fda50-9efd-4136-835b-445947ab6bbb/palm-beach-linen-robe.png', 'The Bondi Store — Palm Beach Linen Robe', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/dc1fda50-9efd-4136-835b-445947ab6bbb/palm-beach-linen-robe.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'palm-beach-linen-robe' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d4686e0d-0ed0-4ce1-8a76-54b3cba95854/coastal-cotton-crew-tee.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d4686e0d-0ed0-4ce1-8a76-54b3cba95854/coastal-cotton-crew-tee.png', 'The Bondi Store — Coastal Cotton Crew Tee', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d4686e0d-0ed0-4ce1-8a76-54b3cba95854/coastal-cotton-crew-tee.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coastal-cotton-crew-tee' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a0585650-73b6-45a0-9cfb-dbef8cec4edf/pacific-cotton-tank.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a0585650-73b6-45a0-9cfb-dbef8cec4edf/pacific-cotton-tank.png', 'The Bondi Store — Pacific Cotton Tank', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/a0585650-73b6-45a0-9cfb-dbef8cec4edf/pacific-cotton-tank.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'pacific-cotton-tank' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d6fb1d09-e7b4-4ec8-b2d4-0d60f155885f/bronte-stretch-denim-shorts.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d6fb1d09-e7b4-4ec8-b2d4-0d60f155885f/bronte-stretch-denim-shorts.png', 'The Bondi Store — Bronte Stretch Denim Shorts', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/d6fb1d09-e7b4-4ec8-b2d4-0d60f155885f/bronte-stretch-denim-shorts.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bronte-stretch-denim-shorts' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/c6f0ac8e-60e9-4301-9ebe-c7fe1c8654e6/sandstone-ceramic-mug.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/c6f0ac8e-60e9-4301-9ebe-c7fe1c8654e6/sandstone-ceramic-mug.png', 'The Bondi Store — Sandstone Ceramic Mug', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/c6f0ac8e-60e9-4301-9ebe-c7fe1c8654e6/sandstone-ceramic-mug.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'sandstone-ceramic-mug' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/db9bcb7c-4081-4cdd-973b-5d560ea3bbd5/coastal-soy-candle.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/db9bcb7c-4081-4cdd-973b-5d560ea3bbd5/coastal-soy-candle.png', 'The Bondi Store — Coastal Soy Candle', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/db9bcb7c-4081-4cdd-973b-5d560ea3bbd5/coastal-soy-candle.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'coastal-soy-candle' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/06429e20-3565-4594-8b66-8996350876c6/eucalyptus-body-oil.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/06429e20-3565-4594-8b66-8996350876c6/eucalyptus-body-oil.png', 'The Bondi Store — Eucalyptus Body Oil', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/06429e20-3565-4594-8b66-8996350876c6/eucalyptus-body-oil.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'eucalyptus-body-oil' AND deleted_at IS NULL;
INSERT INTO product_media (product_id, url, storage_key, alt, position, media_type, gcs_path_original)
  SELECT id, 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/37d4348a-5ba7-4adc-8fd7-13baa6759914/bondi-beach-cotton-towel.png', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/37d4348a-5ba7-4adc-8fd7-13baa6759914/bondi-beach-cotton-towel.png', 'The Bondi Store — Bondi Beach Cotton Towel', 0, 'image', 'tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/37d4348a-5ba7-4adc-8fd7-13baa6759914/bondi-beach-cotton-towel.png'
    FROM products WHERE store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND handle = 'bondi-beach-cotton-towel' AND deleted_at IS NULL;

-- Sanity: confirm all 12 products got media
DO $$
DECLARE
  with_media int;
BEGIN
  SELECT count(DISTINCT pm.product_id) INTO with_media
    FROM product_media pm JOIN products p ON p.id = pm.product_id
   WHERE p.store_id = '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid AND p.deleted_at IS NULL AND p.status = 'active';
  RAISE NOTICE 'product_media wired: % active products have at least one image', with_media;
  IF with_media <> 12 THEN
    RAISE EXCEPTION 'expected 12 products with media, got %', with_media;
  END IF;
END $$;

COMMIT;
