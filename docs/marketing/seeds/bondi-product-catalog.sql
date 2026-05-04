-- bondi-product-catalog.sql
-- Rebuilds the_bondi_store catalog from scratch:
--   * Phase A: archive all existing products + categories (soft-delete)
--   * Phase B: defensively zero any non-zero variant_stock
--   * Phase C: insert 3 new categories (Linen, Cotton, Lifestyle)
--   * Phase D: insert 12 on-brand products with full variants
--   * Phase E: zero-fill variant_stock for the new variants
--
-- All new variants ship with inventory_quantity=0 and
-- inventory_policy='deny' so checkout refuses any order until the
-- merchant flips stock manually. Single transaction — partial
-- failure rolls back fully.
--
-- Tenant: 8c302556-b647-4824-8ce4-73f547ca456e
-- Store:  8b69eea9-2537-4d36-9d99-bafcbad02dbc (slug=the-bondi-store, currency=AUD)
-- Default location: 00000000-0000-0000-0000-000000000001

\set ON_ERROR_STOP on

BEGIN;

-- ──────────────────────────────────────────────────────────────────
-- Pre-flight: confirm exactly one store under this tenant
-- ──────────────────────────────────────────────────────────────────
DO $$
DECLARE
  store_count int;
BEGIN
  SELECT count(*) INTO store_count
   FROM stores WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid;
  IF store_count <> 1 THEN
    RAISE EXCEPTION 'Expected exactly 1 store, found %.', store_count;
  END IF;
END $$;

-- ──────────────────────────────────────────────────────────────────
-- Phase A: Archive all existing products + categories (soft-delete)
-- ──────────────────────────────────────────────────────────────────
UPDATE products
   SET status = 'archived',
       deleted_at = NOW(),
       updated_at = NOW()
 WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid
   AND deleted_at IS NULL;

UPDATE categories
   SET is_active = false,
       deleted_at = NOW(),
       updated_at = NOW()
 WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid
   AND deleted_at IS NULL;

-- ──────────────────────────────────────────────────────────────────
-- Phase B: Zero any leftover non-zero inventory (defensive)
-- ──────────────────────────────────────────────────────────────────
UPDATE variant_stock vs
   SET quantity = 0,
       updated_at = NOW()
  FROM product_variants pv
  JOIN products p ON p.id = pv.product_id
 WHERE vs.variant_id = pv.id
   AND p.tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid
   AND vs.quantity > 0;

-- ──────────────────────────────────────────────────────────────────
-- Phase C: Insert 3 new categories
-- ──────────────────────────────────────────────────────────────────
INSERT INTO categories (id, tenant_id, store_id, name, slug, description, position, is_active)
VALUES ('bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'Linen', 'linen', 'Made for the long Bondi summer.', 10, true);
INSERT INTO categories (id, tenant_id, store_id, name, slug, description, position, is_active)
VALUES ('23b8c1e9-3924-46de-beb1-3b9046685257'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'Cotton & Knit', 'cotton', 'Everyday weight, garment-dyed.', 20, true);
INSERT INTO categories (id, tenant_id, store_id, name, slug, description, position, is_active)
VALUES ('bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'Lifestyle', 'lifestyle', 'Small batch, considered objects.', 30, true);

-- ──────────────────────────────────────────────────────────────────
-- Phase D + E: Insert products + variants + variant_stock
-- ──────────────────────────────────────────────────────────────────

-- ── Bondi Linen Beach Shirt ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'bondi-linen-beach-shirt', 'Bondi Linen Beach Shirt', 'Lightweight 100% European linen shirt cut for long Sydney summers. A relaxed unisex fit with a soft camp collar and mother-of-pearl buttons. Pre-washed for a worn-in hand. Wears well over swimwear, under a jacket, or alone — and keeps getting better with each year on the hanger.', 'active', ARRAY['linen', 'shirt', 'summer', 'unisex']::text[], 'Bondi Linen Beach Shirt — The Bondi Store', 'Relaxed unisex linen beach shirt in pure European linen. Pre-washed, mother-of-pearl buttons.', 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('972a8469-1641-4f82-8b9d-2434e465e150'::uuid, 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('17fc695a-07a0-4a6e-8822-e8f36c031199'::uuid, '972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BLBS-XS', 149.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('9a1de644-815e-46d1-bb8f-aa1837f8a88b'::uuid, '972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BLBS-S', 149.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('b74d0fb1-32e7-4629-8fad-c1a606cb0fb3'::uuid, '972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BLBS-M', 149.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('6b65a6a4-8b81-48f6-b38a-088ca65ed389'::uuid, '972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BLBS-L', 149.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('47378190-96da-4dac-b2ff-5d2a386ecbe0'::uuid, '972a8469-1641-4f82-8b9d-2434e465e150'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BLBS-XL', 149.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('17fc695a-07a0-4a6e-8822-e8f36c031199'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('9a1de644-815e-46d1-bb8f-aa1837f8a88b'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('b74d0fb1-32e7-4629-8fad-c1a606cb0fb3'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('6b65a6a4-8b81-48f6-b38a-088ca65ed389'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('47378190-96da-4dac-b2ff-5d2a386ecbe0'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Tamarama Linen Slip Dress ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'tamarama-linen-slip-dress', 'Tamarama Linen Slip Dress', 'Bias-cut midi dress in stonewashed European linen. Slim straps, a hidden side-seam pocket, soft drape that catches the breeze. Designed to layer with a tee underneath when the wind comes off the headland, or to wear straight to dinner.', 'active', ARRAY['linen', 'dress', 'summer']::text[], 'Tamarama Linen Slip Dress — The Bondi Store', 'Bias-cut midi linen slip dress, stonewashed European linen with hidden pockets.', 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('6c307511-b2b9-437a-a8df-6ec4ce4a2bbd'::uuid, 'c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-TLSD-XS', 179.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('371ecd7b-27cd-4130-8722-9389571aa876'::uuid, 'c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-TLSD-S', 179.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('1a2a73ed-562b-4f79-8374-59eef50bea63'::uuid, 'c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-TLSD-M', 179.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('5be6128e-18c2-4797-a142-ea7d17be3111'::uuid, 'c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-TLSD-L', 179.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('43b7a3a6-9a8d-4a03-980d-7b71d8f56413'::uuid, 'c241330b-01a9-471f-9e8a-774bcf36d58b'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-TLSD-XL', 179.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('6c307511-b2b9-437a-a8df-6ec4ce4a2bbd'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('371ecd7b-27cd-4130-8722-9389571aa876'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('1a2a73ed-562b-4f79-8374-59eef50bea63'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('5be6128e-18c2-4797-a142-ea7d17be3111'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('43b7a3a6-9a8d-4a03-980d-7b71d8f56413'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Manly Linen Wide-Leg Pants ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'manly-linen-wide-leg-pants', 'Manly Linen Wide-Leg Pants', 'High-rise linen trousers with a generous wide leg and side-seam pockets. The waist sits at the natural waistline with a clean elasticated back panel for comfort and shape. Cut from heavyweight European linen that softens with every wash.', 'active', ARRAY['linen', 'pants', 'summer']::text[], 'Manly Linen Wide-Leg Pants — The Bondi Store', 'High-rise wide-leg linen pants in heavyweight European linen.', 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('ec1b8ca1-f91e-4d4c-9ff4-9b7889463e85'::uuid, '759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-MLWLP-XS', 169.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('4b0dbb41-8d52-48f1-942c-3fe860e7a113'::uuid, '759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-MLWLP-S', 169.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('e2acf72f-9e57-4f7a-a0ee-89aed453dd32'::uuid, '759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-MLWLP-M', 169.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('3139d32c-93cd-49bf-9c94-1cf0dc98d2c1'::uuid, '759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-MLWLP-L', 169.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('a9488d99-0bbb-4599-91ce-5dd2b45ed1f0'::uuid, '759cde66-bacf-43d0-8b1f-9163ce9ff57f'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-MLWLP-XL', 169.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('ec1b8ca1-f91e-4d4c-9ff4-9b7889463e85'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('4b0dbb41-8d52-48f1-942c-3fe860e7a113'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('e2acf72f-9e57-4f7a-a0ee-89aed453dd32'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('3139d32c-93cd-49bf-9c94-1cf0dc98d2c1'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('a9488d99-0bbb-4599-91ce-5dd2b45ed1f0'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Coogee Linen Beach Shorts ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'coogee-linen-beach-shorts', 'Coogee Linen Beach Shorts', 'Drawstring linen shorts in a relaxed unisex cut. Patch pockets at the front, a single welt at the back, and an elastic waist that doesn''t bite. Made to dry quickly in the sun and rumple beautifully through the day.', 'active', ARRAY['linen', 'shorts', 'unisex', 'beach']::text[], 'Coogee Linen Beach Shorts — The Bondi Store', 'Unisex drawstring linen shorts. Relaxed cut, quick-drying European linen.', 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('ddd1dfb2-3b98-4ef8-9af6-1a26146d3f31'::uuid, 'fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CLBS-XS', 89.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('7412b293-4729-4739-a14f-f3d719db3ad0'::uuid, 'fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CLBS-S', 89.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('29a3b2e9-5d65-4441-9588-42dea2bc372f'::uuid, 'fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CLBS-M', 89.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('ab9099a4-35a2-40ae-9af3-05535ec42e08'::uuid, 'fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CLBS-L', 89.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('aefcfad8-efc8-4849-b3aa-7efe4458a885'::uuid, 'fc377a4c-4a15-444d-85e7-ce8a3a578a8e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CLBS-XL', 89.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('ddd1dfb2-3b98-4ef8-9af6-1a26146d3f31'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('7412b293-4729-4739-a14f-f3d719db3ad0'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('29a3b2e9-5d65-4441-9588-42dea2bc372f'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('ab9099a4-35a2-40ae-9af3-05535ec42e08'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('aefcfad8-efc8-4849-b3aa-7efe4458a885'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Palm Beach Linen Robe ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('a28defe3-9bf0-4273-9247-6f57a5e5a5ab'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'palm-beach-linen-robe', 'Palm Beach Linen Robe', 'An open-front linen robe to throw over swimwear. V-neck, three-quarter sleeves, and an exaggerated tie at the waist that you can knot once or wrap twice. Cuts a clean line from the towel to the bar.', 'active', ARRAY['linen', 'robe', 'cover-up', 'beach']::text[], 'Palm Beach Linen Robe — The Bondi Store', 'Open-front linen beach robe with tie waist. Two sizes: XS-S, M-L.', 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('a28defe3-9bf0-4273-9247-6f57a5e5a5ab'::uuid, 'bdd640fb-0667-4ad1-9c80-317fa3b1799d'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('3eabedcb-baa8-4dd4-88bd-64072bcfbe01'::uuid, 'a28defe3-9bf0-4273-9247-6f57a5e5a5ab'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PBLR-XS-S', 199.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('451b4cf3-6123-4df7-b656-af7229d4beef'::uuid, 'a28defe3-9bf0-4273-9247-6f57a5e5a5ab'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PBLR-M-L', 199.00, 'AUD', 0, 'deny', 1, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('3eabedcb-baa8-4dd4-88bd-64072bcfbe01'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('451b4cf3-6123-4df7-b656-af7229d4beef'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Coastal Cotton Crew Tee ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'coastal-cotton-crew-tee', 'Coastal Cotton Crew Tee', 'An everyday tee cut from heavyweight 220gsm organic cotton. Pre-washed for softness, double-stitched at the hem, and a relaxed crew neck that doesn''t cling. Designed and printed in Sydney.', 'active', ARRAY['cotton', 'tee', 'everyday']::text[], 'Coastal Cotton Crew Tee — The Bondi Store', 'Heavyweight 220gsm organic cotton crew tee, made in Sydney.', '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('5304317f-af42-412f-b838-b3268e944239'::uuid, 'b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CCCT-XS', 65.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('0e51f30d-c6a7-4e39-84b0-32ccd7c524a5'::uuid, 'b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CCCT-S', 65.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('ce177b4e-0837-48a3-9261-a7ab3aa2e4f9'::uuid, 'b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CCCT-M', 65.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('10f1bc81-448a-4a9e-a6b2-bc5b50c187fc'::uuid, 'b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CCCT-L', 65.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('9132b63e-f162-47e4-a9c3-49e03602f8ac'::uuid, 'b02b61c4-a3d7-4628-ace6-6fa2fd5166e6'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CCCT-XL', 65.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('5304317f-af42-412f-b838-b3268e944239'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('0e51f30d-c6a7-4e39-84b0-32ccd7c524a5'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('ce177b4e-0837-48a3-9261-a7ab3aa2e4f9'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('10f1bc81-448a-4a9e-a6b2-bc5b50c187fc'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('9132b63e-f162-47e4-a9c3-49e03602f8ac'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Pacific Cotton Tank ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'pacific-cotton-tank', 'Pacific Cotton Tank', 'Heavyweight ribbed cotton tank with a relaxed armhole and a trim band at the neckline. Designed to wear loose and untucked on the warmest days. Garment-dyed for a soft, slightly faded hand right out of the box.', 'active', ARRAY['cotton', 'tank', 'summer']::text[], 'Pacific Cotton Tank — The Bondi Store', 'Ribbed cotton tank, heavyweight, garment-dyed for a soft hand.', '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('e27a984d-6548-41d0-bfcd-9eb1a7cad415'::uuid, '366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PCT-XS', 49.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('24933b83-7577-40a9-a491-f0b2ea1fca65'::uuid, '366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PCT-S', 49.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('beb79919-3f22-4af8-a3be-d01d43cf2fde'::uuid, '366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PCT-M', 49.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('bf3c4c06-4343-48bc-89fa-6a688fb5d27b'::uuid, '366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PCT-L', 49.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('956269f0-e5d7-4875-adad-d6c795a76d79'::uuid, '366eb16f-508e-4ad7-b7c9-3acfe059a0ee'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-PCT-XL', 49.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('e27a984d-6548-41d0-bfcd-9eb1a7cad415'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('24933b83-7577-40a9-a491-f0b2ea1fca65'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('beb79919-3f22-4af8-a3be-d01d43cf2fde'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('bf3c4c06-4343-48bc-89fa-6a688fb5d27b'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('956269f0-e5d7-4875-adad-d6c795a76d79'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Bronte Stretch Denim Shorts ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'bronte-stretch-denim-shorts', 'Bronte Stretch Denim Shorts', 'High-rise denim shorts in 11oz indigo with a touch of stretch for all-day comfort. Five-pocket styling, antique brass rivets, and a clean rolled cuff. Made from responsibly sourced cotton at our long-time mill.', 'active', ARRAY['denim', 'shorts', 'summer']::text[], 'Bronte Stretch Denim Shorts — The Bondi Store', 'High-rise stretch denim shorts in 11oz indigo, responsibly sourced cotton.', '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '23b8c1e9-3924-46de-beb1-3b9046685257'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('7e570ddf-8270-40a8-a369-b584ff5e9ff0'::uuid, 'ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BSDS-XS', 99.00, 'AUD', 0, 'deny', 0, NOW(), NOW()),
  ('dc713d96-0c0f-4195-817a-f08a1745d6d8'::uuid, 'ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BSDS-S', 99.00, 'AUD', 0, 'deny', 1, NOW(), NOW()),
  ('28f49481-a0a0-4dc4-a720-9bdf1c11f735'::uuid, 'ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BSDS-M', 99.00, 'AUD', 0, 'deny', 2, NOW(), NOW()),
  ('98ae4334-6c12-4ce8-ae34-0454cac5b68c'::uuid, 'ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BSDS-L', 99.00, 'AUD', 0, 'deny', 3, NOW(), NOW()),
  ('988c24c9-61b1-4d22-a280-1c4510435a10'::uuid, 'ff50bde4-3825-47b8-9cab-cc97663f1c97'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BSDS-XL', 99.00, 'AUD', 0, 'deny', 4, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('7e570ddf-8270-40a8-a369-b584ff5e9ff0'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('dc713d96-0c0f-4195-817a-f08a1745d6d8'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('28f49481-a0a0-4dc4-a720-9bdf1c11f735'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('98ae4334-6c12-4ce8-ae34-0454cac5b68c'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW()),
  ('988c24c9-61b1-4d22-a280-1c4510435a10'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Sandstone Ceramic Mug ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('405cacec-8774-49a9-b7d2-1e02ff01cf99'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'sandstone-ceramic-mug', 'Sandstone Ceramic Mug', 'Hand-thrown stoneware mug in unglazed sandstone with a soft-glaze interior. 350ml. No two are identical — small variations in form and glaze pooling are part of the work, not a flaw. Made by a single ceramicist in Berry, NSW.', 'active', ARRAY['ceramic', 'mug', 'home']::text[], 'Sandstone Ceramic Mug — The Bondi Store', 'Hand-thrown 350ml stoneware mug, unglazed sandstone, made in Berry NSW.', 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('405cacec-8774-49a9-b7d2-1e02ff01cf99'::uuid, 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('f143262f-dc5c-4eed-8da0-365bf89897b9'::uuid, '405cacec-8774-49a9-b7d2-1e02ff01cf99'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-SCM-350ML', 45.00, 'AUD', 0, 'deny', 0, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('f143262f-dc5c-4eed-8da0-365bf89897b9'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Coastal Soy Candle ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('1d53434b-b881-49b9-ae27-0da702f06b90'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'coastal-soy-candle', 'Coastal Soy Candle', 'Hand-poured 250g soy wax candle scented with sea salt, driftwood, and a soft trace of eucalyptus. 50-hour burn in an unglazed stoneware vessel that becomes a tumbler when the wax is gone. Refills available.', 'active', ARRAY['candle', 'home', 'soy']::text[], 'Coastal Soy Candle — The Bondi Store', '250g soy wax candle, sea salt and driftwood, hand-poured in stoneware vessel.', 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('1d53434b-b881-49b9-ae27-0da702f06b90'::uuid, 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('c0398710-8976-4334-a281-7efdae849217'::uuid, '1d53434b-b881-49b9-ae27-0da702f06b90'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-CSC-250G', 55.00, 'AUD', 0, 'deny', 0, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('c0398710-8976-4334-a281-7efdae849217'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Eucalyptus Body Oil ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('5715bd6f-a416-4293-84c2-e2e3444ea7c8'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'eucalyptus-body-oil', 'Eucalyptus Body Oil', 'Light-weight body oil in a base of organic jojoba and sweet almond, scented with Tasmanian eucalyptus, lemon myrtle, and a whisper of ylang ylang. 100ml apothecary bottle. Made in small batches in northern NSW. Vegan.', 'active', ARRAY['body', 'oil', 'wellness']::text[], 'Eucalyptus Body Oil — The Bondi Store', '100ml body oil with Tasmanian eucalyptus and lemon myrtle, organic jojoba base.', 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('5715bd6f-a416-4293-84c2-e2e3444ea7c8'::uuid, 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('287d06ca-6f4c-469a-8b22-d3081c8eaee9'::uuid, '5715bd6f-a416-4293-84c2-e2e3444ea7c8'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-EBO-100ML', 65.00, 'AUD', 0, 'deny', 0, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('287d06ca-6f4c-469a-8b22-d3081c8eaee9'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ── Bondi Beach Cotton Towel ──────────────────────────────────────
INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, description, status, tags, seo_title, seo_description, primary_category_id, published_at, created_at, updated_at)
VALUES ('b8db0672-f42d-47cc-80d4-af5974273ca3'::uuid, '8c302556-b647-4824-8ce4-73f547ca456e'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, '34247e1e-073a-4fc9-a975-11e9dc7fcebb'::uuid, 'bondi-beach-cotton-towel', 'Bondi Beach Cotton Towel', 'Turkish cotton beach towel woven flat-weave with a sand-toned stripe and rolled hem. 100×180cm. Lighter and quicker-drying than terry, packs to a third the size, and the more you wash it, the softer it gets.', 'active', ARRAY['towel', 'beach', 'cotton']::text[], 'Bondi Beach Cotton Towel — The Bondi Store', 'Turkish flat-weave cotton beach towel, 100x180cm with sand-toned stripe.', 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid, NOW(), NOW(), NOW());
INSERT INTO product_categories (product_id, category_id) VALUES ('b8db0672-f42d-47cc-80d4-af5974273ca3'::uuid, 'bd9c66b3-ad3c-4d6d-9a3d-1fa7bc8960a9'::uuid);
INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, position, created_at, updated_at) VALUES
  ('f8cda88b-436d-46e2-b83c-fe0be037e5ed'::uuid, 'b8db0672-f42d-47cc-80d4-af5974273ca3'::uuid, '8b69eea9-2537-4d36-9d99-bafcbad02dbc'::uuid, 'TBS-BBCT-100X180CM', 75.00, 'AUD', 0, 'deny', 0, NOW(), NOW());
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
  ('f8cda88b-436d-46e2-b83c-fe0be037e5ed'::uuid, '00000000-0000-0000-0000-000000000001'::uuid, 0, NOW());

-- ──────────────────────────────────────────────────────────────────
-- Sanity report
-- ──────────────────────────────────────────────────────────────────
DO $$
DECLARE
  active_products int;
  active_variants int;
  total_inventory int;
BEGIN
  SELECT count(*) INTO active_products
    FROM products
   WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid
     AND deleted_at IS NULL AND status = 'active';
  SELECT count(*), COALESCE(SUM(inventory_quantity), 0)
    INTO active_variants, total_inventory
    FROM product_variants pv
    JOIN products p ON p.id = pv.product_id
   WHERE p.tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid
     AND p.deleted_at IS NULL AND p.status = 'active'
     AND pv.deleted_at IS NULL;
  RAISE NOTICE 'catalog: % active products, % active variants, total inventory %',
    active_products, active_variants, total_inventory;
  IF active_products <> 12 THEN
    RAISE EXCEPTION 'Expected 12 active products, got %.', active_products;
  END IF;
  IF total_inventory <> 0 THEN
    RAISE EXCEPTION 'Expected total_inventory=0, got %.', total_inventory;
  END IF;
END $$;

COMMIT;
