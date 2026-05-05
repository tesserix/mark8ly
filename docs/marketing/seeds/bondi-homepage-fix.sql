-- bondi-homepage-fix.sql
-- Fixes 5 homepage issues that referenced the archived demo catalog:
--   1. hero_image_url -> new Bondi-coast hero
--   2. homepage_content.hero.subheading -> drop electronics/sports refs
--   3. homepage_content.hero.cta_secondary_url -> /products?category=linen
--   4. seo_default_description -> match new linen/cotton/lifestyle catalog
--   5. seo_og_image_url -> the slip dress lifestyle shot
\set ON_ERROR_STOP on

BEGIN;

UPDATE store_branding SET
  hero_image_url = 'https://storage.googleapis.com/tesseracthub-480811-mark8ly-media/tenants/8c302556-b647-4824-8ce4-73f547ca456e/branding/hero/eb79ff9c-480b-47f6-8a87-bbcd97096430/bondi-coast.png',
  seo_default_description = 'Sun-bleached linen, cotton basics, and small-batch lifestyle goods made for the long Bondi summer. Designed in Sydney, ships worldwide.',
  seo_og_image_url = 'https://cdn.mark8ly.com/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/2d124cf2-71a2-44e1-b913-ec068e574534/tamarama-linen-slip-dress-lifestyle.png',
  homepage_content = jsonb_set(
    jsonb_set(
      jsonb_set(homepage_content,
        '{hero,subheading}',
        '"Sun-bleached linen, cotton basics, and small-batch lifestyle goods made for the long Bondi summer. Designed in Sydney, ships worldwide."'::jsonb),
      '{hero,cta_secondary_url}',
      '"/products?category=linen"'::jsonb),
    '{hero,aside_image_alt}',
    '"Bondi-style sandstone coastline at golden hour"'::jsonb),
  updated_at = NOW()
WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid;

-- Sanity report
DO $$
DECLARE
  hero_set bool;
  og_set bool;
  subheading text;
BEGIN
  SELECT
    hero_image_url IS NOT NULL,
    seo_og_image_url IS NOT NULL,
    homepage_content -> 'hero' ->> 'subheading'
  INTO hero_set, og_set, subheading
  FROM store_branding WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid;
  RAISE NOTICE 'hero_image_url set: %, og_image set: %, subheading: %', hero_set, og_set, substring(subheading, 1, 60);
  IF NOT (hero_set AND og_set) THEN RAISE EXCEPTION 'fix failed'; END IF;
END $$;

COMMIT;
