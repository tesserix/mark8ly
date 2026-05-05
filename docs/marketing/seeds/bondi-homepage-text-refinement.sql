-- bondi-homepage-text-refinement.sql
-- Follow-up to bondi-homepage-fix.sql — tightens the editorial voice on
-- the homepage and announcement banner after a copy review:
--   * heading: "Live the Bondi life" → "For the long summer."
--   * cta_label: "Shop new arrivals" → "Shop the collection"
--   * cta_secondary_label: "Browse the collection" → "Shop linen"
--   * tagline: → "Sun-bleached linen, made for the long Bondi summer."
--   * announcement_text: corrects the free-shipping threshold from $80 → $150
--     (matches the shipping policy page)
--   * homepage_content.hero.image_url: synced to the top-level
--     hero_image_url column. The Next.js storefront renders from the JSONB
--     blob, not the column, so updating only the column was a no-op.
--
-- Run against prod marketplace-api DB (in-cluster psql pod with
-- database-init label, same pattern as bondi-policy-pages.sql).
\set ON_ERROR_STOP on

BEGIN;

UPDATE store_branding
   SET tagline           = 'Sun-bleached linen, made for the long Bondi summer.',
       announcement_text = 'Free shipping on Australian orders over A$150 · 30-day returns',
       homepage_content  = jsonb_set(
                             jsonb_set(
                               jsonb_set(
                                 jsonb_set(homepage_content,
                                   '{hero,heading}',              '"For the long summer."'::jsonb),
                                 '{hero,cta_label}',              '"Shop the collection"'::jsonb),
                               '{hero,cta_secondary_label}',      '"Shop linen"'::jsonb),
                             '{hero,image_url}',
                             to_jsonb(hero_image_url)),
       updated_at        = NOW()
 WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid;

-- Sanity report
DO $$
DECLARE
  json_hero text;
  col_hero text;
BEGIN
  SELECT homepage_content->'hero'->>'image_url',
         hero_image_url
    INTO json_hero, col_hero
    FROM store_branding
   WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e'::uuid;
  IF json_hero IS DISTINCT FROM col_hero THEN
    RAISE EXCEPTION 'hero_image_url out of sync between column (%) and homepage_content.hero.image_url (%)', col_hero, json_hero;
  END IF;
  RAISE NOTICE 'hero in sync: %', json_hero;
END $$;

COMMIT;
